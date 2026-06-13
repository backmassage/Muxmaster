# VAAPI Quality Tuning & Encoder-Capability Corrections

**Status:** done (code + docs landed; hardware/Blu-ray VMAF re-confirmation of the `hqdn3d` constants and banding metric remain — see done-when)
**Goal:** Raise quality-per-bit for raw Blu-ray → Jellyfin transcodes on the VAAPI-first path by adding content-aware pre-filtering (denoise for film/grain, deband for anime) and an optional quality-priority QP strategy, while correcting three encoder behaviors that benchmarking on the dev box proved to be inert or counterproductive (HEVC B-frame detection false positive; QVBR is worse quality-per-bit than CQP; `-compression_level` is inert on this APU).
**Non-goals:** Flipping the default rate control to QVBR (benchmarks show CQP wins quality-per-bit — QVBR stays opt-in for peak-bounding only); removing the existing QVBR/`-compression_level` code (kept as harmless opt-in / portability for Intel and discrete AMD); a hybrid per-title x265 path (user chose VAAPI-first); 2-pass; AV1; tier/level/slices/tiles tuning (no measurable effect on radeonsi); auto-detecting film-vs-anime (the user selects via `--tune`).

## Context digest

Benchmark evidence (VMAF on grain + motion proxies, dev box: Radeon 680M / VCN 3.1
Rembrandt, Mesa 26.1.2 radeonsi, ffmpeg 8.1.1 — see memory `vcn31-vaapi-encode-facts`):

- **HEVC B-frames are a no-op on VCN 3.x.** `-bf 4`/`-bf 8` encode fine but produce
  **zero** B-frames (ffprobe: only I+P). Mesa MR !25565 added H.264 B-frames only.
  Our `check.testVaapiBFrames` exits 0 → reports "yes" → **false positive**.
- **QVBR loses to CQP** at matched size (95.9 vs 98.9 VMAF): `-b:v` binds before the
  `-global_quality` target. QVBR's only real use is bounding peak bitrate for
  bandwidth-limited streaming — not this project's goal.
- **`-compression_level` is bit-identical** across 1/16/32/default for HEVC on this APU.
- **Pre-encode denoise is the biggest lever:** light `hqdn3d` gave −21% size at
  −0.15 VMAF on motion+grain; literature reports 2–3× on real film grain.
  `denoise_vaapi` is **unimplemented on radeonsi**, and CPU filters **cannot** be
  inserted into a hardware-decoded VAAPI surface chain (ffmpeg error −22) — they
  require the software-decode path (the existing `SWVideoFilters` mechanism).
- **10-bit gives no banding win on VCN** (bigger + lower VMAF on a gradient) — keep
  main10 for HDR/compat but drop the SDR-banding rationale.
- **QP knee is ~24–26** for grain; SmartQuality's `+MaxOptimalOverride` upward push
  (toward smaller files) is the one thing working against "least quality loss."

Current pipeline (relevant seams): `planner.BuildPlan` sets `HWDecode` and builds
`VideoFilters` (HW chain) + `SWVideoFilters` (fallback) via `filter.BuildVideoFilter`.
`buildSoftwareDecodeFilters` is the single function feeding both VAAPI-sw and CPU
filter chains (CPU mode always has `HWDecode=false`). The §2b block in `planner.go`
applies the optimal-bitrate upward QP push.

## Files to touch

| File | Change |
|---|---|
| `internal/config/config.go` | `EncoderConfig.Tune` (enum), `QualityPriority` bool; `TuneMode` type + consts; defaults; `Validate` case |
| `internal/config/flags.go` | `--tune`, `--quality-priority` flags + `tuneValue` adapter + help text |
| `internal/planner/filter.go` | inject tune prefilter into `buildSoftwareDecodeFilters` (after deinterlace, before tonemap); `TunePrefilter(cfg)` helper |
| `internal/planner/planner.go` | force `HWDecode=false` when a prefilter is active; apply tune QP bias; gate §2b optimal-bitrate push behind `!QualityPriority`; note line |
| `internal/planner/quality.go` | `tuneQPBias`/`tuneCRFBias` folded into `SmartQuality` selected values |
| `internal/check/check.go` | `testVaapiBFrames` writes a temp encode and verifies `pict_type=B` appears (generic: AMD→false, Intel→true) |
| `internal/planner/*_test.go`, `check` (new test file) | tune matrix, quality-priority, prefilter→sw-decode forcing, B-frame verification |
| `AGENTS.md`, `_docs/design/structure.md`, `CHANGELOG.md` | document tune, quality-priority, corrected B-frame/QVBR/compression_level reality |

## Ordered steps

### Phase 1 — Corrections (bug + framing)

1. **Fix the B-frame capability false positive.** Rewrite `check.testVaapiBFrames`
   to encode a short clip (`color`+motion, ~12 frames, `-bf 2`) to a temp file
   (`os.CreateTemp`, `.mkv`, removed after), then run `ffprobe -show_entries
   frame=pict_type` and return true only if a `B` frame is present. This makes the
   probe truthful on both AMD (no B-frames → `VaapiBFrames=false` → no `-bf` emitted)
   and Intel (real B-frames → stays enabled). The `RetryDropBFrames` class remains as
   a runtime safety net. Update the `logVaapiCaps`/`--check` line wording if needed.

