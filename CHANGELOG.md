# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Changed

- **Architecture:** Extracted `internal/term` package for ANSI colors and TTY detection. `display` now depends on `term` instead of `logging`, breaking the cross-cutting dependency.
- **Stubs consolidated:** 22 stub files across 5 packages replaced with one `doc.go` per package containing a comprehensive implementation plan. Split into separate files when implementing.
- **Display stubs:** `outlier.go` and `renderplan.go` TODOs folded into `banner.go` package doc comment.
- **go.mod:** Bumped Go version to `1.26`.
- **.gitignore:** Stopped ignoring `go.sum` (should be committed per Go modules spec). Added patterns for `dist/`, `*.prof`, `__debug_bin*`.
- **Makefile:** Added `fmt` target; `ci` now runs `vet + fmt + docs-naming + build + test`; `hooks` target guarded for missing `.git`.
- **Logger:** Removed unused `color` and `filePath` struct fields.
- **Docs:** Merged `core-design.md` into `architecture.md`; added `_docs/index.md` entry point; updated structure, audit, and all cross-references.
- **cmd layout:** Moved `cmd/muxmaster/main.go` → `cmd/main.go`; removed nesting.

### Added

- `.golangci.yml` with 19 curated linters and project-specific exclusions.
- `.editorconfig` for cross-editor consistency (Go, Markdown, Makefile, YAML).
- `_docs/index.md` documentation entry point.

### Removed

- `testdata/README.md` placeholder — directory will be created when tests need fixtures.
- 22 individual stub files (replaced by 5 `doc.go` files + 2 display TODOs folded in).
- `CONTRIBUTING.md` — solo project; workflow info lives in `_docs/project/git-guidelines.md`.
- `internal/config/config_test.go` and `internal/display/format_test.go` — tests deferred.

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

[Unreleased]: https://github.com/backmassage/muxmaster/compare/v2.0.0-dev...HEAD
[2.0.0-dev+lint]: https://github.com/backmassage/muxmaster/compare/v2.0.0-dev+restructure...v2.0.0-dev+lint
[2.0.0-dev+restructure]: https://github.com/backmassage/muxmaster/compare/v2.0.0-dev...v2.0.0-dev+restructure
[2.0.0-dev]: https://github.com/backmassage/muxmaster/compare/v1.7.0...v2.0.0-dev
