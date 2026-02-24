# Project audit

This document reviews the project against its structure, documentation, Git, build, and automation guidelines.

Last updated: 2026-02-23

---

## 1. Project structure

| Area | Status | Notes |
|------|--------|-------|
| Top-level layout | **Pass** | `cmd/`, `internal/`, `_docs/`, root meta-files. |
| `internal/` packages | **Pass** | 10 packages: config, term, logging, display, check, probe, naming, planner, ffmpeg, pipeline. |
| Stub consolidation | **Pass** | Unimplemented packages use a single `doc.go` with comprehensive implementation plan. |
| `.gitignore` | **Pass** | Ignores binary, coverage, IDE, OS files. `go.sum` is tracked (per Go modules spec). |
| `.editorconfig` | **Pass** | Cross-editor consistency for Go, Markdown, Makefile. |
| `LICENSE` | **Pass** | MIT license at root. |
| `go.mod` | **Pass** | Module path and Go version (`1.26`) are correct. |

### Internal package layout

- **config** — 3 files: `config.go` (types/defaults), `flags.go` (CLI), `config_test.go`.
- **term** — 1 file: `term.go` (ANSI colors, TTY detection).
- **logging** — 1 file: `logger.go`. Depends on config + term.
- **display** — 3 files: `banner.go`, `format.go`, `format_test.go`. Depends on term (not logging).
- **check** — 1 file: `check.go`.
- **probe**, **naming**, **planner**, **ffmpeg**, **pipeline** — 1 file each (`doc.go`).

---

## 2. Documentation

| Check | Status |
|-------|--------|
| README: overview, build, usage, structure, docs links, license | **Pass** |
| CHANGELOG: Keep a Changelog format, dated entries | **Pass** |
| CONTRIBUTING: setup, branching, commits, build, code style | **Pass** |
| Documentation entrypoint | **Pass** |
| _docs/architecture.md: dependency diagram includes `term` package | **Pass** |
| _docs/project/structure.md: matches actual code layout | **Pass** |

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
| `make ci` | **Pass** | Runs vet + fmt + build + test. |
| Unit tests | **Pass** | `config_test.go` (8 test functions), `format_test.go` (3 test functions). |
| golangci-lint config | **Pass** | `.golangci.yml` with 16 linters and project-specific exclusions. |

**Priority targets for additional tests:** `naming` (parser rules, output paths) and `check` (mock Logger).

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
| Documentation | Pass | Centralized in `README.md` + focused docs under `_docs/` (no separate docs index file) |
| Git practices | Pass | Create `dev` branch when ready |
| Build & testing | Pass | Add tests for naming, check when implemented |
| Naming | Pass | — |
