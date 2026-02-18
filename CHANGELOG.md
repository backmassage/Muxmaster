# Changelog

All notable changes to this project are documented in this file.

## [1.3.0] - 2026-02-18

### Changed

- Updated default quality from `19` to decoupled defaults: VAAPI QP `18`, CPU CRF `20`.
- Updated default AAC audio bitrate from `192k` to `256k`.
- Bumped bundled script version to `Muxmaster.sh v1.3.0`.
- Updated README defaults and release metadata for `1.3.0`.

### Fixed

- Improved FFmpeg stream analysis for HDR/color/interlace detection by targeting the primary non-attachment video stream, preventing false analysis on files with attached cover-art video streams.
- Hardened HDR metadata passthrough so ffmpeg only receives valid color flags when metadata is present, reducing encode failures caused by unknown color values.
- Fixed command-substitution logging contamination in option-builder helpers (`build_audio_opts`, `build_subtitle_opts`, `build_video_filter`) that could corrupt generated FFmpeg argument lists in verbose/warning scenarios.
- Updated per-stream audio handling so AAC tracks are always copied as-is (no AAC-to-AAC re-encode), while non-AAC tracks are encoded to AAC.
- Fixed filename tag cleanup to only trim known release tags at token boundaries, preventing accidental title truncation for names containing substrings like `NonAAC`.

## [1.2.3] - 2026-02-17

### Added

- Argument validation for:
  - `--container` values (`mkv`, `mp4`)
  - `--hdr` values (`preserve`, `tonemap`)
- Runtime safety guard that rejects output directories that are the same as, or nested inside, the input directory.

### Changed

- Updated README for release `1.2.3` and aligned documented behavior/options with `Muxmaster.sh v2.1.0`.
- Clarified CLI help text for `--no-match-audio-layout`.

### Fixed

- Fixed audio channel-selection logic during re-encode so mixed multi-audio inputs no longer get forced to mono when the first track is mono.

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
