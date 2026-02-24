# Project structure

This document explains the folder layout for **human navigation** and maintenance. The structure follows the [foundation plan](../design/foundation-plan.md).

## Top level

```
cmd/muxmaster/   CLI entrypoint
internal/        All application logic (10 packages)
_docs/           Design docs, project reference, legacy artifacts
```

Root meta-files: `README.md`, `CHANGELOG.md`, `CONTRIBUTING.md`, `LICENSE`, `Makefile`, `go.mod`, `.gitignore`, `.golangci.yml`, `.editorconfig`.

## Internal packages

| Package     | Status | Purpose | Key files |
|-------------|--------|---------|-----------|
| **config**  | Implemented | Defaults, CLI flags, validation | `config.go`, `flags.go`, `config_test.go` |
| **term**    | Implemented | ANSI color state, TTY detection | `term.go` |
| **logging** | Implemented | Leveled logger, optional file sink | `logger.go` |
| **display** | Partial | Banner, byte/bitrate formatting; render-plan and outlier TBD | `banner.go`, `format.go`, `format_test.go` |
| **check**   | Implemented | `--check` diagnostics and `CheckDeps` | `check.go` |
| **probe**   | Stub | ffprobe JSON → typed structs | `doc.go` |
| **naming**  | Stub | Filename parsing, output paths, collision | `doc.go` |
| **planner** | Stub | Encode vs remux vs skip, quality, audio/subtitle plans | `doc.go` |
| **ffmpeg**  | Stub | Command building, execution, retry | `doc.go` |
| **pipeline**| Stub | File discovery, per-file loop, stats | `doc.go` |

Stub packages contain a single `doc.go` with the package declaration and a comprehensive implementation plan. When implementing, split into multiple files along the boundaries documented in each `doc.go`.

## Dependency direction

```
cmd/muxmaster
  └─ config, logging, check, display

logging → config, term
display → term
term → config
check → config (+ Logger interface)

pipeline → config, logging, probe, naming, planner, ffmpeg, display
planner → config, probe, logging
ffmpeg → config, planner, logging
```

- **config** and **term** have no internal dependencies (config is a true leaf; term depends only on config).
- **probe** and **naming** stay dependency-free.
- **display** depends on **term**, not on logging — this decouples presentation from the logger.
- **pipeline** is the sole orchestrator.

## Quick finder

| To change… | Look in… |
|------------|----------|
| CLI flag or help text | `internal/config/flags.go` |
| Default value | `internal/config/config.go` (`DefaultConfig`) |
| Log level or sink | `internal/logging/logger.go` |
| Color behavior | `internal/term/term.go` |
| Banner or size/bitrate formatting | `internal/display/` |
| System check | `internal/check/check.go` |
| Probe / naming / plan / ffmpeg / pipeline | Same-named package under `internal/` |
