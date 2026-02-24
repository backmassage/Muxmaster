# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [2.1.0] — Unreleased

### Changed

- **AAC passthrough enforced:** Existing AAC audio streams are never re-encoded regardless of action (encode or HEVC remux). Only non-AAC streams are transcoded. The HEVC remux action label now correctly reads "encode non-AAC audio via …" instead of the misleading "encode AAC via …".

### Added

- **Audio bitrate reporting:** Per-stream input and output audio bitrates are logged for every processed file. Input bitrate (kbps) is read from `ffprobe`; output shows `copy` for AAC passthrough or the target bitrate (e.g. `256k`) for transcoded streams. Example: `Audio[0]: aac | in: 192 kbps | out: copy`.

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

[2.0.0]: https://github.com/backmassage/muxmaster/compare/v2.0.0-dev+lint...v2.0.0
[2.0.0-dev+lint]: https://github.com/backmassage/muxmaster/compare/v2.0.0-dev+restructure...v2.0.0-dev+lint
[2.0.0-dev+restructure]: https://github.com/backmassage/muxmaster/compare/v2.0.0-dev...v2.0.0-dev+restructure
[2.0.0-dev]: https://github.com/backmassage/muxmaster/compare/v1.7.0...v2.0.0-dev
