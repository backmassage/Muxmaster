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

- **config** — 2 files: `config.go` (types/defaults), `flags.go` (CLI).
- **term** — 1 file: `term.go` (ANSI colors, TTY detection).
- **logging** — 1 file: `logger.go`. Depends on config + term.
- **display** — 2 files: `banner.go`, `format.go`. Depends on term (not logging).
- **check** — 1 file: `check.go`.
- **probe**, **naming**, **planner**, **ffmpeg**, **pipeline** — 1 file each (`doc.go`).

---

## 2. Documentation

| Check | Status |
|-------|--------|
| README: overview, build, usage, structure, docs links, license | **Pass** |
| CHANGELOG: Keep a Changelog format, dated entries | **Pass** |
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
| `make ci` | **Pass** | Runs vet + fmt + docs-naming + build + test. |
| Unit tests | **None** | No test files. Add when implementing stub packages. |
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
| Build & testing | Pass | Add tests when implementing stub packages |
| Naming | Pass | — |
