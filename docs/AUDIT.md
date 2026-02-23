# Project structure audit

This document reviews the current folder and file organization and recommends changes for clarity and maintainability.

---

## Current state

### Top level

| Item | Purpose |
|------|--------|
| `cmd/muxmaster/` | Single CLI entrypoint |
| `internal/` | Nine packages (config, logging, display, check, probe, naming, planner, ffmpeg, pipeline) |
| `docs/` | Design docs, legacy reference, structure and audit |
| `go.mod`, `Makefile`, `README.md` | Build and project metadata |

### Internal layout

- **config** — 2 files (`config.go`, `flags.go`). Clear split: types/defaults vs CLI.
- **logging** — 1 file. Fine as-is.
- **display** — 4 files (`banner.go`, `format.go`, `renderplan.go`, `outlier.go`); two are stubs.
- **check** — 1 file. Single responsibility.
- **probe** — 4 stub files (`types.go`, `prober.go`, `hdr.go`, `interlace.go`).
- **naming** — 6 stub files (parser, rules, postprocess, outputpath, collision, harmonize).
- **planner** — 7 stub files (planner, quality, estimation, audio, subtitle, filter, disposition).
- **ffmpeg** — 4 stub files (builder, executor, errors, retry).
- **pipeline** — 3 stub files (runner, discover, stats).

### Gaps observed

- No `.gitignore` — build artifact (`muxmaster` binary) and common IDE/editor files can be committed by mistake.
- `docs/` had no index — newcomers had to guess which doc to open.
- Naming inconsistency: `Muxmaster2_0.md` vs `muxmaster-go-foundation-plan-final.md` (case and style).
- README was minimal; no changelog.

---

## Recommendations

### 1. Add `.gitignore`

Ignore at least:

- The built binary (`muxmaster`, `muxmaster.exe`).
- Go workspace and IDE files (e.g. `*.swp`, `.idea/`, `.vscode/` if local).
- Log/test outputs if any are written to the tree.

This keeps the repo clean and avoids accidental commits of binaries.

### 2. Keep current package layout

The nine-package `internal/` layout matches the foundation plan and gives a clear place for each concern. **No structural change recommended.** Optional tweaks:

- **display**: Four files are manageable; keep `renderplan.go` and `outlier.go` as separate files for when they are implemented.
- **check**: Keeping it as its own package is better than folding into `cmd`; it preserves a single place for diagnostics and `CheckDeps`.

### 3. Standardize doc names and add an index

- Introduce **`docs/INDEX.md`** (or `docs/README.md`) as the table of contents for the docs folder so navigation is obvious.
- Use **`core-design.md`** for core design concepts (summary + link to the full foundation plan).
- Use **`final-design.md`** as the entry point for the final product specification (link to or mirror the 2.0 spec). This gives a consistent naming scheme: `core-design` + `final-design` + index.

### 4. README and CHANGELOG

- **README.md**: Expand with a short project overview, installation/build steps, and a basic usage guide. Link to `docs/INDEX.md` and key design docs.
- **CHANGELOG.md**: Add an initial version entry (e.g. 2.0.0-dev or 0.1.0) so version history has a clear starting point.

### 5. Formatting and conventions

- **Go**: Keep using `gofmt` and `go vet`; no extra formatting rules needed.
- **Markdown**: Use consistent headers and list formatting in docs; the new INDEX and reorganized docs should follow the same style as existing design docs.
- **File naming**: Prefer lowercase-with-hyphens for doc filenames (e.g. `core-design.md`, `final-design.md`) for consistency with `muxmaster-go-foundation-plan-final.md`.

### 6. Optional later improvements

- **Rename `Muxmaster2_0.md`** to `final-design.md` and update references if you want a single canonical filename; otherwise, keep both and have `final-design.md` point to it.
- **Version in one place**: Document in README or CONTRIBUTING how version/commit are set (e.g. Makefile `-ldflags` and where to bump versions).

---

## Summary

| Area | Action |
|------|--------|
| Repo root | Add `.gitignore`; expand README; add CHANGELOG. |
| `internal/` | No structural change; keep current packages and files. |
| `docs/` | Add INDEX; add `core-design.md` and `final-design.md`; optional rename of 2.0 spec. |
| Conventions | Use lowercase-with-hyphens for new doc filenames; keep Go tooling as-is. |

These changes improve clarity and maintainability without altering the core architecture.
