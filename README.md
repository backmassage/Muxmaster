# Muxmaster Media Library Encoder

> A fast, resilient batch encoder/remuxer for Jellyfin-style libraries.

Current version: **1.0**

## At a Glance

- HEVC video encoding via VAAPI or CPU/x265
- Strict AAC audio processing for all tracks (224k target)
- HEVC remux mode by default (copy video + process audio)
- Clean container metadata/chapters by default
- Automatic TV/movie folder structure normalization
- Stream-safe defaults for subtitles and attachment fonts

The script is designed to handle mixed anime/TV/movie files, including dual-audio releases, ASS subtitles, and attachment fonts.

## Mini PC Profile (Tuned/Tested)

This project is actively tuned and validated on:

| Component | Spec |
|---|---|
| CPU | AMD Ryzen 6600H |
| RAM | 20 GB |
| OS | Arch Linux |

---

## Repository Structure (WIP)

```text
.
├── Muxmaster.sh
└── scripts/
    └── helpers/
        ├── harleybox_auto.sh
        └── extra/
            └── .gitkeep
```

Use `scripts/helpers/` for helper `.sh` utilities.

---

## Features

- **Encoder modes**
  - `vaapi` (default): hardware HEVC encode via VAAPI
  - `cpu`: software HEVC encode via `libx265`
- **10-bit first**
  - VAAPI probes main10 first and falls back to main (8-bit) if needed.
- **Audio handling**
  - Tries to convert **all audio tracks** to AAC stereo 224k.
  - If AAC fails for a file, that file is marked as failed (**no audio-copy fallback**).
- **Subtitle handling**
  - Copies subtitle streams by default (`-c:s copy`), so **ASS remains ASS**.
  - If subtitle mux/copy fails, the file is retried without subtitles.
- **Attachment handling**
  - Copies attachment streams by default (fonts/images), which helps ASS styling render correctly.
  - If an input attachment stream is missing required tags (filename/mimetype), the file is retried without attachments.
- **HEVC skip mode**
  - Default behavior remuxes HEVC sources (copy video + process audio).
  - Use `--no-skip-hevc` to force HEVC re-encode.
- **Metadata handling**
  - Default behavior strips container metadata and chapters for cleaner outputs.
  - Use `--keep-metadata` to preserve source container metadata/chapters.
  - If preserve mode fails for a file, that file is retried with clean metadata.
- **Mux/timestamp resilience**
  - Retries with a larger mux queue if FFmpeg reports packet buffer overflow.
  - Retries with generated timestamps when FFmpeg reports non-monotonic DTS.
  - Use `--strict` to disable all automatic per-file retry fallbacks.
- **Safer stream selection**
  - Ignores attached-pic video streams when choosing the main video stream.
- **Readable CLI output**
  - Live FFmpeg FPS/speed progress by default, detailed FFmpeg output with `-v`.
- **Color support**
  - Auto color in TTY, plus `--color` / `--no-color`.
- **File stats section**
  - Shows source video resolution and bitrate for each file.

---

## Requirements

- Bash
- `ffmpeg`
- `ffprobe`
- For VAAPI mode:
  - VAAPI-capable system
  - A render node like `/dev/dri/renderD128`
  - FFmpeg build with `hevc_vaapi`

---

## Quick Start

```bash
chmod +x Muxmaster.sh
./Muxmaster.sh -m vaapi -q 19 "/path/to/input" "/path/to/output"
```

Typical anime/dual-audio remux workflow:

```bash
./Muxmaster.sh -m vaapi -q 19 "/srv/jellyfin/Media/Output" "/mnt/HarleyBox/Anime"
```

### Quick Command Cheat Sheet

| Goal | Command |
|---|---|
| Standard encode pass (HEVC remux default) | `./Muxmaster.sh -m vaapi "/input" "/output"` |
| CPU encode | `./Muxmaster.sh -m cpu "/input" "/output"` |
| Force HEVC re-encode | `./Muxmaster.sh --no-skip-hevc "/input" "/output"` |
| Preserve source metadata/chapters | `./Muxmaster.sh --keep-metadata "/input" "/output"` |
| Disable automatic retry fallbacks | `./Muxmaster.sh --strict "/input" "/output"` |
| Disable live FPS/speed output | `./Muxmaster.sh --no-fps "/input" "/output"` |
| Dry-run plan only | `./Muxmaster.sh -d "/input" "/output"` |
| System diagnostics | `./Muxmaster.sh --check` |

---

## Command Usage

```text
Muxmaster.sh [OPTIONS] <input_dir> <output_dir>
```

### Options

