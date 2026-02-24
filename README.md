# Muxmaster

Jellyfin-optimized media encoder: batch HEVC/AAC encoding and remuxing with smart per-file quality, automatic retry, and deterministic Jellyfin-friendly output naming.

**Version:** 2.0.0 (Go rewrite — full CLI parity with the legacy v1.7.0 shell script)

---

## What it does

Muxmaster is a **single-run CLI** that processes an entire media library in one pass:

1. **Discovers** all video files recursively (14 container formats, skips `extras` directories)
2. **Probes** each file with a single `ffprobe` JSON call — resolution, codec, HDR, interlace, bitrate
3. **Plans** the encoding: smart per-file QP/CRF from resolution and bitrate curves, or fixed quality override
4. **Decides** per file: **encode** (VAAPI hardware or CPU libx265 → HEVC), **remux** (copy edge-safe HEVC video, encode audio), or **skip** (already processed)
5. **Executes** `ffmpeg` with automatic retry on failure (attachment, subtitle, mux queue, and timestamp fixes)
6. **Names** outputs for Jellyfin: `Show/Season 01/Show - S01E01.mkv` for TV, `Movie (2024)/Movie (2024).mkv` for movies

It is an orchestration layer over `ffmpeg` and `ffprobe`, not a codec implementation. The Go rewrite replaces a 2,600-line Bash script with a single static binary.

---

## Quick start

```bash
# Preview what would be done (no files written)
muxmaster --dry-run /media/library /out/library

# Encode with VAAPI hardware acceleration (default)
muxmaster /media/library /out/library

# Encode with CPU (libx265)
muxmaster --mode cpu /media/library /out/library

# Run system diagnostics (ffmpeg, ffprobe, VAAPI, x265, libfdk_aac)
muxmaster --check
```

---

## Installation

### Requirements

- **Go 1.26+** (build only)
- **ffmpeg** and **ffprobe** on `PATH` (required at runtime)
- For VAAPI encoding: a supported GPU with a `/dev/dri/renderD*` device

### Build from source

```bash
git clone <repo-url>
cd Muxmaster
make build        # produces ./muxmaster with version/commit baked in
```

Or without Make:

```bash
go build -o muxmaster ./cmd
```

### Install

```bash
make install      # copies to ~/bin/muxmaster
```

### Other Make targets

```bash
make test         # run all tests
make vet          # go vet
make fmt          # gofmt
make lint         # golangci-lint (if installed)
make coverage     # generate HTML coverage report
make ci           # vet + fmt + docs-naming + build + test
make clean        # remove binary and coverage files
```

---

## Usage

```text
muxmaster [OPTIONS] <input_dir> <output_dir>
```

### Common examples

```bash
# Dry-run: see what would happen without encoding anything
muxmaster -d /media/anime /out/anime

# VAAPI encode (default), MKV output (default)
muxmaster /media/library /out/library

# CPU encode with fixed CRF 22
muxmaster -m cpu -q 22 /media/library /out/library

# MP4 output with HDR tonemapping to SDR
muxmaster --container mp4 --hdr tonemap /media/library /out/library

# Force re-encode HEVC sources instead of remuxing
muxmaster --no-skip-hevc /media/library /out/library

# Verbose output with log file
muxmaster -v -l encode.log /media/library /out/library

# Fixed VAAPI QP (disables smart quality for this value)
muxmaster --vaapi-qp 21 /media/library /out/library

# Override AAC bitrate for non-AAC transcodes
muxmaster --audio-bitrate 192k /media/library /out/library
```

### Full option reference

**Encoding**

| Flag | Description | Default |
|------|-------------|---------|
| `-m, --mode <vaapi\|cpu>` | Encoder backend | `vaapi` |
| `-q, --quality <value>` | Fixed QP (VAAPI) or CRF (CPU) | smart per-file |
| `--vaapi-qp <value>` | Fixed VAAPI QP (overrides `--quality`) | 19 |
| `--cpu-crf <value>` | Fixed CPU CRF (overrides `--quality`) | 19 |
| `-p, --preset <name>` | x265 CPU preset | `slow` |
| `--audio-bitrate <rate>` | AAC bitrate for non-AAC audio transcodes (e.g. `128k`, `256k`) | `256k` |

**Container & HDR**

| Flag | Description | Default |
|------|-------------|---------|
| `--container <mkv\|mp4>` | Output container format | `mkv` |
| `--hdr <preserve\|tonemap>` | HDR handling strategy | `preserve` |
| `--no-deinterlace` | Disable automatic yadif deinterlacing | auto-detect on |

**Streams**

