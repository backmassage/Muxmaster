# Changelog

All notable changes to this project are documented in this file.

## [1.2] - Unreleased

### Added

- Helper utility `scripts/helpers/clean_timestamps_remux.sh` for clean stream-copy remux with generated PTS:
  - `ffmpeg -fflags +genpts -i input.mkv -map 0 -c copy output_fixed.mkv`
- CLI flags `--clean-timestamps` / `--no-clean-timestamps` to control proactive timestamp regeneration in base remux/encode runs.
- CLI flags `--match-audio-layout` / `--no-match-audio-layout` to normalize all output audio streams to a consistent stereo layout.

### Changed

- Expanded troubleshooting guidance for timestamp issues to include a dedicated clean-remux step before retesting playback.

## [1.1] - 2026-02-16

### Added

- `--strict` mode to disable automatic per-file FFmpeg fallback retries.
- Helper scripts folder organization with `scripts/helpers/` and `scripts/helpers/extra/`.
- Dedicated release changelog (`CHANGELOG.md`).

### Changed

- Project version finalized to `1.1`.
- Moved `harleybox_auto.sh` to `scripts/helpers/harleybox_auto.sh`.
- Updated HarleyBox helper mount/fstab defaults to remove `umask=000` and include `nofail`.
- Preserved audio/subtitle stream metadata (title/language tags) during remux and encode flows.

### Fixed

- Mitigated common FFmpeg remux/encode failures with targeted retry fallbacks:
  - attachment tag issues (missing filename/mimetype),
  - subtitle mux/copy failures,
  - mux queue overflow (`Too many packets buffered for output stream`),
  - timestamp discontinuity / non-monotonic DTS issues.
- Added keep-metadata to clean-metadata fallback for per-file robustness.
- Preserved distinct audio/subtitle track titles per stream index (fixes dual-audio name loss).
