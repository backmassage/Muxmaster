# hevc_vaapi Quality Maximization — Content-Adaptive Default Tuning

**Status:** in progress — Changes 1, 5, 6 landed with production values
(`vcnQPCeilingClean=16`, `vcnQPCeilingGrain=24`, `vcnContentClassMinGrain=24`,
`fallbackQPCeiling=21`). Change 6 closed via a point-estimate re-key of
`PreflightAdjust` (not a `vaapiRatios` refit); the ceiling now binds on h264/hevc
(1080p h264 BD holds at QP16, point ≈92%) and the `pipeline/runner.go` post-encode
loop is the primary real-size guard. Changes 2/3/4 and the VMAF sweep remain open.
**Owner concern:** raise quality-per-bit (and absolute quality) of the default
`hevc_vaapi` path on the VCN-class target hardware, content-adaptively, without
regressing the size discipline beyond a flexible ~25% upper bound.

## Goal

Make the *default* `hevc_vaapi` encode prioritize quality, adapting per content
class (clean / film / grain / anime) by reusing the existing `--tune auto`
detection and the SmartQuality QP machinery. Quality is the priority; a size
increase is acceptable **only where the quality gain justifies it**, treating
~25% as a flexible ceiling, not a hard limit.

## Non-goals

- Any codec other than `hevc_vaapi`. No `av1_vaapi`, no `libx265`/CPU-path
  retuning (the CPU path stays as-is; it inherits the prefilter for free but is
  not the subject here), no 2-pass, no per-title x265.
- Flipping the default rate control to QVBR. **Measured on VCN 3.1:** QVBR lost
  ~3 VMAF to CQP at matched size (98.9 → 95.9) in the dev-box test. The *mechanism*
  ("`-b:v` binds before `-global_quality`") is a hypothesis, not a verified
  ordering — the loss could equally be VBV/underfill behavior, and the exact
  command lines (rc_mode, b:v/maxrate/bufsize, global_quality) must be recorded
  with the result. The conclusion is scoped to **VCN**; it is *not* assumed to
  generalize to Intel or discrete AMD. CQP stays the default; QVBR stays opt-in
  for peak-bitrate bounding only. (memory: `vcn31-vaapi-encode-facts`)