| Flag | Description | Default |
|------|-------------|---------|
| `--no-skip-hevc` | Re-encode HEVC video instead of remuxing | remux edge-safe HEVC |
| `--no-subs` | Strip all subtitle streams | keep subtitles |
| `--no-attachments` | Strip attachments (fonts, images) | keep attachments |

**Output & behavior**

| Flag | Description | Default |
|------|-------------|---------|
| `-d, --dry-run` | Preview only; no files written | off |
| `-f, --force` | Overwrite existing output files | skip existing |
| `--strict` | Disable automatic ffmpeg retry | retry enabled |
| `--smart-quality` / `--no-smart-quality` | Per-file quality adaptation | on |
| `--clean-timestamps` / `--no-clean-timestamps` | Regenerate PTS/DTS | on |
| `--match-audio-layout` / `--no-match-audio-layout` | Normalize audio channel layout | on |

**Display**

| Flag | Description | Default |
|------|-------------|---------|
| `-v, --verbose` | Show debug output and full ffmpeg logs | off |
| `--show-fps` / `--no-fps` | Show live ffmpeg encoding FPS | on |
| `--no-stats` | Hide per-file source stats | stats on |
| `--color` / `--no-color` | Force or disable ANSI colors | auto (TTY) |
| `-l, --log <path>` | Append plain-text logs to file | none |

**Utility**

| Flag | Description |
|------|-------------|
| `-c, --check` | Run system diagnostics and exit |
| `-V, --version` | Print version and exit |
| `-h, --help` | Show help and exit |

### Exit codes

- `0` — all files processed successfully (or dry-run)
- `1` — one or more files failed, or a fatal error occurred (bad args, missing ffmpeg, invalid paths)

---

## How it works

### Encoding pipeline

For each discovered media file:

```
Validate → Probe → Parse filename → Resolve output path → Plan → Execute → Report
```

- **Validate**: skip files under 1 KB (likely corrupt)
- **Probe**: single `ffprobe -print_format json` call extracts codec, resolution, bitrate, HDR, interlace, and stream info
- **Parse filename**: 14 regex rules extract show name, season, episode, or movie title and year
- **Plan**: smart quality selects QP/CRF per-file based on resolution and bitrate curves; decides encode vs remux based on HEVC edge-safety (profile + pix_fmt)
- **Execute**: runs ffmpeg with automatic retry (up to 4 attempts) for attachment errors, subtitle mux issues, queue overflow, and timestamp discontinuities
- **Quality retry**: if output exceeds 105% of input size, re-encodes with tighter quality settings

### Audio handling

- AAC streams are always copied (no lossy-to-lossy re-encode)
- Non-AAC streams are transcoded to AAC via `libfdk_aac` at configured bitrate (`--audio-bitrate`, default `256k`), 48 kHz, up to 2 channels
- Optional channel layout normalization (`--match-audio-layout`)

### Subtitle and attachment handling

- **MKV**: subtitles copied, attachments (fonts) preserved
- **MP4**: text subtitles converted to `mov_text`, bitmap subtitles skipped, attachments not supported

### Output naming

| Type | Pattern |
|------|---------|
| TV show | `<Show>/Season 01/<Show> - S01E01.mkv` |
| Movie | `<Name> (<Year>)/<Name> (<Year>).mkv` |
| Specials | `<Show>/Season 00/<Show> - S00E101.mkv` (OP/ED/PV) |

Collision resolution appends ` - dup1`, ` - dup2`, etc. TV show names with year tags are harmonized across the batch.

---

## Project structure

```
cmd/             CLI entrypoint
internal/        All application logic (10 packages)
  config/        Defaults, CLI flags, validation
  term/          ANSI color state, TTY detection
  logging/       Leveled logger, optional file sink
  display/       Banner, byte/bitrate formatting
  check/         --check diagnostics, pre-pipeline dep validation
  probe/         ffprobe JSON parsing, HDR/interlace/HEVC detection
  naming/        Filename parser, output paths, collision, harmonization
  planner/       Per-file quality, estimation, filters, audio/subtitle plans
  ffmpeg/        Command builder, executor, error patterns, retry state
  pipeline/      File discovery, per-file orchestration, batch stats
_docs/           Design docs, project reference, legacy artifacts
```

For the full dependency map and architecture, see [_docs/architecture.md](_docs/architecture.md).

---

## Documentation

See [_docs/index.md](_docs/index.md) for the full doc index.

Key references:

- **Architecture:** [_docs/architecture.md](_docs/architecture.md)
- **Design plan:** [_docs/design/foundation-plan.md](_docs/design/foundation-plan.md)
- **Project structure:** [_docs/project/structure.md](_docs/project/structure.md)
- **Changelog:** [CHANGELOG.md](CHANGELOG.md)

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
