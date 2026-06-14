# Muxmaster — Agent Guide

Jellyfin-oriented batch media encoder. Go orchestration layer over ffmpeg/ffprobe.

## Build and test

```bash
make build       # build with version/commit injection (git describe --always --dirty)
make test        # all tests, verbose
make ci          # vet + fmt + docs-naming + build + test
make lint        # golangci-lint (19 linters)
make coverage    # HTML coverage report
```

Plain `go build ./cmd` works but skips version/commit injection.

## Architecture

- Package dependency map: `_docs/architecture.md`
- Full type and behavioral reference: `_docs/design/foundation-plan.md`
- File finder ("where to change what"): `_docs/design/structure.md`
- Quality pipeline design: `_docs/design/quality-system.md`

### Package dependency rules

Dependencies flow top-down. Never introduce upward or circular dependencies.

```
cmd/main.go           → config, logging, check, display, pipeline, ffmpeg
internal/config       → (nothing internal)
internal/term         → config
internal/logging      → config, term
internal/display      → term
internal/check        → config
internal/probe        → (nothing internal — pure logic + ffprobe)
internal/naming       → (nothing internal — pure logic)
internal/planner      → config, probe
internal/tune         → config             (grain pre-pass; shells ffmpeg like check)
internal/ffmpeg       → config, planner
internal/pipeline     → config, probe, naming, planner, ffmpeg, tune, display, term
```

**Leaf packages** (`config`, `probe`, `naming`) must stay dependency-free. `term` is near-leaf (imports only `config`).
`planner` combines config + probe; it must never import ffmpeg or pipeline.
`pipeline` is the sole orchestrator — only it wires all packages together.

### Per-file processing flow

```
pipeline.Run → for each file:
  1. Validate (readable, >1KB)
  2. probe.Probe → ProbeResult
  3. logInputMeta (codec, resolution, bitrate, HDR/interlace flags)
  4. naming.ParseFilename → ParsedName
  5. naming.GetOutputPath → output path
  6. CollisionResolver.Resolve → final path
  7. planner.BuildPlan → FilePlan (sets HWDecode for VAAPI)
  8. ffmpeg.Execute with retry loop
  9. Post-encode quality escalation (if output > input)
  10. Update RunStats
```

## Quality system (SmartQuality pipeline)

The quality system spans `internal/planner/` and `internal/pipeline/runner.go`:

```
SmartQuality (quality.go)
  → resolution, bitrate, and density curves adjust QP/CRF from defaults
  → SmartQualityBias applied (default -2, favors quality)

OptimalBitrate (estimation.go)
  → target output kbps from codec generation gain + density

QPForTargetBitrate (estimation.go)
  → find QP closest to optimal target

Bounded push (planner.go)
  → AMD/VCN: finalQP = max(baseQP, min(targetQP, qpCeiling))  [quality-first default]
  → size-priority / non-AMD: capped at baseQP + 3              [legacy fallback]
  → quality-priority: push skipped entirely

PreflightAdjust (estimation.go)
  → bump QP until estimate ≤ 105% of input, max 4 bumps

Post-encode escalation (pipeline/runner.go)
  → if output > input after encode, bump QP by 1, re-encode (max 2 times)
```

Design principle: **quality over compression**. Accept mild size overshoot rather
than destroying quality. The post-encode loop handles genuine blowups.

## Key conventions