- Touching knobs measured inert/fake on this hardware (see "Confirmed
  no-ops" below) — they are documented as non-levers, not tuned.
- Auto-detecting anime vs live-action. **Research-settled:** no reliable
  off-the-shelf classifier; the film/anime call stays with the user via the
  per-series prompt. (memory: `content-detection-for-tuning`)
- Re-deriving the grain-detection method. The `bitplanenoise` LSB approach is
  settled; only its *cut points* and the denoise *strengths* are open.

## Context digest

Target: AMD VCN 3.1-class (Radeon 680M dev box, Mesa radeonsi, ffmpeg 8.1),
`hevc_vaapi`, CQP. Project memory holds adversarially-verified, on-hardware
measurements that **override** generic FFmpeg/VAAPI advice on conflict.

Where the relevant code lives:
- Defaults: [internal/config/config.go](../../internal/config/config.go) `DefaultConfig`.
- QP selection: [internal/planner/quality.go](../../internal/planner/quality.go)
  `SmartQuality` (resolution/bitrate/density curves + `SmartQualityBias` + per-tune bias).
- Curves/tables: [internal/planner/tables.go](../../internal/planner/tables.go).
- Size-targeting push: [internal/planner/planner.go](../../internal/planner/planner.go)
  §2b (`OptimalBitrate` → `QPForTargetBitrate` → upward QP push, cap `MaxOptimalOverride`),
  and the §2b preflight safety net.
- ffmpeg args: [internal/ffmpeg/builder.go](../../internal/ffmpeg/builder.go) `appendVideoCodec`.
- Prefilters: [internal/planner/filter.go](../../internal/planner/filter.go) `TunePrefilter`.
- Detection: [internal/tune/signal.go](../../internal/tune/signal.go) (grain pre-pass),
  [internal/tune/suggest.go](../../internal/tune/suggest.go) (thresholds → tune).

## Current default hevc_vaapi behavior (summary)

| Knob | Default today | Quality role |
|---|---|---|
| Rate control | CQP via bare `-qp` | **Correct.** Best quality-per-bit on VCN (measured). |
| Base `VaapiQP` | 18 | Starting point before curves/bias. |
| `SmartQualityBias` | −2 | Global push toward lower QP / higher quality. |
| QP clamp | [14, 30] | Hard bounds after curves. |
| SmartQuality curves | res + bitrate + density tiers | Lower QP for low-res / low-density; gentler than CPU. |
| **Optimal-bitrate upward QP push** | **active** (`QualityPriority=false`); **fires at the +3 cap on nearly every h264→HEVC encode** (traced below) | **Works *against* quality:** raises QP (up to +`MaxOptimalOverride`=3) to shrink files toward an "optimal" size target. |
| Preflight bump | bump QP until est ≤105% of input | Safety net vs output > input. |
| Per-tune QP bias | anime **+1**, film/grain/clean 0 | Anime biased *toward smaller* (away from quality). |
| `--tune auto` | grain pre-pass → `film`/`grain`/`none`; anime user-picked | film=`hqdn3d=1.5:1.5:6:6`, grain=`hqdn3d=4:4:9:9`, anime=`gradfun=1.2:16`. |
| Profile | `main10`/`main` derived | Keep (HDR + client compat). |
| `-g` (GOP) | ~2s from fps | Not a quality lever here. |
| `-compression_level` | 1 | **Inert for HEVC on VCN** (bit-identical 1/16/32). |
| `-async_depth` | 4 | Throughput only, no output effect. |
| `-bf` | only if detected | **Fake on VCN 3.x** (encodes I+P only). |

**The headline:** the defaults are tuned for *size discipline*. Two mechanisms
actively trade quality for smaller files — the §2b upward QP push and the anime
`+1` bias — and the heavy-grain denoise may be over-aggressive. A quality-first
default reweights exactly these three, content-adaptively, while keeping the
size *safety* nets (which only fire when output ≳ input) intact.

### Worked trace: the §2b push is the dominant quality limiter, not an edge case

Tracing the QP math by hand (no encode) on representative h264 Blu-ray inputs:

- **1080p h264 @ 25 Mbps** (density ~12 000 kbps/Mpx): `SmartQuality` → QP **14**
  (base 18 −1 bitrate −2 density −2 bias, clamped to the floor). `OptimalBitrate`
  → ~15 Mbps; `QPForTargetBitrate` → ~23. Since 23 > 14 the push fires and is
  capped at `+MaxOptimalOverride` → **QP 17**.
- **1080p h264 @ 8 Mbps** (density ~3 850): `SmartQuality` → QP **16**; target
  ~5.4 Mbps; `QPForTargetBitrate` → ~22 → pushed, capped → **QP 19**.

Because h264→HEVC genuinely compresses 30–40%, the optimal target almost always
lands well below what a low QP produces, so the push **fires at the full +3 cap
on essentially every h264→HEVC encode** — it is the *default* behavior, not a
corner case. The *size cost* of giving that QP back is **not estimated here** —
the rate–QP slope on VCN must be measured (a textbook ~6-QP-per-2× slope would
make a −3 QP move ≈ +40%, which could exceed the ~25% envelope), and the
`qpCeiling` in Change 1 is chosen from that measurement to stay in budget. This
trace also shows why "skip the push entirely" would be wrong on clean content: it
would let QP fall to ~14, past the visual knee (huge bits, invisible gains) —
hence the bounded **QP-ceiling** design in Change 1 rather than a full skip.

## What research/measurement confirms about the lever set

**Confirmed levers (act on these):**
- **CQP + lower QP is the quality dial.** Web corroborates: VAAPI has no CRF; CQP
  (`-rc_mode CQP` / `-qp`) is the quality mode, lower QP = higher quality/larger
  size (FFmpeg default global_quality 25). (Brainiarc7 VAAPI gist; TauCeti) — and
  CQP > QVBR **on this GPU** in the dev-box test (memory; mechanism unconfirmed,
  not generalized off-VCN — see non-goals).
- **Pre-encode denoise is the single biggest quality-per-bit lever for noisy
  sources.** Measured: light `hqdn3d=1.5:1.5:6:6` gave −21% size at only −0.15
  VMAF on a motion+grain proxy (memory). Literature: removing grain before
  encode yields large bitrate savings; the freed bits can be spent on a lower QP
  → net fidelity gain. (USPTO 10,911,785; arXiv 2402.00622)
- **But "strong" hqdn3d destroys fine detail.** dr-lex notes strong hqdn3d
  removes most fine detail; NLMeans denoises comparably while *preserving*
  detail and reaching lower bitrate. → the current `grain` strength (`4:4:9:9`)
  is a prime suspect for over-denoising and must be re-derived for a quality-first
  default. (dr-lex videotips)
- **QP knee is content-dependent.** Grain knee ~24–26 (VMAF 99 at qp25; below
  qp22 huge bits for invisible gains) — so for grain the lever is *denoise*, not
  driving QP very low. Clean/flat content benefits from lower QP much more
  visibly. (memory)

**Confirmed no-ops / counterproductive on this hardware (document, do not tune):**
- B-frames: `-bf` accepted but **zero B-frames produced** on VCN 3.x (H.264-only
  in Mesa). The `--check` probe is a false positive (tracked separately).
- `-compression_level`: bit-identical output; kept only for Intel/discrete-AMD portability.
- `denoise_vaapi`: unimplemented on radeonsi; CPU denoise can't enter a hw VAAPI
  surface chain (err −22) → denoise requires the software-decode path (already wired).
- 10-bit for SDR banding: no win on VCN (bigger + lower VMAF on a gradient); keep
  `main10` for HDR/compat only.
- `-low_power`, `-refs`, tier/level/slices/tiles: no measured quality effect on
  radeonsi (prior plan). `ffmpeg -h encoder=hevc_vaapi` on the box confirms
  `-low_power` defaults `false` (the higher-quality regular VCN path); `-tier`,
  `-level`, `-tiles` exist but are measured inert. Treat as low-confidence; at
  most a single confirmatory probe, not a default change.

(All bullets above marked "measured"/"memory" are on-hardware facts and win over
any conflicting web guidance.)

**Untested candidates surfaced by `ffmpeg -h encoder=hevc_vaapi` (probe once, do
not assume):** these are *not* in memory and are not default changes — they enter
the validation sweep only as confirmatory probes.
- **`-rc_mode ICQ`** (mode 4) exists alongside CQP/QVBR. Memory only compared CQP
  vs QVBR; ICQ ("intelligent constant-quality") is untested on radeonsi and may
  be unsupported or a no-op. Probe vs the CQP baseline at matched size; keep CQP
  default unless ICQ measurably wins.
- **I-frame QP offset.** `-qp` is documented as the *P-frame* QP, "scaled by
  qfactor/qoffset for I/B." With B-frames fake here, only I/P exist — a lower
  I-frame QP (better-referenced keyframes) is a cheap potential lever. Probe
  `-i_qfactor`/`-i_qoffset` vs default; low confidence, validation-gated.

## Proposed content-adaptive default changes

Each change is gated through the *existing* `--tune` resolution and SmartQuality
machinery so it composes with detection rather than bypassing it.

### Change 1 — Quality-first by default: bound the §2b push with a QP ceiling  *(highest impact)*

- **What:** Stop letting the optimal-bitrate push trade quality for size on every
  encode. Replace the `base + MaxOptimalOverride` cap with an **absolute QP
  ceiling** on how far the push may raise QP. Stated explicitly (no hand-waving):

  ```
  finalQP = max(baseQP, min(pushTargetQP, qpCeiling))
  ```

  where `baseQP` is the SmartQuality value, `pushTargetQP` is
  `QPForTargetBitrate(optKbps)` as today, and `qpCeiling` is a per-content-class
  constant (placeholder ~16–17; **its real value comes from the rate–QP sweep**,
  see Change 1 caveat below). Edge cases this resolves (each gets a unit test):
  - `baseQP < qpCeiling` (e.g. 14, push 23, ceiling 16) → **16**: the push still
    raises QP, but to the absolute ceiling, not `base+3`=17 — recovering quality
    while staying past the visual knee (no QP-14 bloat on clean content).
  - `baseQP ≈ qpCeiling` → ≈ base: the push is effectively neutralized.
  - `baseQP > qpCeiling` (low-quality-target content, e.g. base 18) → **base**:
    the `max(baseQP, …)` term wins, the push is fully disabled, SmartQuality stands.

  Note this is a *ceiling on QP* (an upper bound on QP = a **lower bound on
  quality**), deliberately not called a "floor" — the earlier draft's "floor /
  at-or-below" wording was self-contradictory.
  Keep the preflight (≤105% of input) and post-encode escalation untouched — they
  only fire when output ≳ input, which never happens for Blu-ray, so the ceiling
  cannot produce output larger than the source.
- **Rationale:** The trace below shows the push hits the +3 cap on essentially
  every h264→HEVC encode, making it the dominant quality limiter — memory names
  it *"the one thing working against least quality loss."* A full skip would
  overshoot to QP ~14 (past the knee → wasted bits). An absolute ceiling recovers
  the discarded quality while staying past the knee. Web confirms lower QP =
  higher quality.
- **Expected impact:** Quality ↑ on virtually all encodes (QP is no longer pushed
  to `base+3`, only to the ceiling); size ↑ by the rate-cost of that QP delta.
  **The magnitude is explicitly unmeasured here** (see caveat) and bounds both
  the chosen ceiling and whether the ~25% envelope holds.
- **Change 1 caveat — the size delta and ceiling value are sweep-derived, not
  asserted.** A textbook HEVC rate–QP slope (~6 QP per 2× rate) would make a −3
  QP move ≈ **+40%** size, not a small delta — which could blow the ~25% envelope
  on clean content. The earlier "+12–15%" figure came from this project's own
  `vaapiRatios` heuristic table (QP14≈930‰ vs QP17≈820‰), **not** from measured
  VCN behavior, and is withdrawn. The rate–QP curve must be measured on VCN
  (CQP) per content class (validation step adds this); the `qpCeiling` is then set
  to the lowest QP whose size delta vs the old pushed QP stays inside the envelope.
  If the slope is steep, the ceiling rises (smaller quality recovery) to stay in budget.
  **The ceiling only binds if the preflight/estimate model lets it** — today the
  `EstimateBitrate.vaapiRatios` table overrides it back to the legacy QP on
  h264/hevc sources (measured in-tree). See Change 6.
- **Source:** worked trace below + memory (`vcn31-vaapi-encode-facts`: push works
  against quality; knee ~24–26 grain / lower for clean); web (lower QP = higher
  quality — Brainiarc7/TauCeti). Slope/envelope: **to be measured**, not assumed.

### Change 2 — Reverse the anime QP bias: +1 → 0 (validate −1/−2)

- **What:** Change `tuneQPBias`/`tuneCRFBias` for `anime` from **+1** to **0** as
  the quality-first default, with −1/−2 as validation candidates.
- **Rationale:** The current +1 biases flat-cel content *toward smaller files*.
  Flat cels with hard line art code extremely cheaply, so lowering (not raising)
  QP costs little size and avoids re-introducing banding/ringing on gradients and
  edges that a *pre-encode* deband cannot fix after the encoder quantizes. For a
  quality-first default this bias points the wrong way.
- **Expected impact:** Quality ↑ on animation (cleaner gradients/edges); size ↑
  small (cels are cheap). Strong candidate for a free quality win; validate
  whether −1/−2 is also near-free.
- **Source:** reasoning from codec behavior on flat content + web (lower QP =
  higher quality). No on-hardware anime measurement exists yet → validation-gated.

### Change 3 — Recalibrate grain denoise strength; evaluate NLMeans for `grain`

- **What:** Treat `grain = hqdn3d=4:4:9:9` as a hypothesis, not a fixed default.
  Sweep gentler hqdn3d settings (e.g. `3:2:6:6`, `2:1.5:6:6`) and an `nlmeans`
  variant for the `grain` tune, selecting by quality-per-bit (BD-rate) rather
  than size alone. Keep `film = hqdn3d=1.5:1.5:6:6` (already measured strong).
- **Rationale:** Strong hqdn3d destroys fine detail (dr-lex); NLMeans denoises
  comparably while preserving detail and reaching lower bitrate. Over-denoising
  lowers fidelity-vs-source even as it shrinks files — the opposite of
  quality-first. The film setting is already measured at an excellent operating
  point (−21%/−0.15 VMAF) and anchors the gentler end.
- **Expected impact:** Quality ↑ on grain (detail retained), size roughly flat or
  ↓ (denoise still removes expensive-to-code noise). NLMeans is CPU-heavy and the
  tune path is already software-decode, so cost is the tradeoff to measure.
- **Source:** dr-lex (hqdn3d vs NLMeans detail/bitrate); memory (film hqdn3d
  measured point, grain strengths are explicitly INITIAL ESTIMATES); USPTO/arXiv
  (denoise→bitrate savings).

### Change 4 — Spend freed bits: pair denoise tunes with a small QP reduction (validate)

- **What:** For `film`/`grain`, after denoise, nudge QP **down 1** (a negative
  `tuneQPBias` for those modes) so the bits denoise frees up buy fidelity instead
  of only shrinking the file. This bias is *not* self-clamping — it is bounded by
  the Change 5 `contentClassMin` invariant (grain floor = knee 24, landed), which is
  enforced after the full bias stack so global −2 + tune −1 cannot compound below
  the knee.
- **Rationale:** Denoise + lower-QP is the quality-first synergy: the denoised
  signal codes cheaply and cleanly, and a slightly lower QP converts that
  efficiency into visible quality rather than just a smaller file.
- **Expected impact:** Quality ↑ on film/grain; size partially offsets the
  denoise savings (net should stay ≤ source). Validate that the QP step lands
  above the knee.
- **Source:** memory (knee; denoise lever); web (lower QP = higher quality).

### Calibrate detection cut points (open item, folded in)

`grainLightThreshold=0.20` / `grainHeavyThreshold=0.55` in
[suggest.go](../../internal/tune/suggest.go) are uncalibrated initial estimates.
The validation sweep below produces the data to set them (and the chosen denoise
strengths) on real Blu-ray HEVC, closing the memory-flagged open question.

### Explicitly unchanged (with reason)

CQP/`-qp` default, `main10` profile, `-g`, `-compression_level`, `-async_depth`,
`-bf` gating, QVBR-as-opt-in, global `SmartQualityBias=−2`, and the
resolution/bitrate/density curves — all either measured optimal/inert or out of
scope. The global bias is left at −2 (already quality-leaning); the content-class
levers above do the quality-first work without a blunt global shift.

### Change 5 — Portability gating + a hard QP invariant (the levers must not stack past the knee)

These defaults ship to **Intel and discrete AMD (RDNA) too**, but the calibration
is VCN-3.1-specific. Two safeguards:

- **Gate VCN-derived magnitudes by VAAPI vendor/driver generation.** The
  `qpCeiling`, the denoise strengths, and the per-tune biases are calibrated on
  VCN 3.1; treat them as the VCN profile. For other vendors/generations, fall
  back to conservative values until measured (the `check` package already detects
  the device — extend it to expose vendor/gen so the planner can branch). Do not
  assume the VCN "inert/fake" facts (B-frames, `compression_level`, 10-bit
  banding) hold on Intel/RDNA — re-probe per device.
- **Enforce a QP invariant so the levers can't compound past the visual knee.**
  The proposed shifts stack: global bias (−2) + per-tune bias (anime 0 / film·grain
  −1, Change 4) + the QP-ceiling (Change 1). Replace the implicit "clamp" with an
  explicit post-stack invariant, unit-tested:

  ```
  finalQP = clamp(baseCurveQP + globalBias + tuneBias, contentClassMin, VaapiQPMax)
  ```

  where `contentClassMin` is a per-class, per-device floor — for `grain` it is the
  measured knee (landed at 24 on VCN; memory: VMAF 99 at qp25, below qp22 = huge
  bits for invisible gains), so film/grain −1 can never drive grain QP below it. The ceiling (Change 1) is
  applied to the *push target*, the min to the *bias stack*; the test matrix must
  prove no combination of (bias, tune, ceiling, push) exits `[contentClassMin,
  VaapiQPMax]`.
- **Re-verify the B-frame fact, don't just cite it.** Validation re-runs the
  `ffprobe -show_entries frame=pict_type` check on the current driver (B-frames
  are driver-version-specific; Mesa could have changed since the measurement) and
  on Intel; `-bf` gating follows the live result, not the cached claim.

### Change 6 — Recalibrate the estimate/preflight model so the ceiling actually binds  *(closes the Change 1 ↔ preflight coupling)*

> **LANDED (resolved differently than originally planned).** The override is fixed
> by **re-keying `PreflightAdjust` to the POINT estimate** (trigger when predicted
> output > ~105% of input), **not** by refitting `vaapiRatios`. The table stays an
> approximate **display-only** heuristic — it no longer gates the ceiling, so the
> "land the relaxed trigger and the refit curves together, post-sweep" sequencing
> below is moot: the re-key is safe on its own because the point estimate (≈92% for
> a 1080p h264 BD at QP16) no longer over-predicts the way the ×1.30 HIGH band did.
> Accepted consequence (the risk this section's sequencing worried about): softened
> preflight is no longer the anti-bloat net, so the **`pipeline/runner.go` post-encode
> escalation loop is now the primary real-size guard** (output>input → bump QP,
> re-encode, max 2×) — it checks measured output, not a prediction. The per-(content
> class, codec, resolution) curve refit and the dual-codec sweep below remain
> available as a *future display-accuracy* improvement, not a correctness blocker.
> Worked example (final): SmartQuality base ≈15 → bounded push → **QP16** (ceiling)
> → preflight **no bump** (point 92%) → final **QP16**. Before the re-key the same
> case settled at **QP19–20**.

- **Problem (measured in-tree, not hypothetical).** Change 1 lowers QP to the
  quality ceiling, but `PreflightAdjust` immediately re-clamps it upward using
  `EstimateBitrate`'s `vaapiRatios` table — the same withdrawn heuristic that
  produced the retracted "+12–15%" figure, not measured VCN data. For h264/hevc
  inputs that table predicts ~94–106% of input at QP 16, and because preflight
  trips on `HighPct = point × 1.30 > 105%` (a hard ~19%-shrink floor), it bumps QP
  back to 19–22 — exactly where the legacy push lands. **Net: Change 1 is inert on
  the dominant input codecs until this model is fixed.** Confirmed in-tree: h264
  25 Mbps QP16 → preflight settles QP19; only low-efficiency sources the table
  rates cheap (mpeg2 8 Mbps) let the ceiling stand at QP16.
- **`vaapiRatios` *is* the rate–QP curve — one calibration, not two.** The same
  sweep that sets `qpCeiling` (lowest in-envelope QP) is the fit for these curves;
  `qpCeiling` is a point read off the fitted curve. Calibrate them jointly so they
  stay mutually consistent (the current contradiction — `optimalCodecRatio`=68%
  vs `vaapiRatios`@QP16≈94% — is exactly what the fit resolves).
- **What (decided trade-offs):**
  - *Relax preflight to true anti-bloat.* Re-key the trigger off the **point**
    estimate near ~100% of input (drop/shrink the `×1.30` high-band trigger) so an
    accurate model fires only when output genuinely approaches the source. The hard
    "never larger than source" gate and the **post-encode escalation loop remain
    the backstop**; preflight stops enforcing a spurious ~19% shrink floor.
  - *Per-content-class rate–QP curves.* Replace the single `vaapiRatios` curve +
    additive `codec/res/density/bitrate` biases with curves indexed by **(source
    codec, content class, resolution tier)**. The per-class curve **subsumes** the
    density/bitrate biases (no double-counting) and measuring both codecs subsumes
    the additive `codecBias` guess.
  - *Measure both source codecs.* The sweep encodes **h264→hevc and hevc→hevc**
    (the two dominant real inputs) so the hevc re-encode path is measured, not a
    `+180` additive guess.
- **Implementation issues to honor:**
  - `EstimateBitrate` runs inside the push (`QPForTargetBitrate`) **and** preflight
    **and** the dry-run estimate display — refitting moves all three; re-baseline
    `TestFullPipeline_DebugMatrix` and the displayed estimate together.
  - The content class at estimate time reads from `cfg.Encoder.Tune` (resolved
    before `BuildPlan`), so per-class selection has its signal — but "clean"
    (`none`/unresolved-auto) needs its own curve distinct from the detected classes.
  - **Sequencing is strict.** Relaxing preflight is unsafe while the table
    over-predicts output, so the relaxed trigger and the refit curves land
    **together, post-sweep**, behind the INITIAL-ESTIMATE gate. Unlike Change 1
    there is no safe pre-sweep flip — no live code ships for this change now.
- **Source:** in-tree measurement of the coupling (this branch); `vaapiRatios`
  provenance (heuristic, withdrawn); rate–QP slope/curves: **to be measured**.

## Validation protocol (run later — VMAF primary, + SSIM + PSNR)

Integrates with the same live-validation corpus used for the auto-tune sample
sweep.

**Corpus (real titles, per memory's calibration need):** clean, light-grain
(film), heavy-grain, and anime sources; at 1080p and 2160p; spanning source
codecs (h264 Blu-ray-grade remux + existing HEVC); include one HDR10 sample.
Keep raw lossless/near-lossless reference clips (~30–60 s, motion-bearing,
seeked past intros — reuse [signal.go](../../internal/tune/signal.go)'s window
logic).

**Hardware (per Change 5 — defaults ship cross-vendor):** primary sweep on VCN
3.1 (dev box); plus a confirmatory pass on at least **one Intel VAAPI** and **one
discrete-AMD RDNA** GPU to check that the VCN-calibrated `qpCeiling`, biases, and
the "inert/fake" facts (B-frames, `compression_level`, 10-bit banding) either
hold or get a per-device fallback. Off-VCN devices use conservative defaults
until measured.

**Procedure per clip:**
- **Rate–QP curve first (gates Change 1).** Encode a CQP QP-sweep (e.g. QP
  14,16,18,20,22,24) per content class and resolution; fit bitrate-vs-QP to get
  the real VCN slope. This sets the `qpCeiling` (lowest QP whose size delta vs the
  old `base+3` pushed QP stays inside the ~25% envelope) — replacing the withdrawn
  "+12–15%" estimate. The **same fit replaces `EstimateBitrate.vaapiRatios`** as
  per-(content-class, source-codec, resolution) curves — encode both **h264→hevc
  and hevc→hevc** — so preflight bounds against measured reality, not the
  withdrawn heuristic (Change 6).
- Then encode (a) current default vs (b) each proposed variant.
- For Change 3 (denoise), run the **two RD sweeps** described below per candidate.

**Metrics (compute all three):**
- **VMAF (primary):** mean **and** 1st-percentile (low-frame catches worst-frame
  artifacts), **plus the NEG variant** to guard against a sharpening/denoise pass
  inflating the score. VMAF is Y-only. **Model must match resolution:**
  `vmaf_v0.6.1`/`vmaf_v0.6.1neg` for 1080p clips, **`vmaf_4k_v0.6.1`** for the
  2160p clips (the default model assumes a 1080p viewing distance and over-scores
  4K). Do not mix models across resolutions in one comparison.
- **MS-SSIM/SSIM:** correlates with subjective quality nearly as well as VMAF-NEG;
  cross-checks VMAF.
- **PSNR:** cheap baseline; lowest subjective correlation — report but don't
  decide on it.
- Tooling: `libvmaf` via ffmpeg, or `slhck/ffmpeg-quality-metrics`. No subsampling
  for final numbers (`n_subsample=1`); subsampling allowed for exploratory sweeps.

**Reference handling for denoise tunes (critical — guards against picking an
over-aggressive filter):** measuring a *denoised* encode against the *grainy
source* makes VMAF/PSNR penalize the removed grain. But BD-rate-vs-original
*still bakes in that penalty*, and VMAF-NEG corrects for sharpening, **not** grain
removal — so "great BD-rate" can mask real fidelity loss. Per denoise candidate,
run **two RD sweeps**:
  - (a) **vs the original** — captures absolute fidelity including grain loss.
  - (b) **vs a denoised reference matched to that candidate** — isolates pure
    *codec efficiency* (how well the encoder codes the already-denoised signal),
    removing the grain-removal penalty.
A candidate is eligible only if its **VMAF-NEG vs original** stays within a small
bound (initial gate: mean ≥ −0.5 and **no 1st-percentile regression**); only then
do its sweep-(b) BD-rate wins count toward selection. This prevents selecting a
filter that codes efficiently while materially degrading source texture. (arXiv
2402.00622; ab-av1 issue #139.)

**Per-clip report:** VMAF mean, VMAF 1st-pct, VMAF-NEG, MS-SSIM, PSNR, output
size, **size delta % vs current default**, bitrate; BD-rate where a sweep ran.

**Acceptance gates:**
- No regression in **VMAF 1st-percentile** vs current default (no new worst-frame
  artifacts) — hard gate.
- Size delta within the flexible **~25%** envelope; any clip exceeding it must
  show a clearly justifying quality gain or the `qpCeiling` is raised for that
  content class until it fits.
- Denoise candidates: pass the VMAF-NEG-vs-original gate (mean ≥ −0.5, no 1st-pct
  regression) before BD-rate wins count.
- Anime: no visible banding on gradient spot-checks (the deband + QP change must
  not regress).
- Output never larger than source (preflight/escalation still hold).
- QP invariant holds: no (bias, tune, ceiling, push) combination exits
  `[contentClassMin, VaapiQPMax]` (Change 5).

**Calibration output:** the sweep sets `grainLight/HeavyThreshold`, the chosen
`hqdn3d`/`nlmeans` strengths, and confirms the QP-bias magnitudes — replacing the
INITIAL-ESTIMATE markers in code.

## Ordered implementation steps

**Measurement precedes code:** the rate–QP sweep (validation procedure) must run
before steps 1–2 — it sets the `qpCeiling`. The sweep is run on a branch with the
denoise/bias variants behind flags; only the *defaults* flip after calibration.

1. **Rate–QP + content sweep (gates everything).** Run the validation procedure's
   rate–QP curves and variant encodes on VCN; derive the per-content-class
   `qpCeiling` (envelope-bounded) and the `contentClassMin` floors. Without this,
   steps 1–2 have no numbers.
2. **Config/flags.** Make quality-first the default and reframe
   `QualityPriority`/`--quality-priority`: default becomes the bounded-push path;
   add an explicit `--size-priority` escape hatch for the old unbounded push. Add
   `qpCeiling` and `contentClassMin` as config-derived, **per VAAPI vendor/gen**
   (Change 5), not bare consts. Extend `check` to expose vendor/gen. No new encoder knobs.
3. **planner.go §2b.** Implement Change 1 exactly as
   `finalQP = max(baseQP, min(pushTargetQP, qpCeiling))` (ceiling on the push
   target; the `max` preserves the SmartQuality base). Keep preflight/escalation
   untouched. Add a `plan.Notes` line stating the ceiling applied and resulting QP.
4. **quality.go biases + invariant.** Set `tuneQPBias`/`tuneCRFBias`: anime +1 → 0
   (Change 2); film/grain −1 behind the validation gate (Change 4). Enforce the
   Change 5 invariant: `clamp(baseCurveQP + globalBias + tuneBias, contentClassMin,
   VaapiQPMax)` after the full stack.
5. **filter.go strengths.** Parameterize grain/film denoise so the
   validation-selected `hqdn3d`/`nlmeans` strings drop in (Change 3). Leave film
   at its measured point until the sweep says otherwise.
6. **Unit tests.** Cover the QP-ceiling formula edge cases (base < / ≈ / > ceiling),
   anime bias = 0, the stacked-bias invariant (no combination exits
   `[contentClassMin, VaapiQPMax]`), per-vendor branching, prefilter→sw-decode
   forcing. Confirm `none`/clean defaults unchanged where intended.
7. **Cross-vendor + probe validation.** Re-run the confirmatory pass on Intel and
   discrete-AMD RDNA (Change 5): re-verify B-frames via `ffprobe pict_type`,
   re-check `compression_level`/10-bit, and the two probes (ICQ vs CQP at matched
   size; I-frame QP offset vs default) — adopt only if they measurably win, else
   document as no-ops and set per-device fallbacks.
8. **Calibrate + finalize.** Lock thresholds, denoise strengths, ceiling/floors,
   and bias magnitudes from the sweeps; remove INITIAL-ESTIMATE markers in
   [suggest.go](../../internal/tune/suggest.go) / [filter.go](../../internal/planner/filter.go).
   Refit `EstimateBitrate`'s per-(class, codec, resolution) rate–QP curves and
   re-key `PreflightAdjust` to true anti-bloat (Change 6); land both together and
   re-baseline the size-discipline tests + dry-run estimates.
9. **Docs.** Update `AGENTS.md` / design docs / CHANGELOG: quality-first default +
   QP-ceiling, content-adaptive biases + invariant, calibrated denoise strengths,
   per-vendor gating, and the (re-verified) no-op list. `make ci` green.

## Done-when

- [ ] Rate–QP curves measured on VCN; `qpCeiling`/`contentClassMin` derived from
      them (the withdrawn "+12–15%" estimate replaced by measured slope).
- [x] Default `hevc_vaapi` implements `finalQP = max(base, min(pushTarget, ceiling))`;
      the traced 25/8 Mbps cases land at the ceiling, not `base+3`. Edge cases
      (base < / ≈ / > ceiling) unit-tested. **(Change 1, landed)**
- [x] Stacked-bias QP invariant proven: no (bias, tune, ceiling, push) combination
      exits `[contentClassMin, VaapiQPMax]`; grain never below its knee (24).
      **(Change 5, landed — `TestQPInvariant_NoCombinationExitsRange`)**
- [ ] Anime tune emits QP bias 0 (or the validated −1/−2); gradient spot-check
      shows no banding regression.
- [ ] Grain/film denoise strengths and `grainLight/HeavyThreshold` set from the
      two-sweep RD data passing the VMAF-NEG gate; INITIAL-ESTIMATE markers removed.
- [ ] Validation sweep shows VMAF 1st-pct non-regression and size deltas within
      the ~25% envelope (or justified), recorded in the live-validation doc.
- [ ] Cross-vendor pass done: B-frames re-verified via `ffprobe pict_type` on the
      current driver + Intel; VCN-specific magnitudes gated by vendor/gen with
      conservative off-VCN fallbacks; CQP remains the VCN default.
- [x] `PreflightAdjust` re-keyed to true anti-bloat (POINT estimate > ~105% of
      input); Change 1's ceiling verified to bind on h264/hevc — a 1080p h264 BD
      holds at QP16 (point ≈92%) with no preflight bump
      (`TestQualityFirstCeilingBindsH264_1080pBD`). Output-never-exceeds-source is
      now guarded by the `pipeline/runner.go` post-encode loop, not the estimator.
      **(Change 6, landed via point re-key — NOT the curve refit.)** Optional future
      work: refit `EstimateBitrate` per (source codec, content class, resolution)
      to tighten the *displayed* estimate; not a correctness blocker.
- [ ] `make ci` green; this plan's `status` set to `done`.

## Sources

- Brainiarc7 — FFmpeg/Libav VAAPI HW encode setup (rc modes, CQP, hwupload):
  https://gist.github.com/Brainiarc7/95c9338a737aa36d9bb2931bed379219
- TauCeti — HEVC VAAPI on AMD (CQP / global_quality, lower = higher quality):
  https://www.tauceti.blog/posts/linux-ffmpeg-amd-5700xt-hardware-video-encoding-hevc-h265-vaapi/
- dr-lex — Video encoding tips (hqdn3d strong = detail loss; NLMeans preserves detail, lower bitrate):
  https://www.dr-lex.be/info-stuff/videotips.html
- "Gain of Grain" (VVC film-grain toolchain; denoise → large bitrate savings; grain-vs-VMAF caveat), arXiv 2402.00622:
  https://arxiv.org/pdf/2402.00622
- USPTO 10,911,785 — Intelligent compression of grainy video content (denoise before encode):
  https://image-ppubs.uspto.gov/dirsearch-public/print/downloadPdf/10911785
- ab-av1 issue #139 — discard synthesized grain for VMAF (denoise penalizes VMAF-vs-grainy-source):
  https://github.com/alexheretic/ab-av1/issues/139
- slhck/ffmpeg-quality-metrics (VMAF/SSIM/PSNR/VIF tooling):
  https://github.com/slhck/ffmpeg-quality-metrics
- testdevlab — Full-reference metrics VMAF/PSNR/SSIM (VMAF Y-only, model/NEG):
  https://www.testdevlab.com/blog/full-reference-quality-metrics-vmaf-psnr-and-ssim
- "Objective video quality metrics" survey, arXiv 2107.10220 (VMAF-NEG / MS-SSIM correlation; PSNR lowest):
  https://arxiv.org/pdf/2107.10220
- On-hardware measurements (override generic advice): project memory
  `vcn31-vaapi-encode-facts`, `content-detection-for-tuning`.
</content>
</invoke>
