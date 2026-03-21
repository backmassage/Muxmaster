# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [2.2.0] — 2026-03-21

### Changed

- **Quality-first smart quality curves.** All VAAPI density, resolution, and bitrate curve adjustments reduced to prioritize quality over compression. Density curve: ultra-low +8→+4, low +5→+2, below-average +3→+1. Resolution curve: 360p +6→+3, 480p +4→+2, 720p +3→+1. Bitrate curve: <1200 kbps +3→+2, <2500 kbps +2→+1. CPU curves reduced proportionally. High-density bonus made more aggressive (>8000 kbps/Mpx: -1→-2).
- **VaapiQPMax lowered from 36 to 30.** QP above 30 produces severe visible artifacts; the new cap prevents the pipeline from reaching destructive quality levels.
- **Optimal bitrate override capped at +3.** `QPForTargetBitrate` can no longer push QP more than 3 steps above the SmartQuality result, preventing the estimation model from overriding quality curves by a large margin.
- **Preflight target relaxed to 105%.** `PreflightAdjust` now tolerates 5% overshoot (was 100%) to avoid chasing marginal gains at the cost of quality. Max bumps reduced from 8 to 4.
- **Estimation high ratio reduced from 145% to 130%.** The pessimistic estimate multiplier was overestimating output, causing unnecessary preflight bumps.
- **Estimation density biases reduced.** Ultra-low: 400→250, low: 250→150, below-average: 150→80, medium: 40→20. Keeps biases proportional to the gentler quality curves.
- **Post-encode escalation made gentler.** Quality bump step reduced from 2 to 1; max re-encodes from 3 to 2. Each re-encode attempt now only increases QP/CRF by 1 instead of 2.
- **Build version injection uses `git describe --always --dirty`** instead of `git rev-parse --short HEAD`, showing uncommitted-change state in the binary version string.

### Added

- **`AGENTS.md`** — Project-level guide for AI assistants and new contributors: architecture, quality system overview, dependency rules, conventions, common gotchas.
- **Quality system architecture doc** (`_docs/design/quality-system.md`) — Full pipeline stage reference with estimation model, constants table, and density threshold definitions.
- **`doc.go` for all packages** — Package-level documentation with purpose statements and file manifests for `config`, `term`, `logging`, `display`, `check`, `probe`, `naming`, `ffmpeg`, and `pipeline`.
- **File-level header comments** on all 27 implementation files that were missing them. Each comment describes the file's role within its package.
- **Named density constants** — `DensityUltraLow` (1000) through `DensityVeryHigh` (10000) replace magic numbers in quality curves and estimation biases.
- **`.cursor/rules/`** — Three Cursor rule files (`project.mdc`, `quality.mdc`, `testing.mdc`) for project-specific AI guidance.
- **Test helper extraction** — Probe-result builders (`h264SDR`, `hevcEdgeSafe`, etc.) moved from `planner_test.go` to `helpers_test.go`.

### Fixed

- **Duplicate `// Package` doc comments.** Source files that predated `doc.go` had package-level comments that would conflict with `go doc`. Replaced with file-level comments; `doc.go` is now the single source of package documentation.
- **Stale `PreflightAdjust` docstring.** Comment still referenced "100% target" and "8 steps" after values were changed to 105% and 4.
- **README quality escalation description.** Updated to match new behavior: "bumped and re-encoded (up to 2 times)" (was "bumped by 2… up to 3 times").
- **Architecture naming rule count.** Corrected "15 naming rules" to 14 (matching `rules.go`).
- **Docs-naming check.** `AGENTS.md` added to the Makefile allowlist for root-level markdown files.
- **`.gitignore` cleanup.** Removed duplicate `.cursor/` entries; added `!.cursor/rules/` exception so rule files are tracked.

---

## [2.1.2] — 2026-02-25

### Changed

