# Project structure

This document explains the folder layout for **human navigation** and maintenance. The structure follows the [foundation plan](muxmaster-go-foundation-plan-final.md).

## Top level

- **`cmd/muxmaster/`** — CLI entrypoint. Parses flags, wires config and logger, runs either `--check` or the pipeline.
- **`internal/`** — All application logic. Not importable by other projects.
- **`docs/`** — Design docs, legacy script, and navigation. See [docs/INDEX.md](INDEX.md) for the table of contents (core-design, final-design, foundation plan, project structure, audit, legacy).

## Internal packages (by responsibility)

| Package    | Purpose | When you need to… |
|-----------|---------|-------------------|
| **config**  | Defaults, CLI flags, validation | Change a default, add a flag, or validate paths/mode. |
| **logging** | Leveled logger, colors, log file | Add a log level or change output format. |
| **display** | Banner, byte/bitrate formatting, render plans, outlier detection | Change how we present info to the user. |
| **check**    | `--check` diagnostics and `CheckDeps` | Add a system check or change encoder preflight. |
| **probe**    | ffprobe JSON → typed structs | Change what we read from media files. |
| **naming**   | Filename parsing, output paths, collision, TV year harmonization | Change naming rules or output layout. |
| **planner**  | Encode vs remux vs skip, quality, audio/subtitle plans, filters | Change decision logic or quality model. |
| **ffmpeg**   | Command building, execution, stderr classification, retry | Change ffmpeg args or retry behavior. |
| **pipeline** | File discovery, per-file loop, stats | Change discovery rules or orchestration. |

## Dependency direction

- **config** and **logging** have no internal dependencies.
- **probe** and **naming** stay dependency-light (no other `internal` packages).
- **planner** depends on config and (later) probe types.
- **ffmpeg** depends on config and planner (FilePlan).
- **pipeline** wires probe, naming, planner, ffmpeg, display; it is the only package that orchestrates the full flow.
- **check** depends only on config; it accepts a small Logger interface.

## Suggested simplifications (optional)

- **Keep the current layout.** It matches the foundation plan and keeps a clear place for each concern. Finding “where does X happen?” is straightforward.
- **Single-file packages:** `check` is one file. Merging it into `cmd/muxmaster` would reduce one package but mix CLI and diagnostics; keeping it separate is recommended.
- **Skeleton files:** Each planned file exists as a stub with a TODO. You can collapse stubs into fewer files during early development and split again when implementing, but the target layout is as above.

## Quick finder

- Add or change a **CLI flag** → `internal/config/flags.go` and optionally `config.go`.
- Change **defaults** → `internal/config/config.go` (`DefaultConfig`).
- Change **help text** → `internal/config/flags.go` (`printUsage`).
- Add a **log level or sink** → `internal/logging/logger.go`.
- Change **banner or formatting** → `internal/display/`.
- Add a **system check** → `internal/check/check.go`.
- **Probe / naming / plan / ffmpeg / pipeline** → same-named package under `internal/`.
