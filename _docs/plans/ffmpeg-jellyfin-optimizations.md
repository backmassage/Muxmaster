# FFmpeg Jellyfin-Playback Optimizations

**Status:** done (implemented 2026-06-12; QVBR default stays `cqp` pending real-content validation, see done-when)
**Goal:** Close playback-correctness gaps (Dolby Vision, SDR color tags, mov_text subs) and adopt researched encoder optimizations (QVBR rate control, compression_level, dialog-aware downmix, x265 AQ) with runtime capability detection so the tool stays portable beyond the dev box (AMD 680M / Mesa radeonsi).
**Non-goals:** Dolby Vision *preservation* (we strip to the HDR10 base layer); multichannel AAC output (stereo stays the target); HDR10+ dynamic metadata; AV1; two-pass loudness normalization.

## Context digest

Muxmaster is a Go orchestrator over ffmpeg targeting Jellyfin direct play: HEVC
(hevc_vaapi CQP / libx265 CRF), AAC passthrough or libfdk_aac stereo, MKV
default. Dev/primary host: AMD Radeon 680M (Rembrandt, VCN 3.1), Mesa radeonsi
26.x, libva 2.23, ffmpeg 8.1.1 (libplacebo, dovi_rpu bsf available).

User decisions (2026-06-12): strip DV / keep HDR10 base; stereo output with an
improved downmix (no 5.1 tracks); AMD-first with capability detection (Intel
must keep working); full researched scope including VAAPI rate-control changes.

### Research findings driving this plan

Verified (adversarially, with sources):

