# Anime 4K Upscaling / Remastering Research Plan

**Status:** WIP research stub; no implementation planned yet.
**Scope:** evaluate whether Muxmaster should grow an opt-in anime remaster mode,
starting with 4K upscale experiments for anime content.

## Goal

Determine whether Muxmaster should add an explicit, experimental anime upscale
path for keeper content, and identify the smallest useful implementation shape.
This is not part of the normal compression pipeline: upscaling creates more
pixels and often larger outputs, so it conflicts with the current "preserve
quality while reducing size" assumptions.

## Non-goals

- Do not replace the current encode/remux flow.
- Do not make upscaling automatic for normal batch runs.
- Do not implement a native super-resolution engine in Go.
- Do not start with frame extraction as the primary design unless lighter paths
  fail.
- Do not preserve or optimize for Dolby Vision; existing project policy still
  applies.

## Known Local Hardware / Software Context

Observed on the current dev box:

| Area | Observed value |
|---|---|
| CPU | AMD Ryzen 5 6600H, 6 cores / 12 threads |
| GPU | Rembrandt iGPU; `lspci` labels Radeon 680M, Vulkan reports Radeon 660M |
| Vulkan driver | RADV / Mesa 26.1.2 |
| RAM | 20 GiB total, about 13 GiB available during probe |
| GPU memory | sysfs reports 3 GiB VRAM and about 10 GiB GTT; Vulkan reports UMA-style device-local budget around 7.5 GiB |
| Disk | about 135 GiB free on `/`; `/tmp` tmpfs about 9.4 GiB free |
| ffmpeg | `n8.1.1`, built with `libplacebo`, `vulkan`, `vaapi`, `vapoursynth`, `zscale`, `libvmaf` |
| Existing external upscalers | `vapoursynth` / `vspipe` installed; `video2x`, `realesrgan-ncnn-vulkan`, `realcugan-ncnn-vulkan`, Anime4K shaders not installed |

Quick synthetic local test:

```bash
ffmpeg -benchmark \
  -init_hw_device vulkan=vk:0 -filter_hw_device vk \
  -f lavfi -i testsrc2=size=1920x1080:rate=24:duration=10 \
  -vf "format=yuv420p,hwupload,libplacebo=w=3840:h=2160:upscaler=spline36,hwdownload,format=yuv420p" \
  -f null -
```

Result: 1080p -> 4K `libplacebo` spline scaling ran at about **45 fps**
(about **1.86x realtime**) before real encoding. This is only a sanity check,
not an Anime4K or AI model benchmark.

## Candidate Approaches

### 1. `libplacebo` + Anime4K Shaders

**Feasibility:** high.

FFmpeg already has `libplacebo` and Vulkan support, and `libplacebo` supports
custom mpv-style shader paths. Anime4K is shader-oriented and optimized for
real-time anime upscaling, especially clean 1080p anime viewed on 4K displays.

Likely Muxmaster shape:

- Add an explicit remaster/upscale mode.
- Add a filter-plan branch that inserts `libplacebo` scaling/shader filters.
- Force an appropriate software/Vulkan filter path before encode.
- Keep audio/subtitle/container handling in the normal planner/ffmpeg flow.

Pros:

- Best fit with the existing ffmpeg command-builder architecture.
- No frame extraction temp directory.
- Probably viable on the current Radeon 660M-class iGPU.
- Good first experiment for 1080p -> 2160p anime.

Risks:

- Anime4K shaders can oversharpen, halo, or make older/noisy sources worse.
- Exact shader chain/preset matters.
- The result is a baked upscale, while Anime4K upstream cautions that real-time
  playback upscaling avoids irreversible re-encoding.

### 2. Delegate to Video2X

**Feasibility:** medium-high after installing external tool.

Video2X 6.x supports Anime4K, Real-ESRGAN, Real-CUGAN, RIFE, Vulkan/ncnn
backends, and custom mpv-compatible GLSL shaders. It is already a video-oriented
pipeline, so it avoids some frame-sync and temp-storage problems.

Likely Muxmaster shape:

- Treat Video2X as an optional external backend.
- Muxmaster prepares inputs and final output naming.
- Video2X performs upscale/filtering.
- Muxmaster optionally performs final encode/remux/tagging if needed.

Pros:

- Fastest way to test many engines.
- Less custom orchestration.
- Likely better than building our own frame pipeline.

Risks:

- External dependency surface is large.
- Progress/error integration may be rough.
- Need clear ownership of audio/subtitle copying, metadata, and failure cleanup.

### 3. Real-ESRGAN / Real-CUGAN ncnn Vulkan

**Feasibility:** technically feasible, likely slow on this hardware.

Real-ESRGAN has an anime video model. Real-CUGAN is anime/illustration focused
and has ncnn Vulkan tooling. Both should run on AMD Vulkan hardware in principle,
but the current iGPU is not a large ML accelerator.

Expected use:

- Good for short clips and comparison stills.
- Potentially multi-hour for full 23-minute episodes.
- Best reserved for a later optional "heavy remaster" backend.