- Config struct uses sub-structs: `cfg.Encoder.*` (video encoder), `cfg.Audio.*` (audio), `cfg.Display.*` (logging/output). Pipeline behavior flags remain at the top level.
- Testability seams: `pipeline.Logger` interface decouples runner/report from `*logging.Logger`; `ffmpeg.RunFunc` decouples `Execute` from real subprocesses. Both accept mocks in tests.
- VAAPI constant-QP encoding by default; `--vaapi-rc qvbr` switches to QVBR (quality target + maxrate ceiling) when the driver supports it (detected by `CheckDeps`, see below). CPU uses CRF with maxrate ceiling and `aq-mode=3` (dark-scene AQ). **CQP is the quality-per-bit default and stays that way** — VCN 3.1 benchmarking showed QVBR loses to CQP at matched size (95.9 vs 98.9 VMAF, because `-b:v` binds before `-global_quality`); QVBR is opt-in *only* for bounding peak bitrate on bandwidth-limited streaming.
- **Corrected encoder reality on AMD VCN 3.x** (measured on the dev box; see memory `vcn31-vaapi-encode-facts`): HEVC B-frames are a no-op (the driver accepts `-bf` and encodes cleanly but emits only I+P — Mesa added H.264 B-frames only), so `testVaapiBFrames` now *verifies* a B-frame is present via ffprobe rather than trusting exit status; `-compression_level` is bit-identical across all values for HEVC (kept only for Intel/discrete-AMD portability). Neither fact is AMD-special-cased — the probe stays generic, so Intel iHD still reports B-frames: yes.
- **Content-aware prefiltering (`--tune auto|none|film|grain|anime`).** Injects a CPU prefilter into the software-decode chain: `film`→`hqdn3d=1.5:1.5:6:6`, `grain`→`hqdn3d=4:4:9:9` (denoise — the biggest quality-per-bit lever on grainy Blu-ray), `anime`→`gradfun=1.2:16` (deband). Pre-encode denoise on the software path is the only working lever because `denoise_vaapi` is unimplemented on radeonsi and CPU filters cannot enter a hardware VAAPI surface chain (ffmpeg error −22). **Gotcha: an active prefilter forces software decode** (`plan.HWDecode=false`) in VAAPI mode — `TunePrefilter(cfg)!=""` is checked in `BuildPlan` before the HWDecode gate. `anime` carries **no** QP/CRF bias (the deband handles banding; an earlier `+1` was reversed for the quality-first default — `tuneQPBias`/`tuneCRFBias` in `quality.go`). Denoise strengths are named constants in `filter.go` so sweep-selected values drop in.
- **`--tune auto` (default) — per-series grain detection + prompt.** `internal/tune` runs a cheap ffmpeg grain pre-pass (`bitplanenoise` LSB over a downscaled/thinned ~24 s window; `tune.DetectContent`→`Suggest`) and the pipeline (`autotune.go`) groups files by series, prompts the user **once per series** to confirm/override the suggested profile, and applies it to every episode. Grain is the only reliably auto-detected axis (deep research found no robust anime-vs-live-action classifier), so the user makes the film/anime call. **Gated on `--tune auto` AND an interactive stdin** (`term.IsTerminal(os.Stdin)`): non-interactive runs resolve to `none` (no pre-pass, no prompt — byte-identical to pre-tune args, so all pipeline tests are unaffected). `TunePrefilter(TuneAuto)` returns `""`, so the planner treats unresolved `auto` as a no-op; the pipeline resolves `auto`→concrete via a shallow `cfg` copy before `BuildPlan`. Explicit `--tune none|film|grain|anime` skips the pre-pass/prompt and applies globally. `--analyze` uses `Analyze` (not `Run`), so it never prompts. Grain thresholds in `tune/suggest.go` are **initial estimates** pending a real-Blu-ray sweep.
- **`--quality-priority`** (default off) skips the §2b optimal-bitrate *upward* QP/CRF push in `planner.go`, letting the SmartQuality value stand for keeper content; the preflight/post-encode size safety nets stay intact. Pairs naturally with `--tune`.
- **Quality-first push + per-vendor gating (`hevc_vaapi`, plan `hevc-vaapi-quality-maximization.md` — Changes 1/5/6 landed).** The §2b push is bounded by an absolute QP ceiling (`finalQP = max(baseQP, min(pushTargetQP, qpCeiling))`, `quality.go::boundedPushQP`) instead of the legacy `base + maxOptimalOverride`. **Gated to AMD/VCN** (`cfg.Encoder.VaapiVendor`, detected from the render node's PCI ids in `check.go`); non-AMD/unknown vendors and `--size-priority` keep the legacy unbounded push (the conservative fallback). The stacked-bias invariant `clamp(baseCurveQP + globalBias + tuneBias, contentClassMin, VaapiQPMax)` in `SmartQuality` keeps grain pinned at its knee. **Production values (reasoned estimates, not placeholders): `vcnQPCeilingClean=16`, `vcnQPCeilingGrain=24`, `vcnContentClassMinGrain=24` (the measured grain knee — VMAF 99 at qp25), `fallbackQPCeiling=21`; `--denoise-qp-bias` (Change 4) stays validation-gated off.** Change 6 resolved: preflight no longer overrides the ceiling. `PreflightAdjust` (`estimation.go`) was re-keyed from the ×1.30 HIGH estimate to the POINT estimate (triggers only when predicted output > ~105% of input), so a 1080p h264 BD now holds at the ceiling QP16 (point ≈92%) instead of being dragged to ~QP20. The `vaapiRatios` table stays an **approximate display-only heuristic** (not refit); the **post-encode escalation loop in `pipeline/runner.go` is now the primary real-size anti-bloat guard** (output>input → bump QP, re-encode, max 2×) since softened preflight no longer nets genuine overshoots the estimator mispredicts.
- VAAPI capability detection: `check.CheckDeps` runs cheap test encodes on the selected render node and writes `cfg.Encoder.VaapiQVBR` / `VaapiBFrames` back to config. Failures are capability facts, not errors. AMD VAAPI is "partial support" — never assume Intel feature parity; every new VAAPI flag needs a capability gate or graceful degradation (`-compression_level` and `-async_depth` degrade gracefully and are set unconditionally).
- Dolby Vision policy: profile 5 (no HDR10 base layer) → `ActionSkip` with reason. Any other DV profile: remux appends `-bsf:v:0 dovi_rpu=strip=1` (else `-c:v copy` carries the DOVI config record and DV clients engage DV mode); encode drops RPUs inherently (neither encoder writes them). We never preserve DV.
- VAAPI hardware decode enabled by default (full GPU pipeline); falls back to software decode for HDR tonemap and H.264 10-bit (Hi10p) sources.
- HDR10 static metadata (mastering display + MaxCLL/MaxFALL) parsed from ffprobe `side_data_list`. CPU mode injects via `-x265-params`. VAAPI has no `-master_display`/`-max_cll` options: `hevc_vaapi` re-emits the HDR SEI from decoded-frame side data (parsed from the source bitstream by the decoder), which survives both the HW-decode surface chain and the SW prefilter chains. So `plan.MasterDisplay`/`MaxCLL` are intentionally *not* consumed on the VAAPI branch — the builder instead pins `-sei hdr+a53_cc` (= the encoder default, preserving a53_cc captions) when HDR10 metadata is present, so emission no longer depends on an ffmpeg default that could change silently. **Verified empirically on ffmpeg 8.1 / Mesa radeonsi (VCN 3.1):** ffprobe confirms "Mastering display metadata" + "Content light level metadata" in the VAAPI output bitstream across HW-decode and all `--tune` SW-prefilter paths (see memory `vcn31-vaapi-encode-facts`).
- VaapiQPMax = 30 — QP above this produces severe visible artifacts.
- AAC audio is always passthrough (never re-encoded lossy-to-lossy).
- Multichannel→stereo transcodes use a dialog-forward `pan` downmix (center at full weight, 0.6 fronts/surrounds, 0.3 LFE) built from the probed channel layout; pan's `<` syntax renormalizes gains so it cannot clip. Unknown layouts fall back to plain `-ac 2`.
- Encode output is always tagged: probed color/chroma tags pass through for SDR and HDR10-preserve; the HDR→SDR tonemap path tags bt709 explicitly.
- MKV outputs that map subtitles get `-max_interleave_delta 0` (sparse subs otherwise trip the muxer's 10 s interleave cap).
- MKV subtitle handling is per-stream when needed: mov_text/tx3g converts to srt via indexed maps; everything else copies.
- MKV outputs carry cover art (attached_pic video streams) via per-stream `-c:v:N copy`; the spec must be emitted *after* the global `-c:v` to win for that stream.
- Remux path skips timestamp fix (+genpts); retry engine handles failures.
- Output directory must never be inside input directory (Config.ValidatePaths).
- All ffmpeg interaction goes through `internal/ffmpeg/` — never call exec directly.
- Tests use probe-result builders (h264SDR, hevcEdgeSafe, etc.) in planner helpers_test.go.
- Matrix tests cover every resolution × bitrate × codec combination.

## Common gotchas

- `probe.ProbeResult.PrimaryVideo` can be nil (audio-only files) — always nil-check.
- `VideoBitRate()` falls back to format bitrate minus audio when stream bitrate is zero.
- Naming parser uses ordered regex rules — rule priority matters (first match wins).
- The retry engine handles 7 error classes: attachment, subtitle, mux queue, timestamp, hardware decode (falls back to software decode + hwupload via `FilePlan.SWVideoFilters`), rate control (QVBR → CQP), and B-frames (drop `-bf`).
- `dovi_rpu=strip=1` has no profile-conversion option (folklore); it removes the config record and RPUs, nothing more. It works on `-c:v copy`. The `strip` option is a boolean and needs the explicit `=1` — bare `dovi_rpu=strip` fails to parse ("Unable to parse \"strip\" option value \"strip\" as boolean").
- QVBR requires smart quality: without an `OptimalBitrateKbps` target (override set or `--no-smart-quality`) the planner silently falls back to CQP.
- Density = kbps × 1,000,000 / pixels (kbps per megapixel).
- H.264 10-bit (Hi10p) sources disable VAAPI hardware decode — most drivers lack AVC 10-bit decode support. `vaapiHWDecodeViable` in `planner.go` gates this via `probe.PixFmtIs10Bit`.
