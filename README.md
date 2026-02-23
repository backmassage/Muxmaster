# Muxmaster

Jellyfin-optimized media encoder: HEVC/AAC encoding and remuxing with deterministic output naming.

**Version:** 2.0 (Go rewrite; CLI parity with legacy v1.7.0 shell script.)

## Build

```bash
make build
```

Requires Go 1.23+.

## Usage

```text
muxmaster [OPTIONS] <input_dir> <output_dir>
```

- **Encoding:** `-m vaapi|cpu`, `-q <quality>`, `--container mkv|mp4`
- **Behavior:** `-d` dry-run, `--check` system diagnostics, `-f` overwrite existing
- **Help:** `-h`, `-V` version

See `muxmaster -h` for all options.

## Design

- Single static binary; no runtime deps beyond `ffmpeg`/`ffprobe`.
- Full CLI parity with the legacy Bash script (see `docs/legacy/`).
- Foundation plan and 2.0 design: `docs/muxmaster-go-foundation-plan-final.md`, `docs/Muxmaster2_0.md`.
