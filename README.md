# Jellyfin Media Library Encoder

Batch-convert media for a Jellyfin-style library with:

- HEVC video (VAAPI or CPU/x265)
- AAC audio (all tracks by default)
- Optional HEVC remux mode (copy video, process audio)
- Clean output folder structure for TV and movies

The script is designed to be resilient with mixed anime/TV/movie files, including dual-audio releases, ASS subtitles, and attachment fonts.

---

## Features

- **Encoder modes**
  - `vaapi` (default): hardware HEVC encode via VAAPI
  - `cpu`: software HEVC encode via `libx265`
- **10-bit first**
  - VAAPI probes main10 first and falls back to main (8-bit) if needed.
- **Audio handling**
  - Tries to convert **all audio tracks** to AAC stereo 214k.
  - If AAC fails for a file, that file is marked as failed (**no audio-copy fallback**).
- **Subtitle handling**
  - Copies subtitle streams by default (`-c:s copy`), so **ASS remains ASS**.
- **Attachment handling**
  - Copies attachment streams by default (fonts/images), which helps ASS styling render correctly.
- **HEVC skip mode**
  - `--skip-hevc`: if the source is already HEVC, copy video and process audio only.
- **Safer stream selection**
  - Ignores attached-pic video streams when choosing the main video stream.
- **Readable CLI output**
  - Quiet FFmpeg output by default, detailed FFmpeg output with `-v`.
- **Color support**
  - Auto color in TTY, plus `--color` / `--no-color`.
- **CSV result reporting**
  - Writes per-file results (encoded/remuxed/skipped/failed) with rename tracking.

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
chmod +x jellyfin-encode.sh
./jellyfin-encode.sh -m vaapi -q 19 "/path/to/input" "/path/to/output"
```

Typical anime/dual-audio remux workflow:

```bash
./jellyfin-encode.sh -m vaapi --skip-hevc -q 19 "/srv/jellyfin/Media/Output" "/mnt/HarleyBox/Anime"
```

---

## Command Usage

```text
jellyfin-encode.sh [OPTIONS] <input_dir> <output_dir>
```

### Options

| Option | Description |
|---|---|
| `-m, --mode <vaapi|cpu>` | Encoder mode (default: `vaapi`) |
| `-q, --quality <value>` | VAAPI QP or CPU CRF (default: `19`) |
| `-p, --preset <preset>` | CPU preset for x265 (default: `slow`) |
| `-d, --dry-run` | Preview planned operations only |
| `--skip-hevc` | HEVC files: copy video, process audio |
| `--include-extras` | Include files from `NC`/`Extras`/`Sample` folders |
| `--no-subs` | Do not copy subtitle streams |
| `--no-attachments` | Do not copy attachment streams |
| `-f, --force` | Overwrite existing output files |
| `-l, --log <path>` | Write plain logs to a file |
| `--csv-log <path>` | Write per-file results CSV to a custom path |
| `--no-csv-log` | Disable CSV result logging |
| `--` | End options parsing (use before paths starting with `-`) |
| `--color` | Force colored logs |
| `--no-color` | Disable colored logs |
| `-v, --verbose` | Verbose mode (includes FFmpeg details/progress) |
| `-c, --check` | Run dependency/system checks only |
| `-h, --help` | Show help |

---

## Defaults and Behavior

- Output container: **MKV**
- Keyframe interval: **48**
- Audio target: **AAC stereo 214k** (all tracks)
- If AAC fails on a file: file processing fails (**no audio-copy fallback**)
- Subtitles: copied by default (ASS and others preserved)
- Attachments: copied by default
- Extras folders (`NC`, `NCOP`, `NCED`, `Extras`, `Sample`, `Featurettes`) are skipped by default (`--include-extras` to include)
- CSV results: written by default to `<output>/encode-results-YYYYmmdd-HHMMSS.csv`
- Existing output files: skipped by default (`--force` to overwrite)

Supported input extensions:

- `mkv`, `mp4`, `avi`, `m4v`, `mov`, `wmv`, `flv`, `webm`, `ts`, `m2ts`

By default, the script skips common extras/sample folders like:

- `NC`, `NCOP`, `NCED`, `Extras`, `Sample`, `Featurettes`

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
./jellyfin-encode.sh -m vaapi -q 19 "/media/input" "/media/output"
```

### CPU encode

```bash
./jellyfin-encode.sh -m cpu -q 20 -p medium "/media/input" "/media/output"
```

### Keep existing outputs untouched (default)

```bash
./jellyfin-encode.sh "/media/input" "/media/output"
```

### Force overwrite existing outputs

```bash
./jellyfin-encode.sh -f "/media/input" "/media/output"
```

### Disable subtitle and attachment copying

```bash
./jellyfin-encode.sh --no-subs --no-attachments "/media/input" "/media/output"
```

### Verbose FFmpeg diagnostics

```bash
./jellyfin-encode.sh -v -m cpu "/media/input" "/media/output"
```

### Custom CSV results path

```bash
./jellyfin-encode.sh --csv-log "/media/output/encode-report.csv" "/media/input" "/media/output"
```

### System diagnostics only

```bash
./jellyfin-encode.sh --check
```

---

## Troubleshooting

### "No VAAPI device"

- Check `/dev/dri/renderD*` exists.
- Ensure user permissions allow access to render device.
- Try CPU mode to confirm pipeline: `-m cpu`.

### Attachment warnings like "Could not find codec parameters for stream ... Attachment: none"

- These often come from odd font attachments in MKVs.
- The script maps only needed streams and should still proceed in most cases.

### Audio issues on specific files

- Run with `-v` to capture detailed FFmpeg output.
- AAC is strict for all tracks; unsupported inputs will fail for that file.

### Output already exists and file is skipped

- Use `-f` / `--force` to overwrite.

---

## Notes

- Logs printed with colors are for terminal readability; log files remain plain text.
- CSV rows include action (`encode`/`remux`), status, source/destination paths, and a `renamed` column.
- For large libraries, start with a small subset or `--dry-run` first.

