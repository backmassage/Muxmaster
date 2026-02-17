# Muxmaster Media Library Encoder

> A fast, resilient batch encoder/remuxer for Jellyfin-style libraries.

Current version: **1.1**

## Quick Changelog

### v1.1 (2026-02-16)

- Added FFmpeg hardening retries for attachment tag issues, subtitle mux failures, mux queue overflow, and timestamp discontinuities.
- Added `--strict` mode to disable automatic per-file retry fallbacks.
- Preserved original audio/subtitle stream metadata (track titles/language tags).
- Reorganized helper script structure and moved `harleybox_auto.sh` to `scripts/helpers/`.
- Updated HarleyBox helper mount settings to remove `umask=000` and add `nofail` to fstab options.

Full release notes: [`CHANGELOG.md`](./CHANGELOG.md)

## At a Glance

- HEVC video encoding via VAAPI or CPU/x265
- Strict AAC audio processing for all tracks (224k target)
- HEVC remux mode by default (copy video + process audio)
- MP4 output by default for browser/Edge playback compatibility
- Clean container metadata/chapters by default
- Automatic TV/movie folder structure normalization
- Stream-safe defaults for timestamp/audio render compatibility

The script is designed to handle mixed anime/TV/movie files, including dual-audio releases and browser-focused playback constraints.

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
        ├── clean_timestamps_remux.sh
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
  - Preserves original audio track metadata per track (title/language tags), including multi/dual-audio releases.
  - Avoids copying noisy per-stream encoder/duration tags into clean outputs.
- **Subtitle handling**
  - For MP4 output, subtitles are converted to `mov_text` when compatible.
  - If subtitle mux/convert fails, the file is retried without subtitles.
  - Preserves original subtitle track metadata (title/language tags).
- **Attachment handling**
  - MP4 output automatically skips attachment streams (fonts/images) for container compatibility.
  - If attachments are present and cause mux issues in non-MP4 workflows, the file is retried without attachments.
- **HEVC skip mode**
  - Default behavior remuxes HEVC sources (copy video + process audio).
  - Use `--no-skip-hevc` to force HEVC re-encode.
- **Metadata handling**
  - Default behavior strips container metadata and chapters for cleaner outputs.
  - Use `--keep-metadata` to preserve source container metadata/chapters.
  - If preserve mode fails for a file, that file is retried with clean metadata.
  - Stream-level audio/subtitle metadata is preserved in both clean and keep modes.
- **Mux/timestamp resilience**
  - Retries with a larger mux queue if FFmpeg reports packet buffer overflow.
  - Retries with generated timestamps when FFmpeg reports non-monotonic DTS.
  - Use `--strict` to disable all automatic per-file retry fallbacks.
- **MP4 stream compatibility flags**
  - Uses `hvc1` video tag plus `+faststart` and `+use_metadata_tags` for better Edge/browser handling.
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
| Disable proactive timestamp regeneration | `./Muxmaster.sh --no-clean-timestamps "/input" "/output"` |
| Disable forced matching audio layout | `./Muxmaster.sh --no-match-audio-layout "/input" "/output"` |
| Edge-safe pass (timestamps + matched audio layout, default behavior) | `./Muxmaster.sh "/input" "/output"` |
| Regenerate clean timestamps before retest | `scripts/helpers/clean_timestamps_remux.sh "/input.mkv" "/output_fixed.mkv"` |
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
| `--no-subs` | Do not process subtitle streams |
| `--no-attachments` | Do not include attachment streams |
| `--strict` | Disable automatic FFmpeg retry fallbacks (fail fast per file) |
| `--clean-timestamps` | Enable proactive timestamp regeneration on first remux/encode attempt (`-fflags +genpts`, default: on) |
| `--no-clean-timestamps` | Disable proactive timestamp regeneration |
| `--match-audio-layout` | Normalize all output audio streams to stereo layout with stable resampling (default: on) |
| `--no-match-audio-layout` | Disable explicit stereo layout normalization |
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

