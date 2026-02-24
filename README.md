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

---

## Quick start

```bash
# Preview what would be done (no encoding or remuxing)
muxmaster -d /media/library /out/library

# Encode with VAAPI (default), output to MKV
muxmaster /media/library /out/library

# Run system diagnostics (ffmpeg, ffprobe, VAAPI, x265, AAC)
muxmaster -c
```

---

## Installation and build

**Requirements:**

- **Go 1.26+**
- **ffmpeg** and **ffprobe** on `PATH` (required at runtime)
- For VAAPI encoding: a supported GPU and `/dev/dri/renderD*` device

**Build:**

```bash
git clone <repo-url>
cd Muxmaster
make build
```

This produces the `muxmaster` binary in the project root.

```bash
make fmt       # format all Go files
make vet       # run go vet
make ci        # vet + fmt + docs-naming + build + test (run before pushing)
make lint      # run golangci-lint (if installed)
make install   # installs to $(HOME)/bin
```

**Version and commit** are injected at build time via `make build` (see `Makefile`). Without `make`, use:

```bash
go build -o muxmaster ./cmd
```

---

## Usage

**Synopsis:**

```text
muxmaster [OPTIONS] <input_dir> <output_dir>
```

**Examples:**

```bash
# CPU encoding, fixed CRF 22
muxmaster -m cpu -q 22 /media/library /out/library

# Preview what would happen
muxmaster -d /media/library /out/library
```

**Option groups:**

- **Encoding:** `-m vaapi|cpu`, `-q <quality>`, `--cpu-crf`, `--vaapi-qp`, `-p <preset>`, `--container mkv|mp4`
- **Behavior:** `-d` dry-run, `-f` overwrite existing, `--strict` no retry fallbacks, `--no-skip-hevc` re-encode HEVC
- **Streams:** `--no-subs`, `--no-attachments`
- **Display:** `-v` verbose, `--no-color`, `-l <path>` log file
- **Utility:** `-c` check, `-V` version, `-h` help

Full list:

```bash
muxmaster -h
```

**Exit codes:**

- `0` — success (or partial success; batch continues on per-file failure)
- `1` — fatal error (e.g. bad args, missing ffmpeg, invalid paths)

---

## Project structure

```
cmd/             CLI entrypoint
internal/        All application logic (10 packages)
_docs/           Design docs, project reference, legacy artifacts
```

For the full package map, dependency direction, and "where to change what", see [_docs/project/structure.md](_docs/project/structure.md).

---

## Documentation

See [_docs/index.md](_docs/index.md) for the full doc index.

Key references:

- **Architecture and design:** [_docs/architecture.md](_docs/architecture.md)
- **Implementation reference:** [_docs/design/foundation-plan.md](_docs/design/foundation-plan.md)
- **Changelog:** [CHANGELOG.md](CHANGELOG.md)

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