2. **Reframe QVBR and `-compression_level` (docs/comments only, no code removal).**
   In `AGENTS.md` and the QVBR step comment, state QVBR is opt-in for peak-bitrate
   bounding (bandwidth-limited streaming), **not** a quality improvement, and that CQP
   is the recommended default for quality-per-bit. Cancel the prior plan's
   "flip default to QVBR" item. Note `-compression_level` is inert for HEVC on VCN
   (kept for Intel/discrete-AMD portability). Remove the stale done-when item from
   `ffmpeg-jellyfin-optimizations.md` that proposed flipping the default.

### Phase 2 — Content-aware pre-filtering (`--tune`)

3. **Config + flag.** Add `TuneMode` (`none`|`film`|`anime`|`grain`, default `none`)
   to `EncoderConfig`, a `tuneValue` flag adapter mirroring `hdrModeValue`, `--tune`
   flag + help text, and a `Validate` case. `none` preserves all current behavior.

4. **Prefilter mapping + injection.** Add `TunePrefilter(cfg) string` in `filter.go`:
   - `film` → `hqdn3d=1.5:1.5:6:6` (light luma+chroma spatial/temporal)
   - `grain` → `hqdn3d=4:4:9:9` (heavier; strong Blu-ray grain)
   - `anime` → `gradfun=1.2:16` (fast gradient deband; `deband` is the heavier
     alternative if validation shows residual banding)
   - `none` → `""`
   Inject the prefilter string into `buildSoftwareDecodeFilters` **after** the
   deinterlace stage and **before** the HDR tonemap/format/hwupload stages (denoise
   must follow deinterlace; deband before scaling). This single function feeds both
   the VAAPI-sw and CPU chains, so CPU mode gets the prefilter for free.

5. **Force software decode when a prefilter is active.** In `planner.go`, when
   `TunePrefilter(cfg) != ""` and mode is VAAPI, set `plan.HWDecode = false` before
   building filters (CPU filters cannot enter a VAAPI surface chain — proven by the
   error −22 test). `VideoFilters` then carries the sw chain directly; no
   `SWVideoFilters` fallback needed. Add a one-line `plan.Notes` entry naming the
   active tune + prefilter and that GPU decode is disabled for it.

6. **Tune QP bias.** In `quality.go` `SmartQuality`, add `tuneQPBias`/`tuneCRFBias`
   to the clamped selected values: `anime` +1 (flat cels tolerate slightly higher QP;
   deband handles banding), `film`/`grain`/`none` 0. Re-clamp to existing min/max.

### Phase 3 — Quality-priority QP strategy

7. **`--quality-priority` flag** (bool, default off) in config + flags. When set, skip
   the §2b optimal-bitrate **upward** QP/CRF push in `planner.go` (the
   `targetQP > plan.VaapiQP` / `targetCRF > plan.CpuCRF` blocks), letting the
   SmartQuality value — which already biases toward quality for high-density Blu-ray
   sources — stand as-is. Leaves the preflight/post-encode size safety nets intact
   (they only fire when output ≳ input, which never happens for Blu-ray). Add a note
   line when active. Document that it pairs naturally with `--tune` for keeper content.

### Phase 4 — Tests & docs

8. **Tests.**
   - `planner`: matrix over `tune ∈ {none,film,anime,grain}` × `mode ∈ {vaapi,cpu}`
     asserting prefilter presence/order in the filter chain, `HWDecode` forced false
     for VAAPI+prefilter, and QP bias applied; `quality-priority` on/off asserting the
     optimal-bitrate push is skipped (QP equals SmartQuality value) vs applied.
   - `check`: `testVaapiBFrames` verification logic — table-drive the ffprobe
     pict_type parsing (B present → true; only I/P → false) against canned frames.
   - Confirm `none` defaults leave every existing planner/builder test unchanged.
9. **Docs.** `AGENTS.md` (tune profiles, quality-priority, corrected encoder reality,
   prefilter-forces-sw-decode gotcha), `structure.md` quick-finder rows, `CHANGELOG`
   `[Unreleased]` entries. `make ci` green.

## Done-when

- [x] `make ci` passes; new matrix tests cover tune × mode, quality-priority on/off, and B-frame ffprobe verification; all pre-existing tests pass unchanged with `--tune none`.
- [~] `--check` on the 680M now reports **B-frames: no** (verification fix), and still QVBR: yes; on an Intel iHD stack the same probe reports B-frames: yes (logic is generic, not AMD-special-cased). *(Code landed and `pictTypesContainB` table-tested; live `--check` on each stack still to be eyeballed.)*
- [x] `--tune film` / `grain` on a VAAPI run forces software decode and inserts the `hqdn3d` prefilter after deinterlace (unit-asserted: `TestBuildPlan_TuneMatrix`, `TestBuildPlan_TunePrefilterOrder`). *(≥15% smaller / ≤0.3 VMAF and final `hqdn3d` constants still to be re-confirmed on a real Blu-ray.)*
- [x] `--tune anime` inserts the deband prefilter (unit-asserted). *(Banding-metric comparison on a flat-gradient sample still to be run.)*
- [x] `--quality-priority` makes the planner emit the SmartQuality QP with no upward optimal-bitrate push (unit-asserted: `TestBuildPlan_QualityPriority`). *(Blu-ray QP/VMAF comparison still to be run.)*
- [x] `--tune none` and no `--quality-priority`: ffmpeg args unchanged (regression guard — all 460 pre-existing tests pass unchanged).
- [x] Docs state CQP is the quality default and QVBR is opt-in peak-bounding only; the "flip default to QVBR" item is cancelled in `ffmpeg-jellyfin-optimizations.md`.