- Output container: **MP4**
- Keyframe interval: **48**
- Audio target: **AAC stereo 224k** (all tracks)
- If AAC fails on a file: file processing fails (**no audio-copy fallback**)
- Subtitles: converted to `mov_text` when compatible; incompatible subtitle formats are dropped via retry fallback
- Attachments: skipped automatically for MP4 container compatibility
- Container metadata/chapters: stripped by default (`--keep-metadata` to preserve)
- FFmpeg FPS/speed live progress is on by default (`--no-fps` to disable)
- Per-file source video stats are shown by default (`--no-stats` to hide)
- Existing output files: skipped by default (`--force` to overwrite)
- HEVC sources: remux by default (`--no-skip-hevc` to force HEVC re-encode)
- Automatic FFmpeg fallback retries are enabled by default (`--strict` disables them)
- Proactive timestamp regeneration is on by default (`--no-clean-timestamps` disables it)
- Audio layout normalization to stereo is on by default (`--no-match-audio-layout` disables it)

Supported input extensions:

- `mkv`, `mp4`, `avi`, `m4v`, `mov`, `wmv`, `flv`, `webm`, `ts`, `m2ts`

---

## Output Structure

The script attempts to classify files as TV episodes or movies from filename patterns.

### TV output

```text
<output>/<Show Name>/Season <NN>/<Show Name> - S<NN>E<NN>.mp4
```

### Movie output

```text
<output>/<Movie Name (Year)>/<Movie Name (Year)>.mp4
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

- These often come from odd source font attachments in MKVs.
- MP4 output skips attachment streams by design, so these warnings are usually source-side noise.

### Attachment tag errors like "Attachment stream ... has no filename/mimetype tag"

- MP4 output does not include attachment streams, so this is generally avoided by default.
- Manual override remains available: `--no-attachments`.

### Subtitle mux/copy errors

- MP4 output converts subtitles to `mov_text` when possible.
- If subtitle conversion/muxing fails (common with image-based subtitles), the script automatically retries without subtitles.
- Manual override: run with `--no-subs`.

### "Too many packets buffered for output stream"

- Newer script versions automatically retry the file with a larger FFmpeg mux queue.
- If issues persist, run with `-v` to inspect the full FFmpeg stream mapping and packet flow.

### Non-monotonic DTS / timestamp ordering errors

- Newer script versions automatically retry with generated timestamps for common DTS/PTS anomalies (including missing/invalid PTS messages).
- Proactive timestamp regeneration is enabled by default for all files.
- If needed, you can still explicitly force it:

```bash
./Muxmaster.sh --clean-timestamps "/input" "/output"
```

- If remuxes came from Blu-ray or large batch pipelines, timestamp irregularities can still break MSE when switching audio tracks.
- Run a clean stream-copy remux first, then retest playback:

```bash
ffmpeg -fflags +genpts -i "input.mkv" -map 0 -c copy "output_fixed.mkv"
```

- Helper command:

```bash
scripts/helpers/clean_timestamps_remux.sh "input.mkv" "output_fixed.mkv"
```

- This often fixes DTS discontinuities, missing PTS, and out-of-order timestamps.

### Edge playback issues when switching audio tracks

- Browser decoders can fail to reinitialize cleanly if tracks have different channel layouts.
- Muxmaster now normalizes output audio tracks to a consistent stereo layout with stable resampling by default.
- If needed, you can still explicitly force/confirm it:

```bash
./Muxmaster.sh --match-audio-layout "/input" "/output"
```

- For problematic Blu-ray/batch sources in Edge, run both protections together:

```bash
./Muxmaster.sh --clean-timestamps --match-audio-layout "/input" "/output"
```

### Audio issues on specific files

- Run with `-v` to capture detailed FFmpeg output.
- AAC is strict for all tracks; unsupported inputs will fail for that file.

### Output already exists and file is skipped

- Use `-f` / `--force` to overwrite.

---

## Notes

- Logs printed with colors are for terminal readability; log files remain plain text.
- For large libraries, start with a small subset or `--dry-run` first.

