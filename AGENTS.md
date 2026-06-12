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
internal/ffmpeg       → config, planner
internal/pipeline     → config, probe, naming, planner, ffmpeg, display, term
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
  → capped at SmartQuality QP + 3 (planner.go)

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
- VAAPI constant-QP encoding by default; `--vaapi-rc qvbr` switches to QVBR (quality target + maxrate ceiling) when the driver supports it (detected by `CheckDeps`, see below). CPU uses CRF with maxrate ceiling and `aq-mode=3` (dark-scene AQ).
- VAAPI capability detection: `check.CheckDeps` runs cheap test encodes on the selected render node and writes `cfg.Encoder.VaapiQVBR` / `VaapiBFrames` back to config. Failures are capability facts, not errors. AMD VAAPI is "partial support" — never assume Intel feature parity; every new VAAPI flag needs a capability gate or graceful degradation (`-compression_level` and `-async_depth` degrade gracefully and are set unconditionally).
- Dolby Vision policy: profile 5 (no HDR10 base layer) → `ActionSkip` with reason. Any other DV profile: remux appends `-bsf:v:0 dovi_rpu=strip` (else `-c:v copy` carries the DOVI config record and DV clients engage DV mode); encode drops RPUs inherently (neither encoder writes them). We never preserve DV.
- VAAPI hardware decode enabled by default (full GPU pipeline); falls back to software decode for HDR tonemap and H.264 10-bit (Hi10p) sources.
- HDR10 static metadata (mastering display + MaxCLL/MaxFALL) parsed from ffprobe `side_data_list`. CPU mode injects via `-x265-params`; VAAPI relies on frame side-data passthrough.
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
- `dovi_rpu=strip` has no profile-conversion option (folklore); it removes the config record and RPUs, nothing more. It works on `-c:v copy`.
- QVBR requires smart quality: without an `OptimalBitrateKbps` target (override set or `--no-smart-quality`) the planner silently falls back to CQP.
- Density = kbps × 1,000,000 / pixels (kbps per megapixel).
- H.264 10-bit (Hi10p) sources disable VAAPI hardware decode — most drivers lack AVC 10-bit decode support. `vaapiHWDecodeViable` in `planner.go` gates this via `probe.PixFmtIs10Bit`.