| Option | Description |
|---|---|
| `-m, --mode <vaapi|cpu>` | Encoder mode (default: `vaapi`) |
| `-q, --quality <value>` | VAAPI QP or CPU CRF (default: `19`) |
| `-p, --preset <preset>` | CPU preset for x265 (default: `slow`) |
| `-d, --dry-run` | Preview planned operations only |
| `--skip-hevc` | HEVC files: copy video, process audio (default behavior) |
| `--no-skip-hevc` | Re-encode HEVC video instead of remuxing it |
| `--clean-metadata` | Strip container metadata and chapters (default behavior) |
| `--keep-metadata` | Preserve source container metadata and chapters |
| `--show-fps` | Show live FFmpeg encoding FPS/speed progress (default: on) |
| `--no-fps` | Disable live FFmpeg FPS/speed progress |
| `--no-stats` | Hide per-file source video stats (resolution/bitrate) |
| `--no-subs` | Do not copy subtitle streams |
| `--no-attachments` | Do not copy attachment streams |
| `--strict` | Disable automatic FFmpeg retry fallbacks (fail fast per file) |
| `-f, --force` | Overwrite existing output files |
| `-l, --log <path>` | Write plain logs to a file |
| `--` | End options parsing (use before paths starting with `-`) |
| `--color` | Force colored logs |
| `--no-color` | Disable colored logs |
| `-v, --verbose` | Verbose mode (includes FFmpeg details/progress) |
| `-c, --check` | Run dependency/system checks only |
| `-V, --version` | Print script version and exit |
| `-h, --help` | Show help |

---

## Defaults and Behavior

- Output container: **MKV**
- Keyframe interval: **48**
- Audio target: **AAC stereo 224k** (all tracks)
- If AAC fails on a file: file processing fails (**no audio-copy fallback**)
- Subtitles: copied by default (ASS and others preserved)
- Attachments: copied by default
- Container metadata/chapters: stripped by default (`--keep-metadata` to preserve)
- FFmpeg FPS/speed live progress is on by default (`--no-fps` to disable)
- Per-file source video stats are shown by default (`--no-stats` to hide)
- Existing output files: skipped by default (`--force` to overwrite)
- HEVC sources: remux by default (`--no-skip-hevc` to force HEVC re-encode)
- Automatic FFmpeg fallback retries are enabled by default (`--strict` disables them)

Supported input extensions:

- `mkv`, `mp4`, `avi`, `m4v`, `mov`, `wmv`, `flv`, `webm`, `ts`, `m2ts`

---

## Output Structure

The script attempts to classify files as TV episodes or movies from filename patterns.

### TV output

```text
<output>/<Show Name>/Season <NN>/<Show Name> - S<NN>E<NN>.mkv
```

### Movie output

```text
<output>/<Movie Name (Year)>/<Movie Name (Year)>.mkv
```

---

## Examples

### VAAPI encode

```bash
./Muxmaster.sh -m vaapi -q 19 "/media/input" "/media/output"
```

### CPU encode

```bash
./Muxmaster.sh -m cpu -q 20 -p medium "/media/input" "/media/output"
```

### Keep existing outputs untouched (default)

```bash
./Muxmaster.sh "/media/input" "/media/output"
```

### Force overwrite existing outputs

```bash
./Muxmaster.sh -f "/media/input" "/media/output"
```

### Disable subtitle and attachment copying

```bash
./Muxmaster.sh --no-subs --no-attachments "/media/input" "/media/output"
```

### Verbose FFmpeg diagnostics

```bash
./Muxmaster.sh -v -m cpu "/media/input" "/media/output"
```

### Show live FFmpeg FPS/speed progress (default behavior)

```bash
./Muxmaster.sh --show-fps -m cpu "/media/input" "/media/output"
```

### Disable live FFmpeg FPS/speed progress

```bash
./Muxmaster.sh --no-fps -m cpu "/media/input" "/media/output"
```

### System diagnostics only

```bash
./Muxmaster.sh --check
```

---

## Troubleshooting

### Need hard-fail behavior for debugging

- Use `--strict` to disable all automatic per-file retry fallbacks.
- This is useful when you want the first FFmpeg failure to be the only failure shown.

### "No VAAPI device"

- Check `/dev/dri/renderD*` exists.
- Ensure user permissions allow access to render device.
- Try CPU mode to confirm pipeline: `-m cpu`.

### Attachment warnings like "Could not find codec parameters for stream ... Attachment: none"

- These often come from odd font attachments in MKVs.
- The script maps only needed streams and should still proceed in most cases.

### Attachment tag errors like "Attachment stream ... has no filename/mimetype tag"

- Newer script versions automatically retry the file without attachments.
- If you still need a manual override for a run, use `--no-attachments`.

### Subtitle mux/copy errors

- Newer script versions automatically retry the file without subtitles if subtitle stream muxing fails.
- Manual override: run with `--no-subs`.

### "Too many packets buffered for output stream"

- Newer script versions automatically retry the file with a larger FFmpeg mux queue.
- If issues persist, run with `-v` to inspect the full FFmpeg stream mapping and packet flow.

### Non-monotonic DTS / timestamp ordering errors

- Newer script versions automatically retry with generated timestamps.
- For badly damaged sources, remuxing the source once with FFmpeg may still be required.

### Audio issues on specific files

- Run with `-v` to capture detailed FFmpeg output.
- AAC is strict for all tracks; unsupported inputs will fail for that file.

### Output already exists and file is skipped

- Use `-f` / `--force` to overwrite.

---

## Notes

- Logs printed with colors are for terminal readability; log files remain plain text.
- For large libraries, start with a small subset or `--dry-run` first.

