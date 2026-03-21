# Quality system

Architecture of the smart quality pipeline — how Muxmaster selects per-file
QP/CRF values and prevents output size blowups.

Design principle: **quality over compression**. Accept mild size overshoot
rather than destroying quality. The post-encode escalation loop handles
genuine blowups.

---

## Pipeline stages

```
1. SmartQuality        (planner/quality.go)     → base QP/CRF from curves
2. OptimalBitrate      (planner/estimation.go)   → target output kbps
3. QPForTargetBitrate  (planner/estimation.go)   → QP for that target
4. Optimal override    (planner/planner.go)      → capped merge with SmartQuality
5. PreflightAdjust     (planner/estimation.go)   → bump if estimate > 105% of input
6. Post-encode escal.  (pipeline/runner.go)      → re-encode if output > input
```

### Stage 1: SmartQuality

Computes per-file QP and CRF by summing three adjustment curves, each keyed
on a different input signal:

| Curve | Input signal | VAAPI range | CPU range |
|-------|-------------|-------------|-----------|
| Resolution curve | pixels (W×H) | -2 to +3 | -2 to +4 |
| Bitrate curve | kbps | -2 to +2 | -2 to +2 |
| Density curve | kbps/Mpx | -2 to +4 | -1 to +3 |

Formula: `QP = Clamp(default + adjustment + SmartQualityBias, QPMin, QPMax)`

SmartQualityBias defaults to -2 (favors quality / lower QP).

### Stage 2: OptimalBitrate

Computes a target output bitrate based on codec generation gain:

| Source codec | Base ratio | Rationale |
|-------------|-----------|-----------|
| h264 → HEVC | 68% | Typical generational compression gain |
| HEVC → HEVC | 95% | Minimal gain from re-encoding same codec |
| VP9, AV1 | 90% | Already efficient modern codecs |
| MPEG2, MPEG4, VC1 | 45% | Large gain from legacy codecs |

Density adjustments shift the ratio (up to +30 for ultra-compressed sources,
down to -8 for premium quality).

### Stage 3: QPForTargetBitrate

Searches VaapiQPMin–VaapiQPMax for the QP whose estimated output midpoint
is closest to the target bitrate from Stage 2.

### Stage 4: Optimal override (planner.go)

Merges SmartQuality QP with the optimal-bitrate QP:

```
finalQP = max(smartQP, min(targetQP, smartQP + 3))
```

The +3 cap prevents the estimation model from overriding the quality curves
by a huge margin. SmartQuality is the primary driver; optimal bitrate is a
soft nudge.

### Stage 5: PreflightAdjust

Iteratively bumps QP/CRF until the estimated output high-end ≤ 105% of
input bitrate. Capped at 4 bumps to prevent the estimator from over-correcting
on files where the model is unreliable.

The 105% target (vs 100%) avoids chasing marginal overshoot at the cost of
quality — the post-encode loop handles genuine blowups.

### Stage 6: Post-encode escalation

After a successful encode, if the output file is larger than the input
(smart quality enabled, no manual override):

1. Bump QP by 1 (qualityBumpStep)
2. Delete output, re-encode
3. Repeat up to 2 times (maxQualityBumps)

This is the last-resort safety net. The preflight stages handle most cases.

---

## Estimation model

The bitrate estimation in `EstimateBitrate` predicts output size using:

```
ratio = vaapiRatio(QP)          # QP-to-ratio lookup table
      + codecBias(sourceCodec)  # h264=+100, hevc=+180, mpeg2=-60
      + resolutionBias(pixels)  # SD=+80, 4K=-40
      + bitrateBias(kbps)       # <1500=+120, >30000=-50
      + densityBias(kbps, px)   # <1000=+250, >10000=-20

ratio = Clamp(ratio, 220, 1050)

lowEstimate  = input × ratio × 75%
highEstimate = input × ratio × 130%
```

The ratio is in per-mille (e.g. 770 means output ≈ 77% of input).

---

## Constants reference

| Constant | Value | Location | Purpose |
|----------|-------|----------|---------|
| VaapiQPMin | 14 | quality.go | Lowest allowed QP (highest quality) |
| VaapiQPMax | 30 | quality.go | Highest allowed QP (QP >30 = severe artifacts) |
| CpuCRFMin | 16 | quality.go | Lowest allowed CRF |
| CpuCRFMax | 30 | quality.go | Highest allowed CRF |
| SmartQualityBias | -2 | config.go | Negative = favor quality |
| maxOptimalOverride | 3 | planner.go | Cap on optimal bitrate QP override |
| PreflightAdjust maxBumps | 4 | estimation.go | Max preflight bump iterations |
| PreflightAdjust target | 105% | planner.go | Overshoot tolerance |
| qualityBumpStep | 1 | runner.go | Post-encode QP increment |
| maxQualityBumps | 2 | runner.go | Max post-encode re-encodes |
| highRatio multiplier | 130% | estimation.go | Pessimistic estimate factor |

### Density thresholds (kbps per megapixel)

| Threshold | Range | Meaning |
|-----------|-------|---------|
| < 1000 | densityUltraLow | Heavily compressed (streaming rips, web-dl) |
| < 1500 | densityLow | Below average for resolution |
| < 2500 | densityBelowAvg | Slightly below typical |
| < 3500 | densityMedium | Average for resolution |
| > 8000 | densityHigh | High quality source (Blu-ray) |
| > 10000 | densityVeryHigh | Premium quality (remux grade) |