- **Post-encode quality escalation.** When the output file is larger than the input (encode path, smart quality enabled), Muxmaster now bumps QP/CRF by 2, deletes the output, and re-encodes — up to 3 times. This prevents VAAPI constant-QP encodes from producing larger files than the source, which happened when encoding already-efficient H.264 sources at low bitrate densities. Previously, only a 102% warning was logged and the larger output was accepted.
- **Improved estimation for low-density sources.** The `estimationDensityBias` values for density < 2500 kbps/Mpx have been significantly increased (e.g. density < 2500: 35→150, density < 1500: 100→250). This makes the preflight QP/CRF bump catch more cases before encoding, and produces more realistic estimated output ranges in the log.
- **Preflight target tightened to 100%.** The pre-encode estimate now bumps QP/CRF until the pessimistic output estimate is ≤100% of input (was 102%), reducing first-pass overshoots.
- **Smart quality favors higher quality.** Default VaapiQP/CpuCRF lowered from 19 to 18; SmartQualityBias from -1 to -2; OptimalBitrate base ratio for h264→HEVC raised from 65% to 68%. Produces ~2 QP steps lower (higher bitrate) for typical content.
- **AAC default bitrate 320 kbps.** Non-AAC audio transcodes now target 320k by default (was 256k).

### Fixed

- **Documentation–code audit.** Systematic review of all documentation against the codebase; every discrepancy listed below has been corrected.
- **foundation-plan.md: broken subsection numbering.** §5 subsections used 6.x, §6 used 7.x, §7 used 10.x — all renumbered to match their parent (5.x, 6.x, 7.x).
- **foundation-plan.md: stale quality retry loop.** §4.3 step 10 and §4.4 state machine updated to reflect post-encode quality escalation (bump QP/CRF when output > input, re-encode up to 3 times).
- **foundation-plan.md: wrong API signatures.** `GetOutputPath(ParsedName, config)` → `GetOutputPath(ParsedName, outputDir, container)`. `ResolveCollision(input, output)` → `CollisionResolver.Resolve(input, output)`. `ParseRule.Extract` was missing `base` parameter.
- **foundation-plan.md: stale type definitions.** `ProbeResult.Streams []StreamInfo` removed (field never existed). `VideoBitRate()` fallback now documents actual behavior (format bitrate minus audio). `AudioStream.BitRate` added. `FilePlan` gained `InputPath`, `VideoStreamIdx`, `AudioStreamCount`, `Estimate`, `PreflightBumps`, `MaxRateKbps`, `BufSizeKbps`, `OptimalBitrateKbps`. `SubtitlePlan` gained `SkipBitmap`, `TextIdxs`. `RetryState` quality fields (`QualityPass`, `MaxQualityPasses`, `QualityStep`) removed.
- **foundation-plan.md: stale Config.** Removed `SmartQualityRetryStep` (never implemented). Added `AudioEncoder` and `AnalyzeOnly`.
- **foundation-plan.md: package count.** §4.1 said "9 internal packages" — corrected to 10. Added missing `display` to entrypoint dependency list.
- **foundation-plan.md: file discovery.** Extensions list updated from 8 to 14 (added `.m4v`, `.mov`, `.mpg`, `.mpeg`, `.vob`, `.ogv`). Extras exclusion list now shows all 4 directory names (`extras`, `extra`, `bonus`, `featurettes`). Specials-folder list updated to include `extras`, `extra`, `bonus`, `featurettes`, `nc`.
- **foundation-plan.md: test file names.** Replaced nonexistent files (`quality_test.go`, `errors_test.go`, `builder_test.go`, `collision_test.go`) with actual test files (`planner_test.go`, `retry_test.go`, `probe_test.go`, `pipeline_test.go`).
- **foundation-plan.md: ffmpeg skeleton.** Added VAAPI hardware init (`-init_hw_device`, `-filter_hw_device`) and `-vf` filter chain placement to the shared command skeleton.
- **foundation-plan.md: decision table.** Removed stale "quality retry step (+2) also configurable" from smart quality bias row.
- **architecture.md: per-file flow.** Corrected `naming.OutputPath` → `naming.GetOutputPath`, `naming.ResolveCollision` → `CollisionResolver.Resolve`, `display.LogFileStats` / `display.LogRenderPlan` → internal runner helpers.
- **README.md: quality retry.** Replaced "Quality retry: if output exceeds 105% of input size" with quality escalation description (bump QP/CRF when output > input, re-encode up to 3 times). Fixed `_docs/` description ("legacy artifacts" → "design docs and project reference").

---

## [2.1.1] — 2026-02-25

### Changed