- **QVBR on AMD**: Mesa 24.3 added QVBR rate control to radeonsi VCN
  ([Mesa 24.3 relnotes](https://docs.mesa3d.org/relnotes/24.3.0)); our Mesa 26.x
  qualifies. ffmpeg's vaapi_encode queries `VAConfigAttribRateControl` at init
  and validates the requested mode against driver caps — rc support is
  runtime-detectable via a test encode.
- **`-compression_level`** maps to the VA quality level
  (`VAEncMiscParameterTypeQualityLevel`), is clamped to the driver-reported
  `VAConfigAttribEncQualityRange`, and degrades to default with a warning if
  unsupported — safe to set unconditionally.
- **AMD VAAPI is "partial support"** per the ffmpeg wiki HWAccel matrix —
  feature parity with Intel must never be assumed; every new VAAPI flag needs a
  capability gate or graceful degradation.
- **Rembrandt encodes HEVC 10-bit** (Renoir+ requirement satisfied — Jellyfin
  AMD HWA docs).
- **`dovi_rpu` bsf `strip` removes both the DOVI configuration record and the
  RPUs** ([ffmpeg bsf docs](https://ffmpeg.org/ffmpeg-bitstream-filters.html));
  it has **no profile-conversion option** (folklore). It works on `-c:v copy`.
- **libx265 only emits DV RPUs when explicitly configured** (and only with
  X265_BUILD ≥ 167); a plain re-encode drops the RPU. hevc_vaapi never writes
  RPUs.
- **hevc_vaapi `-sei hdr`** (in the default `hdr+a53_cc`) writes mastering
  display + CLL SEI from frame side data when present.
- **libplacebo tonemaps DV Profile 5** (Jellyfin uses this path on AMD via
  Vulkan); it is the only correct P5→SDR/HDR10 route.
- **Jellyfin "Dave750" downmix** puts C/LFE into L/R with −3 dB on front/back;
  known weakness: dialog still too quiet. Its "NightmodeDialogue" alternative
  boosts center hard at the cost of effects.

High-confidence but unverified by the research pass (rate-limited mid-run;
consistent with ffmpeg/x265 docs — re-confirm during implementation):

- `max_interleave_delta` defaults to 10 s (microsecond units); sparse subtitle
  streams trip it, causing premature muxer flushes / bad interleaving; `0`
  disables the cap (cost: muxer may buffer more memory).
- x265 `aq-mode=3` = auto-variance AQ biased toward dark scenes; x265 docs
  recommend it to fight banding/blocking in dark content.
- `repeat-headers` is unnecessary for MKV (container carries VPS/SPS/PPS);
  current HDR-only use is harmless.
- mov_text cannot be muxed into MKV; convert to `srt` (positioning/styling from
  tx3g is lost — acceptable).

## Files to touch

| File | Change |
|---|---|
| `internal/probe/types.go`, parse file | DV side data (`dv_profile`, `dv_bl_signal_compatibility_id`), `ChromaLocation` on VideoStream |
| `internal/planner/types.go` | `FilePlan.BSFOpts`, `FilePlan.SkipReason` wiring, rate-control fields |
| `internal/planner/planner.go` | DV policy (skip P5 / strip on remux), max_interleave_delta, attached-pic |
| `internal/planner/filter.go` | SDR color passthrough, bt709 tagging after tonemap, chroma loc |
| `internal/planner/subtitle.go` | per-stream mov_text→srt for MKV |
| `internal/planner/audio.go` | layout-aware `pan` downmix with normalized gains |
| `internal/ffmpeg/builder.go` | emit BSF opts, QVBR args, compression_level, async_depth, bf, aq-mode |
| `internal/ffmpeg/retry.go`, `errors.go` | QVBR→CQP fallback class, B-frame fallback |
| `internal/config/config.go`, `flags.go` | new knobs + detected-capability fields |
| `internal/check/check.go` | capability probes (QVBR, B-frames), write-back to cfg |
| `internal/pipeline/runner.go` | honor `ActionSkip` (currently reserved/unused) |
| `AGENTS.md`, `_docs/design/*` | document new behavior |

## Ordered steps

### Phase 1 — Playback correctness

1. **Probe: Dolby Vision detection.**
   Parse `side_data_list` entries of type `DOVI configuration record` on the
   primary video stream into `VideoStream.DoviProfile` /
   `DoviBLCompatID` (0 when absent). Also capture `chroma_location` while in
   there. Add JSON fixtures for P5, P8.1, and non-DV files.

2. **Planner: DV policy.**
   - `dv_profile == 5`: `Action = ActionSkip`, `SkipReason = "Dolby Vision profile 5 (no HDR10 base layer)"`.
     Wire `ActionSkip` through `pipeline/runner.go` (log + count in RunStats;
     the constant exists but is currently never produced).
   - DV present on the **remux** path: append `-bsf:v dovi_rpu=strip` via new
     `FilePlan.BSFOpts` (builder emits after the video codec section). Without
     this, `-c:v copy` carries the DOVI config record into MKV and DV-capable
     clients engage DV mode on a stream we otherwise treat as HDR10.
   - DV present on the **encode** path: RPU is dropped by re-encoding (both
     encoders); log a one-line note ("stripping Dolby Vision, keeping HDR10
     base layer"). Verify with a P8 sample that output has no DOVI side data.

3. **Filter/color: SDR tagging.**
   - Extend `BuildColorOpts` to pass through `-color_primaries/-color_trc/-colorspace`
     for SDR sources too (whenever the probe reports them), not just HDR10
     preserve.
   - When the HDR→SDR tonemap chain runs, explicitly tag output
     `bt709/bt709/bt709` (the zscale chain converts but the stream stays
     untagged today).
   - Pass through `-chroma_sample_location` when probed.

4. **Subtitles: mov_text→srt for MKV.**
   `BuildSubtitlePlan` currently emits `-c:s copy` for MKV; mov_text sources
   make the mux fail and the retry engine then drops *all* subs. Make the MKV
   path per-stream aware (probe already records subtitle codec): text codecs
   that MKV can't carry (`mov_text`/`tx3g`) get `-c:s:<n> srt`; everything else
   copies. Requires switching MKV subtitle mapping from `-map 0:s?` to indexed
   maps when a conversion is needed (reuse the `TextIdxs` machinery).

### Phase 2 — Mux robustness

5. **`-max_interleave_delta 0` for MKV outputs that map subtitle streams.**
   Add to `ContainerOpts` in the planner. Prevents the sparse-subtitle
   premature-flush/interleaving problem. Document the memory trade-off in a
   comment; re-confirm the 10 s default claim against ffmpeg 8 docs during
   implementation.

6. **Attached cover art (optional, small).**
   Probe already flags `IsAttachedPic`; the builder maps only the primary video
   stream, so covers are silently dropped. For MKV, map attached-pic streams
   with `-c copy` and `-disposition attached_pic`. Skip for MP4.

### Phase 3 — Encoder tuning (capability-gated)

7. **Capability detection in `check`.**
   Extend `CheckDeps`' VAAPI write-back pattern with two cheap lavfi test
   encodes against the selected render node:
   - `-rc_mode QVBR -global_quality 25 -maxrate 2M -b:v 1M` → sets
     `cfg.Encoder.VaapiQVBR = true` on success.
   - `-bf 2` (b_depth default) → sets `cfg.Encoder.VaapiBFrames = true`.
   Failures are silent capability facts, not errors. `--check` prints both.

8. **VAAPI rate control: QVBR with peak ceiling.**
   When `VaapiQVBR` is detected (Mesa ≥ 24.3 on AMD; broadly on Intel iHD):
   replace `-qp N` with `-rc_mode QVBR -global_quality N -b:v <OptimalBitrateKbps>k
   -maxrate <ceiling>k -bufsize <2×ceiling>k`, reusing the planner's existing
   `OptimalBitrateKbps` and the same ceiling logic the CPU path uses
   (`cpuMaxrateHeadroomPct`, capped at input bitrate). This bounds the peak
   spikes that cause direct-play buffering, which CQP cannot. Exact
   quality-value semantics (QP-equivalence of `global_quality` under QVBR on
   radeonsi) must be validated with 2–3 sample encodes before flipping the
   default; ship behind `--vaapi-rc=qvbr|cqp` defaulting to `cqp` until
   validated, then flip default to `qvbr-when-detected`.
   Retry engine: new error class matching rate-control init failures → fall
   back to CQP (mirrors the HW-decode fallback pattern).

9. **VAAPI `-compression_level 1`.**
   Quality-first equivalent of the CPU path's `preset slow`. Driver-clamped and
   degrades gracefully (verified), so set unconditionally on the encode path;
   expose `--vaapi-compression-level` for override. Measure encode-time impact
   on the 680M once (expectation: slower, better quality; if radeonsi maps it
   to a no-op, nothing breaks).

10. **VAAPI `-async_depth 4`.**
    Throughput-only (default 2). No playback effect; cheap win for a batch
    tool. Constant in the builder, no config knob.

11. **B-frames (`-bf`) when detected.**
    AMD VCN ≤ 4 is expected to lack HEVC B-frame encode (the research pass
    could not confirm either way — Jellyfin docs only cover H.264); Intel iHD
    supports it. When `VaapiBFrames` is detected, add `-bf 4`; retry class
    drops it on failure. On the 680M this will likely stay off — that's fine,
    it's a portability win.

12. **x265 `aq-mode=3`.**
    Append to the existing `-x265-params` string for all CPU encodes. Targets
    banding in dark scenes (the most visible artifact class on TV playback);
    aligns with the project's quality-over-compression principle. Leave
    `repeat-headers` as-is (HDR-only, harmless).

### Phase 4 — Audio downmix quality

13. **Layout-aware `pan` downmix with normalized gains.**
    Only when transcoding multichannel (>2ch) → stereo. Build the `pan` spec
    from the probed `ChannelLayout` (5.1, 5.1(side), 7.1 differ in channel
    names — a spec naming absent channels errors out). Use pan's `<` gain
    syntax (renormalizes to prevent clipping, removing any need for a
    limiter), with center channel weighted at full gain relative to ~0.6
    fronts/surrounds and ~0.3 LFE — i.e. a dialog-forward mix between
    Dave750 and NightmodeDialogue. Example for 5.1:
    `pan=stereo|FL<FC+0.60*FL+0.60*BL+0.30*LFE|FR<FC+0.60*FR+0.60*BR+0.30*LFE`.
    Prepend to the existing `aresample/aformat` chain in
    `buildAudioFilterWithRate`. Unknown/unparseable layouts fall back to
    today's plain `-ac 2`. No loudnorm (two-pass, alters dynamics; out of
    scope per decision).

### Phase 5 — Docs and verification

14. Update `AGENTS.md` (DV policy, capability detection, new gotchas) and
    `_docs/design/structure.md`. Matrix tests in `planner_test.go` for DV ×
    action, subtitle codec × container, layout × downmix.

## Done-when

- [x] `make ci` passes; new planner matrix tests cover DV profiles {none, 5, 8} × {remux, encode}, subtitle codecs {srt, ass, mov_text, pgs} × {mkv, mp4}, layouts {stereo, 5.1, 5.1(side), 7.1, unknown}.
- [ ] DV P8 sample remuxed → `ffprobe` shows **no** DOVI configuration record; encoded → ditto; P5 sample → skipped with logged reason. *(Needs real DV samples — cannot be synthesized with stock ffmpeg. Probe parsing + planner policy covered by fixture tests.)*
- [x] SDR encode of a bt709-tagged source → output stream tagged bt709 (verified end-to-end with a synthetic bt709 source: `color_space`/`chroma_location` passed through); tonemap path tags bt709 (unit-tested).
- [x] MP4-sourced file with mov_text subs → MKV output retains a usable srt stream (verified end-to-end; no retry-drop).
- [x] MKV output with subs carries `max_interleave_delta=0` (10 s / 10000000 µs default re-confirmed against ffmpeg 8.1 `-h full`).
- [x] `--check` on the 680M reports QVBR=yes (Mesa 26), B-frames=yes (note: some stacks silently degrade B-frames instead of erroring, so "yes" may mean "accepted"); CQP fallback covered by retry tests (`RetryDisableQVBR`).
- [ ] QVBR validation: 2–3 sample encodes on real content compared against CQP for size/quality/peak bitrate (`ffprobe -show_packets` peak window) before the default flips. *(Synthetic smoke test passed: QVBR encode succeeded on radeonsi and bounded output to 88% of input where CQP overshot to 131%.)*
- [x] VAAPI HDR10 sample → output bitstream contains mastering-display + CLL SEI (verified on the hw-decode path with a synthetic HDR10 source; the sw-decode zscale/hwupload variant uses the same frame-side-data mechanism).
- [x] Downmixed 5.1 sample: pan filter applied, no clipping (`astats` peak −18 dBFS on synthetic 5.1). Dialog-forward listening check on real content still recommended.
