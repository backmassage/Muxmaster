# Project structure

Folder layout for navigation and maintenance. For architecture and type reference, see [foundation-plan.md](foundation-plan.md).

## Top level

```
cmd/             CLI entrypoint
internal/        All application logic (10 packages)
_docs/           Design docs and project reference
```

Root meta-files: `README.md`, `CHANGELOG.md`, `LICENSE`, `Makefile`, `go.mod`, `.gitignore`, `.golangci.yml`, `.editorconfig`.

## Internal packages

| Package     | Purpose | Key files |
|-------------|---------|-----------|
| **config**  | Defaults, CLI flags, validation | `config.go`, `flags.go` |
| **term**    | ANSI color state, TTY detection | `term.go` |
| **logging** | Leveled logger, optional file sink | `logger.go` |
| **display** | Banner, byte/bitrate formatting | `banner.go`, `format.go` |
| **check**   | `--check` diagnostics and `CheckDeps` | `check.go` |
| **probe**   | ffprobe JSON → typed structs, HDR/interlace/HEVC-safe detection | `types.go`, `prober.go`, `hdr.go`, `interlace.go`, `probe_test.go` |
| **naming**  | Filename parsing, output paths, collision, harmonization | `parser.go`, `rules.go`, `postprocess.go`, `outputpath.go`, `collision.go`, `harmonize.go`, `parser_test.go` |
| **planner** | Encode vs remux vs skip, smart quality, estimation, audio/subtitle/filter plans | `types.go`, `planner.go`, `quality.go`, `estimation.go`, `filter.go`, `audio.go`, `subtitle.go`, `disposition.go`, `planner_test.go` |
| **ffmpeg**  | Command building, execution, retry | `builder.go`, `executor.go`, `errors.go`, `retry.go` |
| **pipeline**| File discovery, per-file processing, batch analysis, batch stats | `discover.go`, `runner.go`, `analyze.go`, `stats.go`, `pipeline_test.go` |

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