- **AAC passthrough is now unconditional.** Removed the 400 kbps bitrate threshold that caused lossy-to-lossy re-encoding of high-bitrate AAC streams (e.g. 415 kbps BluRay audio). AAC is already the target codec for Jellyfin direct play — bitrate has no bearing on compatibility.
- **Remux path no longer applies `+genpts`.** `CleanTimestamps` (`-fflags +genpts+discardcorrupt`, `-avoid_negative_ts make_zero`) is now disabled for `ActionRemux`. Edge-safe HEVC sources don't need PTS regeneration, and the flags were adding 4–7% container overhead on every remux. The retry engine can still activate timestamp repair if ffmpeg actually fails with a timestamp error.
- **Removed `-max_interleave_delta 0`.** ffmpeg's default 10-second interleaving buffer is more optimal for cluster packing than the legacy zero-buffer setting. Improves MKV output size slightly, especially with sparse subtitle streams.
- **Batch header log updated.** Audio line now reads "AAC passthrough, non-AAC encode to AAC via …" (was "AAC passthrough if <400 kbps"). HEVC remux line now reads "copy/encode audio" (was "encode audio").
- **Documentation streamlined.** Deleted `git-guidelines.md`, `audit.md`, and `legacy/` folder. Moved `structure.md` to `_docs/design/`. Removed obsolete sections from `foundation-plan.md` (migration phases, gap analysis, bootstrap checklist, duplicate repo structure). Added status callout to `product-spec.md`. Renumbered `foundation-plan.md` sections 1–9.

### Fixed

- **`OptimalBitrate` nil-pointer on files without video.** `OptimalBitrate` returned the format-level bitrate instead of 0 when `PrimaryVideo` was nil (e.g. audio-only files reaching the estimator). Now early-returns 0.
- **Retry test stderr strings didn't match regex patterns.** `TestAdvance_DropAttachments` used a string that didn't match `reAttachmentIssue`; `TestAdvance_MuxQueueEscalation` was missing "stream" from the mux queue overflow pattern. Both now use realistic ffmpeg stderr excerpts.
- **Stale ratio table spot-checks.** `TestVaapiRatioTable` expected QP27→390 (actual: 395); `TestCpuRatioTable` expected CRF28→230 (actual: 235). Updated to match the implemented tables.
- **Preflight test used wrong target threshold.** `TestPreflightAdjust_CompressedSource` used 105% target but the planner calls `PreflightAdjust` with 100%. Test now uses 100% to match the actual call site.
- **4K density assertion too strict.** `TestSmartQuality_Matrix` asserted QP > 19 for all sources with density < 2500 kbps/Mpx, but at 4K the resolution bonus (−1) and bitrate bonus (−1) offset the density penalty (+3), correctly producing QP=19. Assertion now excludes 4K+ resolutions where bonuses are expected to dominate.

---

## [2.1.0] — 2026-02-24

### Changed

- **AAC passthrough enforced:** Existing AAC audio streams are never re-encoded regardless of action (encode or HEVC remux). Only non-AAC streams are transcoded. The HEVC remux action label now correctly reads "encode non-AAC audio via …" instead of the misleading "encode AAC via …".
- **Analyze table: Audio Kbps column removed.** The per-file audio bitrate column has been removed from `--analyze` output (slow and unreliable). The audio description column (codec + channel count) remains.
- **Analyze table: rich per-column coloring.** Resolution (4K→cyan, 1080p→green, 720p→yellow, SD→orange), video codec (HEVC/AV1→green, H.264→blue, legacy→orange), audio description (AAC→green, FLAC/PCM→cyan, AC3/DTS→yellow), bold headers, and dim separators. Two new ANSI variables (`Bold`, `Dim`) added to the `term` package.
- **Banner rainbow uses `term.*` variables** instead of hardcoded ANSI escape codes, ensuring the banner stays in sync with color configuration.
- **`FormatBitrateLabel` returns "—" for unknown bitrate** instead of "0 kbps" when the bitrate is zero or negative.

### Added

- **Batch analysis mode** (`--analyze` / `-a`): Probe-only mode that scans all media files in a directory and prints a tabular report of resolution, video codec, video kbps, and audio description. Uses IQR-based statistical detection to highlight outliers (`[*]` orange) and extreme outliers (`[!]` red) in the video bitrate column. Summary prints IQR bounds and counts. Usage: `muxmaster --analyze /path/to/media`.
- **Audio bitrate reporting:** Per-stream input and output audio bitrates are logged for every processed file. Input bitrate (kbps) is read from `ffprobe`; output shows `copy` for AAC passthrough or the target bitrate (e.g. `256k`) for transcoded streams. Example: `Audio[0]: aac | in: 192 kbps | out: copy`.
- **Audio `BitRate` in probe:** `AudioStream.BitRate` field parsed from `ffprobe` `bit_rate` for per-stream bitrate reporting.

