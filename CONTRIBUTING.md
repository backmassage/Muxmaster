# Contributing to Muxmaster

Thanks for your interest in contributing. This guide covers setup, workflow, and conventions.

---

## Development environment

**Requirements:**

- Go 1.23+ (see `go.mod`)
- `ffmpeg` and `ffprobe` on `PATH` (runtime dependency)
- For VAAPI encoding: a supported GPU and `/dev/dri/renderD*`

**Setup:**

```bash
git clone <repo-url>
cd Muxmaster
git checkout dev          # all work happens on dev, not main
make build                # verify the build works
make test                 # run tests (go test ./...)
```

---

## Branching and commits

Full details are in [_docs/project/git-guidelines.md](_docs/project/git-guidelines.md). The short version:

**Branches:**

- `main` — stable, release-ready. Never push directly.
- `dev` — ongoing development. Default branch for daily work.
- `feature/<short-name>` — new features, branched from `dev`.
- `bugfix/<short-name>` — bug fixes, branched from `dev`.
- `hotfix/<short-name>` — urgent fixes, branched from `main`.

**Commit messages** use the format `type: short description`:

```
feat: add VAAPI encode path
fix: normalize output path with trailing slash
docs: update architecture diagram
chore: bump Go version in go.mod
refactor: split flags into encoding and behavior groups
test: add table-driven tests for naming parser
```

Keep descriptions lowercase (except proper names), no trailing period.

---

## Building and testing

```bash
make build    # compile binary with version/commit ldflags
make test     # run all tests
make vet      # run go vet
make fmt      # format all Go files (gofmt -l -w .)
make docs-naming # validate Markdown filename convention
make ci       # vet + fmt + build + test (run before pushing)
make lint     # run golangci-lint (if installed)
make coverage # generate coverage report → coverage.html
make clean    # remove build artifacts
```

Always run `make ci` before pushing to verify nothing is broken.

---

## Code style

- **Formatting:** `gofmt` is the only formatting rule. All Go code must pass `gofmt`.
- **Linting:** `go vet` is required; `golangci-lint` is recommended.
- **Markdown filenames:** Use lowercase kebab-case for docs under subdirectories (example: `foundation-plan.md`). Root exceptions are `README.md`, `CHANGELOG.md`, and `CONTRIBUTING.md`. Validate with `make docs-naming`.
- **Tests:** Place `*_test.go` files alongside the code they test. Use table-driven tests where appropriate.
- **Packages:** All application logic lives under `internal/`. See [_docs/project/structure.md](_docs/project/structure.md) for the package map and dependency direction.

---

## Project layout

Before implementing or modifying a package, read these docs for context:

- [_docs/project/structure.md](_docs/project/structure.md) — package layout, dependency direction, "where to change what"
- [_docs/architecture.md](_docs/architecture.md) — package dependency diagram and per-file flow
- [_docs/design/core-design.md](_docs/design/core-design.md) — architecture goals and technical choices
- [_docs/design/foundation-plan.md](_docs/design/foundation-plan.md) — full implementation reference with types, phases, and gotchas

---

## Adding a new internal package

1. Create a directory under `internal/` with a lowercase single-word name.
2. Add a package-level doc comment in the primary `.go` file explaining the package's responsibility.
3. Update [_docs/project/structure.md](_docs/project/structure.md) with the new package, its role, and its dependency direction.
4. Ensure the package does not introduce circular dependencies — leaf packages (`config`, `term`, `naming`, `probe`) must remain dependency-free or near-leaf.

---

## Version management

The version is set in the `Makefile` via `-ldflags` and injected at build time. The `main.go` hardcoded value (`2.0.0-dev`) is a fallback for builds done with plain `go build` (without `make`).

When preparing a release:

1. Update `VERSION` in `Makefile`.
2. Add a new section to `CHANGELOG.md` with the version, date, and changes.
3. Merge `dev` → `main`.
4. Tag: `git tag vX.Y.Z && git push origin vX.Y.Z`.
