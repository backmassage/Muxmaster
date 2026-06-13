# Version 2.6 Live Validation Checklist

Purpose: record the hardware/media checks that unit tests cannot prove. Run this before calling the 2.6 quality and VAAPI claims fully validated.

## Environment

- Date:
- Host:
- GPU / driver:
- Mesa / VAAPI driver:
- ffmpeg:
- ffprobe:
- Muxmaster commit:

## Checks

1. `muxmaster --check` on AMD VAAPI.
   - Record chosen render node, QVBR support, and B-frame support.

2. `muxmaster --check` on Intel VAAPI.
   - Record chosen render node, QVBR support, and B-frame support.

3. Dolby Vision profile 5 sample.
   - Confirm planner skips with "Dolby Vision profile 5 (no HDR10 base layer)".

4. Dolby Vision profile 8 / HDR10-base sample.
   - Probe before and after remux/encode.
   - Confirm output has no DOVI side data/config record and remains HDR10-playable.

5. MP4 `mov_text` subtitles to MKV.
   - Confirm text subtitles become SRT and sparse subtitle muxing does not fail.

6. MKV cover art plus video bitstream filter.
   - Confirm attached picture streams survive when `-bsf:v:0 dovi_rpu=strip` is present.

7. Auto-tune sample sweep.
   - Run clean, light-grain, heavy-grain, and anime/deband samples.
   - Record suggested tune, selected tune, output size, spot-check visual result, and VMAF/SSIM if available.

8. QVBR vs CQP spot check.
   - Run matched samples with `--vaapi-rc cqp` and supported `--vaapi-rc qvbr`.
   - Record size, bitrate spikes, playback behavior, and quality metric notes.

## Results

Add dated result notes below this line. Keep failures with exact command lines and probe excerpts so follow-up fixes are reproducible.