### Fixed

- **Nil-pointer dereference in `logFileStats`:** Added nil guard for `pr.PrimaryVideo` — function previously assumed the caller validated, but was itself unsafe.
- **`VideoStreamIdx` not set in `BuildPlan`:** The plan's video stream index was silently defaulting to 0 (wrong if stream 0 is a thumbnail). Now set inside `BuildPlan` from probe data.
- **`Container` not set in `BuildPlan`:** Same fragile caller-sets-field pattern — `plan.Container` was set by `runner.go` after `BuildPlan` returned. Moved into `BuildPlan` so any caller gets the correct value.
- **Duplicate `clamp()` and quality constants:** Both `planner/quality.go` and `ffmpeg/retry.go` defined identical functions and constants. Exported from `planner` as `Clamp`, `CpuCRFMin`, etc.; removed duplicates from `ffmpeg`.
- **Custom `itoa` reimplementation in `probe/types.go`:** Removed 15-line hand-rolled function; replaced with `strconv.Itoa`.
- **Misleading function name `colorPadRight`:** Renamed to `colorRightAlign` — the function right-aligns (left-pads), not right-pads.
- **Analyze table alignment:** ANSI color escape sequences no longer break column padding. Plain text is padded first, then wrapped in color codes.
- **Architecture docs:** Fixed false `logging` dependency listed for `planner` and `ffmpeg` packages; added missing `term` dependency for `pipeline`.
- **Project docs:** Updated structure.md (all packages implemented, display no longer "Partial", analyze.go listed), fully rewrote audit.md to reflect current state.

---

## [2.0.0] — 2026-02-24 — Go rewrite release

Complete rewrite from a 2,600-line Bash script to a single static Go binary with full CLI parity.

### Added

- **Probe** (`internal/probe`): `ffprobe` JSON parsing, HDR detection (`smpte2084`/`arib-std-b67`/`bt2020`), interlace detection, HEVC edge-safety validation (profile + pix_fmt), bitmap subtitle codec identification.
- **Naming** (`internal/naming`): 14 ordered regex rules for TV/movie/specials filename parsing, Jellyfin-friendly output paths (`Show/Season XX/Show - SXXEXX.mkv`, `Movie (Year)/Movie (Year).mkv`), collision resolution with `- dupN` suffixes, TV show year harmonization index.
- **Planner** (`internal/planner`): Smart per-file QP/CRF selection from resolution×bitrate curves with configurable bias, output bitrate estimation, video filter chain building (yadif, HDR tonemapping, VAAPI hwupload), audio planning (AAC passthrough, non-AAC→AAC 256k/48kHz/2ch), subtitle/attachment planning (MKV copy-all, MP4 mov_text), stream disposition management.
- **FFmpeg** (`internal/ffmpeg`): Full command builder from `FilePlan` + `RetryState`, executor with stderr capture and optional tee, regex-based error classification (attachment, subtitle, mux queue, timestamp), two-tiered retry state machine (error fixes + quality adjustment).
- **Pipeline** (`internal/pipeline`): Recursive file discovery with `extras` directory pruning, per-file orchestration (validate→probe→name→plan→execute→report), TV year index building, batch header/summary logging, bitrate outlier detection, `RunStats` aggregation with space-saved reporting.
- **Main integration** (`cmd/main.go`): Signal handling via `context.WithCancel` + `SIGINT`/`SIGTERM`, graceful shutdown between files, exit code 1 on failures.
- **Rainbow banner**: ASCII art logo rendered in 5 cycling ANSI colors (red/orange/yellow/green/blue) with plain-text fallback.
- Comprehensive test suites for `naming` (14 parse rules + post-processing + path generation), `probe` (JSON parsing + HDR + interlace + HEVC safety + live integration), `planner` (quality curves + filters + audio/subtitle plans), and `pipeline` (discovery + dry-run integration).

### Changed

- **Architecture:** Extracted `internal/term` package for ANSI colors and TTY detection.
- **Stubs consolidated:** 22 stub files replaced with full implementations across 5 packages.
- **go.mod:** Bumped Go version to `1.26`.
- **Makefile:** Version set to `2.0.0`; added `fmt` target; `ci` runs `vet + fmt + docs-naming + build + test`.
- **cmd layout:** Moved `cmd/muxmaster/main.go` → `cmd/main.go`.
- **Check:** `CheckDeps` now derives `VaapiProfile`/`VaapiSwFormat` from capability probing (main10/p010 preferred over main/nv12).
- **Docs:** Merged `core-design.md` into `architecture.md`; added `_docs/index.md` entry point; fully updated README with usage examples and option reference.

