# Muxmaster

Jellyfin-optimized media encoder: HEVC/AAC encoding and remuxing with deterministic, Jellyfin-friendly output naming.

**Version:** 2.0 (Go rewrite; target is full CLI parity with the legacy v1.7.0 shell script.)

---

## Project overview

Muxmaster is a **single-run CLI** that:

- Scans a media directory and discovers video files
- Probes each file with **ffprobe** (one JSON call per file)
- Decides whether to **encode** (VAAPI or CPU HEVC), **remux** (copy video, process audio), or **skip**
- Writes outputs under a target directory with consistent naming (TV: `Show/Season XX/Show - SXXEXX.mkv`; movies: `Name (Year)/Name.mkv`)
- Supports dry-run, system diagnostics (`--check`), and configurable quality/container/stream options

It is an **orchestration layer** over `ffmpeg` and `ffprobe`, not a codec implementation. The Go rewrite replaces a large Bash script with a single static binary and clear package boundaries.

- **Design and layout:** See [docs/INDEX.md](docs/INDEX.md) for the documentation index and [docs/PROJECT_STRUCTURE.md](docs/PROJECT_STRUCTURE.md) for where to change things in the codebase.

---

## Installation / build

**Requirements**

- **Go 1.26+**
- **ffmpeg** and **ffprobe** on `PATH` (required at runtime)
- For VAAPI encoding: a supported GPU and `/dev/dri/renderD*` device

**Build**

```bash
git clone <repo-url>
cd Muxmaster
make build
```

This produces the `muxmaster` binary in the project root.

```bash
make test      # run tests (go test ./...)
make install   # installs to $(HOME)/bin
```

**Version and commit** are injected at build time via `make build` (see `Makefile`). Without `make`, use:

```bash
go build -o muxmaster ./cmd/muxmaster
```

---

## Basic usage

**Synopsis**

```text
muxmaster [OPTIONS] <input_dir> <output_dir>
```

**Examples**

```bash
# Preview what would be done (no encoding or remuxing)
muxmaster -d /media/library /out/library

# Encode with VAAPI (default), output to MKV
muxmaster /media/library /out/library

# CPU encoding, fixed CRF 22
muxmaster -m cpu -q 22 /media/library /out/library

# Run system diagnostics (ffmpeg, ffprobe, VAAPI, x265, AAC)
muxmaster -c
```

**Option groups**

- **Encoding:** `-m vaapi|cpu`, `-q <quality>`, `--cpu-crf`, `--vaapi-qp`, `-p <preset>`, `--container mkv|mp4`
- **Behavior:** `-d` dry-run, `-f` overwrite existing, `--strict` no retry fallbacks, `--no-skip-hevc` re-encode HEVC
- **Streams:** `--no-subs`, `--no-attachments`
- **Display:** `-v` verbose, `--no-color`, `-l <path>` log file
- **Utility:** `-c` check, `-V` version, `-h` help

Full list:

```bash
muxmaster -h
```

**Exit codes**

- `0` — success (or partial success; batch continues on per-file failure)
- `1` — fatal error (e.g. bad args, missing ffmpeg, invalid paths)

---

## Design and docs

- **Documentation index:** [docs/INDEX.md](docs/INDEX.md)
- **Project structure and “where to change what”:** [docs/PROJECT_STRUCTURE.md](docs/PROJECT_STRUCTURE.md)
- **Structure audit and recommendations:** [docs/AUDIT.md](docs/AUDIT.md)
- **Guidelines compliance audit:** [docs/GUIDELINES_AUDIT.md](docs/GUIDELINES_AUDIT.md)
- **Changelog:** [CHANGELOG.md](CHANGELOG.md)
- **Git (branching, commits, releases):** [GIT_GUIDELINES.md](GIT_GUIDELINES.md)