Pros:

- More visible restoration potential for 720p, DVD, bad compression, and older
  anime.
- Anime-specific models exist.

Risks:

- Runtime may be painful for full episodes.
- Tiling may be required to fit memory.
- Frame extraction workflows can consume large disk space and introduce VFR /
  timestamp / sync complexity.
- AI models can hallucinate or alter line art/style.

### 4. VapourSynth / vs-mlrt Remaster Pipeline

**Feasibility:** possible, high complexity.

VapourSynth is installed locally, and ffmpeg is built with VapourSynth support.
This is the most flexible route for serious remastering: IVTC/deinterlace,
descale, denoise, dehalo, deband, upscale, regrain, then encode.

Pros:

- Highest ceiling for genuinely difficult sources.
- Can model source-specific remaster recipes.

Risks:

- Becomes a remastering lab, not just a batch encoder.
- Requires per-source judgment.
- Hard to make safe as a generic batch option.
- Testing matrix becomes large quickly.

## Recommended Experiment Order

1. **Clip corpus:** collect short samples only:
   - clean 1080p digital anime,
   - 720p digital anime,
   - DVD/interlaced or bad-deinterlaced anime,
   - grainy/film-sourced anime,
   - heavily compressed source.

2. **Baseline outputs:**
   - current Muxmaster encode at source resolution,
   - standard ffmpeg 4K upscale using `libplacebo` spline/lanczos,
   - Anime4K shader path if shaders are installed.

3. **Visual comparison:**
   - lines and halos,
   - gradients and banding,
   - subtitle/overlay edges,
   - film grain/noise handling,
   - motion stability/flicker.

4. **Runtime comparison:**
   - fps during upscale-only,
   - fps with final encode,
   - memory/VRAM pressure,
   - output size.

5. **Heavy model comparison, only if shader path is promising:**
   - Real-ESRGAN AnimeVideo,
   - Real-CUGAN,
   - Video2X as wrapper if available.

## Architecture Notes For A Future Implementation

Upscaling should be modeled as a new explicit mode, not a tune:

```text
--remaster anime4k
--upscale-engine anime4k|video2x|realesrgan|realcugan
--target-height 2160
```

Possible internal shape:

| Concern | Likely home |
|---|---|
| CLI flags/config | `internal/config` |
| Planning upscale/remaster action | `internal/planner` |
| ffmpeg `libplacebo` filter args | `internal/ffmpeg` |
| External backend orchestration, if used | probably new leaf-ish package plus `pipeline` wiring |
| Per-file orchestration | `internal/pipeline` |

Important boundary: all direct ffmpeg interaction must remain in
`internal/ffmpeg`. If an external non-ffmpeg upscaler is introduced, it should
have its own narrow wrapper and be orchestrated by `pipeline`, not hidden inside
`planner`.

## Quality System Interaction

The current SmartQuality pipeline assumes the output should usually be smaller
than the input. A 4K remaster mode needs different rules:

- Disable or bypass "output larger than input" post-encode escalation for
  remaster outputs.
- Use target-resolution-aware bitrate/QP defaults.
- Make output-size growth explicit in logs.
- Consider a separate filename suffix/profile, e.g. `Remastered.2160p`.
- Keep remaster mode opt-in and loud.

## Open Questions

- Which Anime4K shader chain is the best first preset for 1080p -> 2160p?
- Does `libplacebo` + Anime4K remain near realtime on this Radeon 660M-class iGPU?
- How much slower is final 4K HEVC VAAPI encode after the upscale filter?
- Does source-resolution 1080p anime actually look better baked to 4K than
  player-side upscaling on the target client?
- Are Real-CUGAN / Real-ESRGAN outputs worth the runtime on this box?
- How should subtitles, cover art, chapters, and sparse-subtitle muxing interact
  with a remaster output?

## Tentative Decision

Start with **`libplacebo` + Anime4K shader experiments**. It is the best match
for the current machine and the existing Muxmaster architecture. Treat Video2X
as the second experiment path if we want to compare heavier engines quickly.
Treat Real-ESRGAN/Real-CUGAN as clip-only research until local benchmarks prove
full episodes are tolerable.

## References Already Reviewed

- Anime4K: https://github.com/bloc97/anime4k
- FFmpeg `libplacebo` filter docs: https://ffmpeg.org/ffmpeg-filters.html
- Real-ESRGAN AnimeVideo docs: https://github.com/xinntao/Real-ESRGAN/blob/master/docs/anime_video_model.md
- Real-ESRGAN ncnn Vulkan: https://github.com/xinntao/Real-ESRGAN-ncnn-vulkan
- Real-CUGAN: https://github.com/bilibili/ailab/blob/main/Real-CUGAN/README_EN.md
- Real-CUGAN ncnn Vulkan: https://github.com/nihui/realcugan-ncnn-vulkan
- Video2X: https://github.com/k4yt3x/video2x
- vs-mlrt: https://github.com/AmusementClub/vs-mlrt
