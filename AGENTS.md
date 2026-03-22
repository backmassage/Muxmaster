# Muxmaster — Agent Guide

Jellyfin-oriented batch media encoder. Go orchestration layer over ffmpeg/ffprobe.

## Build and test

```bash
make build       # build with version/commit injection (git describe --always --dirty)
make test        # all tests, verbose
make ci          # vet + fmt + docs-naming + build + test
make lint        # golangci-lint (16 linters)
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
cmd/main.go           → config, logging, check, display, pipeline
internal/config       → (nothing internal)
internal/term         → config
internal/logging      → config, term
internal/display      → term
internal/check        → config
internal/probe        → (nothing internal — pure logic + ffprobe)
internal/naming       → (nothing internal — pure logic)
internal/planner      → config, probe
internal/ffmpeg       → config, planner
internal/pipeline     → config, logging, probe, naming, planner, ffmpeg, display, term
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
- VAAPI constant-QP encoding; CPU uses CRF with maxrate ceiling.
- VAAPI hardware decode enabled by default (full GPU pipeline); falls back to software decode for HDR tonemap.
- HDR10 static metadata (mastering display + MaxCLL/MaxFALL) parsed from ffprobe `side_data_list`. CPU mode injects via `-x265-params`; VAAPI relies on frame side-data passthrough.
- VaapiQPMax = 30 — QP above this produces severe visible artifacts.
- AAC audio is always passthrough (never re-encoded lossy-to-lossy).
- Remux path skips timestamp fix (+genpts); retry engine handles failures.
- Output directory must never be inside input directory (Config.ValidatePaths).
- All ffmpeg interaction goes through `internal/ffmpeg/` — never call exec directly.
- Tests use probe-result builders (h264SDR, hevcEdgeSafe, etc.) in planner helpers_test.go.
- Matrix tests cover every resolution × bitrate × codec combination.

## Common gotchas

- `probe.ProbeResult.PrimaryVideo` can be nil (audio-only files) — always nil-check.
- `VideoBitRate()` falls back to format bitrate minus audio when stream bitrate is zero.
- Naming parser uses ordered regex rules — rule priority matters (first match wins).
- The retry engine handles 4 error classes: attachment, subtitle, mux queue, timestamp.
- Density = kbps × 1,000,000 / pixels (kbps per megapixel).
