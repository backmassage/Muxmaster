# Project audit

This document reviews the project against its structure, documentation, Git, build, and automation guidelines.

Last updated: 2026-02-24

---

## 1. Project structure

| Area | Status | Notes |
|------|--------|-------|
| Top-level layout | **Pass** | `cmd/`, `internal/`, `_docs/`, root meta-files. |
| `internal/` packages | **Pass** | 10 packages: config, term, logging, display, check, probe, naming, planner, ffmpeg, pipeline. |
| All packages implemented | **Pass** | No stubs remain; every package has full production code. |
| `.gitignore` | **Pass** | Ignores binary, coverage, IDE, OS files. `go.sum` is tracked (per Go modules spec). |
| `.editorconfig` | **Pass** | Cross-editor consistency for Go, Markdown, Makefile. |
| `LICENSE` | **Pass** | MIT license at root. |
| `go.mod` | **Pass** | Module path and Go version (`1.26`) are correct. |

### Internal package layout

- **config** — 3 files: `config.go` (types/defaults), `flags.go` (CLI), `config_test.go`.
- **term** — 1 file: `term.go` (ANSI colors incl. Bold/Dim, TTY detection).
- **logging** — 1 file: `logger.go`. Depends on config + term.
- **display** — 2 files: `banner.go`, `format.go`. Depends on term (not logging).
- **check** — 1 file: `check.go`. Depends on config; accepts Logger interface.
- **probe** — 5 files: `types.go`, `prober.go`, `hdr.go`, `interlace.go`, `probe_test.go` + `probe_live_test.go`. No internal deps.
- **naming** — 7 files: `parser.go`, `rules.go`, `postprocess.go`, `outputpath.go`, `collision.go`, `harmonize.go`, `parser_test.go`. No internal deps.
- **planner** — 9 files: `types.go`, `planner.go`, `quality.go`, `estimation.go`, `filter.go`, `audio.go`, `subtitle.go`, `disposition.go`, `doc.go`, `planner_test.go`. Depends on config + probe.
- **ffmpeg** — 4 files: `builder.go`, `executor.go`, `errors.go`, `retry.go`. Depends on config + planner.
- **pipeline** — 5 files: `discover.go`, `runner.go`, `analyze.go`, `stats.go`, `pipeline_test.go`. Depends on config, logging, probe, naming, planner, ffmpeg, display, term.

---

## 2. Documentation

| Check | Status |
|-------|--------|
| README: overview, build, usage, structure, docs links, license | **Pass** |
| CHANGELOG: Keep a Changelog format, dated entries | **Pass** |
| Documentation entrypoint (`_docs/index.md`) | **Pass** |
| `_docs/architecture.md`: dependency diagram matches actual imports | **Pass** |
| `_docs/project/structure.md`: matches actual code layout | **Pass** |

---

## 3. Git practices

| Convention | Expected |
|------------|----------|
| Stable | `main` |
| Development | `dev` |
| Feature | `feature/<short-description>` |
| Bugfix | `bugfix/<short-description>` |
| Hotfix | `hotfix/<short-description>` |
| Commit format | `type: short description` |
| Release tags | `vX.Y.Z` |

---

## 4. Build and testing

| Check | Status | Notes |
|-------|--------|-------|
| Makefile targets | **Pass** | build, test, vet, fmt, lint, docs-naming, coverage, ci, clean, install. |
| `make ci` | **Pass** | Runs vet + fmt + docs-naming + build + test. |
| Unit tests | **Pass** | Test suites for config, probe, naming, planner, and pipeline (discovery + dry-run). |
| golangci-lint config | **Pass** | `.golangci.yml` with 19 linters and project-specific exclusions. |

---

## 5. Naming conventions

| Area | Convention | Status |
|------|-----------|--------|
| Go packages | Lowercase single-word | **Pass** |
| Go files | Lowercase (`config.go`, `doc.go`) | **Pass** |
| Root meta-files | UPPERCASE (`README.md`, `LICENSE`) | **Pass** |
| Docs under `_docs/` | Lowercase-with-hyphens | **Pass** |

---

## Summary

| Category | Status | Open items |
|----------|--------|------------|
| Project structure | Pass | — |
| Documentation | Pass | — |
| Git practices | Pass | Create `dev` branch when ready |
| Build & testing | Pass | — |
| Naming | Pass | — |