### Removed

- `testdata/README.md` placeholder.
- 22 individual stub files.
- `CONTRIBUTING.md` (solo project).

---

## [2.0.0-dev+lint] — 2026-02-23 — linting and code audit

### Added

- `.golangci.yml` with 16 curated linters and project-specific exclusions.
- `internal/config/config_test.go`: 8 table-driven test functions for validation and defaults.
- `internal/display/format_test.go`: 3 table-driven test functions for formatting helpers.
- Makefile targets: `lint`, `coverage`.

### Fixed

- `flags.go`: Removed redundant `version` variable that shadowed `main.version` (help text now reflects build-time version).
- `check.go`: Handle ignored errors from `cmd.Output()`, deduplicated CPU test args.
- `logger.go`: Renamed parameter shadowing struct field; extracted helper functions.
- `banner.go`: Use `logging.Magenta` variable instead of hardcoded ANSI escape.
- `main.go`: Display commit hash in startup log; document two-phase error handling.
- `probe/types.go`, `naming/parser.go`: Fixed stale doc references to renamed foundation plan.

---

## [2.0.0-dev+restructure] — 2026-02-23 — project restructuring

### Changed

- Reorganized `docs/` into `design/`, `project/`, and `legacy/` subdirectories.
- Renamed doc files to lowercase-with-hyphens convention.
- Merged structure audit and guidelines audit into single `docs/project/audit.md`.
- Removed redundant `final-design.md` wrapper (content covered by `product-spec.md`).

### Added

- `.gitignore` for build artifacts, IDE files, coverage output.
- `LICENSE` (MIT).
- `CONTRIBUTING.md` with setup, workflow, and conventions.
- `.editorconfig` for cross-editor consistency.
- `docs/architecture.md` with Mermaid package dependency diagram.
- `scripts/commit-msg` Git hook for conventional commit validation.
- `testdata/` directory for future test fixtures.
- Makefile targets: `lint`, `coverage`.

---

## [2.0.0-dev] — 2025-02-23 — initial Go rewrite skeleton

### Added

- Go module and project skeleton under `cmd/muxmaster` and `internal/`.
- **Config:** Defaults, CLI flag parsing (`internal/config`), and validation (paths, mode, container, HDR).
- **Logging:** Leveled logger with optional ANSI colors and optional log file (`internal/logging`).
- **Display:** Banner and byte/bitrate formatting (`internal/display`); stubs for render-plan and outlier logging.
- **Check:** System diagnostics (`--check`) and dependency check (`CheckDeps`) for ffmpeg, ffprobe, VAAPI, x265, and AAC (`internal/check`).
- **Skeleton packages:** Stub files for `probe`, `naming`, `planner`, `ffmpeg`, and `pipeline` (no implementation yet).
- **Docs:** Project structure guide, design docs, and documentation index.
- **Build:** Makefile targets `build`, `test`, `vet`, `ci`, `clean`, `install` with version/commit ldflags.
- **README:** Project overview, installation/build, and basic usage.
- **CHANGELOG:** This file.

### Notes

- Pipeline (discover → probe → plan → execute) is not yet implemented; the binary runs config, check, and path validation only.
- Unit tests are planned for a later phase; test files were removed in favor of a skeleton-first approach.

[2.2.0]: https://github.com/backmassage/muxmaster/compare/v2.1.2...v2.2.0
[2.1.2]: https://github.com/backmassage/muxmaster/compare/v2.1.1...v2.1.2
[2.1.1]: https://github.com/backmassage/muxmaster/compare/v2.1.0...v2.1.1
[2.1.0]: https://github.com/backmassage/muxmaster/compare/v2.0.0...v2.1.0
[2.0.0]: https://github.com/backmassage/muxmaster/compare/v2.0.0-dev+lint...v2.0.0
[2.0.0-dev+lint]: https://github.com/backmassage/muxmaster/compare/v2.0.0-dev+restructure...v2.0.0-dev+lint
[2.0.0-dev+restructure]: https://github.com/backmassage/muxmaster/compare/v2.0.0-dev...v2.0.0-dev+restructure
[2.0.0-dev]: https://github.com/backmassage/muxmaster/compare/v1.7.0...v2.0.0-dev
