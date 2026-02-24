# Project structure

This document explains the folder layout for **human navigation** and maintenance. The structure follows the [foundation plan](../design/foundation-plan.md).

## Top level

```
cmd/             CLI entrypoint
internal/        All application logic (10 packages)
_docs/           Design docs, project reference, legacy artifacts
```

Root meta-files: `README.md`, `CHANGELOG.md`, `LICENSE`, `Makefile`, `go.mod`, `.gitignore`, `.golangci.yml`, `.editorconfig`.

## Internal packages

| Package     | Status | Purpose | Key files |
|-------------|--------|---------|-----------|
| **config**  | Implemented | Defaults, CLI flags, validation | `config.go`, `flags.go` |
| **term**    | Implemented | ANSI color state, TTY detection | `term.go` |
| **logging** | Implemented | Leveled logger, optional file sink | `logger.go` |
| **display** | Partial | Banner, byte/bitrate formatting; render-plan and outlier TBD | `banner.go`, `format.go` |
| **check**   | Implemented | `--check` diagnostics and `CheckDeps` | `check.go` |
| **probe**   | Stub | ffprobe JSON → typed structs | `doc.go` |
| **naming**  | Implemented | Filename parsing, output paths, collision, harmonization | `parser.go`, `rules.go`, `postprocess.go`, `outputpath.go`, `collision.go`, `harmonize.go`, `parser_test.go` |
| **planner** | Partial | Encode vs remux vs skip, quality, audio/subtitle plans | `doc.go`, `types.go` |
| **ffmpeg**  | Implemented | Command building, execution, retry | `builder.go`, `executor.go`, `errors.go`, `retry.go` |
| **pipeline**| Stub | File discovery, per-file loop, stats | `doc.go` |

Remaining stub packages contain a single `doc.go` with the package declaration and a comprehensive implementation plan. When implementing, split into multiple files along the boundaries documented in each `doc.go`.

For the full dependency map and rules, see [architecture.md](../architecture.md).

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
