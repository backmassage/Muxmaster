# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) where applicable.

---

## [1.0.0] — 2025-02-23 — initial release

### Added

- Go project skeleton: config, logging, display, check, and stub packages (probe, naming, planner, ffmpeg, pipeline).
- Documentation: README, CHANGELOG, GIT_GUIDELINES, docs index, core-design, final-design, project structure, audits.
- Build: Makefile (build, test, vet, ci, install), .gitignore.

### Notes

- CLI runs config validation, --check, and path checks; pipeline not yet implemented.

---

## [2.0.0-dev] — 2025-02-23 — initial Go rewrite skeleton

### Added

- Go module and project skeleton under `cmd/muxmaster` and `internal/`.
- **Config:** Defaults, CLI flag parsing (`internal/config`), and validation (paths, mode, container, HDR).
- **Logging:** Leveled logger with optional ANSI colors and optional log file (`internal/logging`).
- **Display:** Banner and byte/bitrate formatting (`internal/display`); stubs for render-plan and outlier logging.
- **Check:** System diagnostics (`--check`) and dependency check (`CheckDeps`) for ffmpeg, ffprobe, VAAPI, x265, and AAC (`internal/check`).
- **Skeleton packages:** Stub files for `probe`, `naming`, `planner`, `ffmpeg`, and `pipeline` (no implementation yet).
- **Docs:** Project structure guide, audit with recommendations, and documentation index.
- **Build:** Makefile targets `build`, `test`, `vet`, `clean`, `install` with version/commit ldflags.
- **README:** Project overview, installation/build, and basic usage.
- **CHANGELOG:** This file.

### Notes

- Pipeline (discover → probe → plan → execute) is not yet implemented; the binary runs config, check, and path validation only.
- Unit tests are planned for a later phase; test files were removed in favor of a skeleton-first approach.

[2.0.0-dev]: https://github.com/backmassage/muxmaster/compare/v1.7.0...HEAD
