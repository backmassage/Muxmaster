# Per-Series Auto-Tune (`--tune auto`)

**Status:** done (code + tests + docs landed; grain thresholds remain initial estimates pending a real-Blu-ray sweep)
**Goal:** Make the default run choose a content-aware prefilter without the user hand-specifying `--tune` per title. A cheap ffmpeg grain pre-pass produces a *suggested* profile per **series** (grouped across all seasons/episodes); the user is prompted **once per series** to confirm/override, and that choice applies to every episode of that series. Grain detection is reliable enough to auto-suggest; film-vs-anime is not (deep research: no robust classifier exists), so the human makes that call via the prompt.
**Non-goals:** A trained ML classifier; non-interactive auto-apply of denoise (non-TTY runs fall back to `none` — safe, unattended-clean); per-scene (vs per-series) tuning; calibration of final grain thresholds on real Blu-rays (ship initial estimates + a calibration note); deband-strength auto-detection (banding metric is an open question).

## Context digest

Deep research (105-agent harness, adversarially verified) + local ffmpeg experiments on this box established:
- **Grain is auto-detectable** via the encoder-canonical *denoise-residual on flat regions* (AV1 spec / SVT-AV1). A cheaper, edge-robust proxy that works here: `bitplanenoise` on the LSB (bitplane 1). Local measurement: flat 0.00, smooth gradient 0.04, grainy 0.95 — clean separation, sub-second on a downscaled/thinned sample.
- **`signalstats` TOUT is a poor grain detector** (built for analog VHS speckle; false-positives on high-contrast detail) — avoid for grain.
- **Anime-vs-live-action is NOT robustly automatable** — no off-the-shelf classifier (MobileNet "animation" label refuted). Best published heuristic (US 6,810,144: animation = fewer/larger flat regions) is medium-confidence with no production thresholds. → user confirms via prompt.
- **Cost is negligible**: sample a ~20–30s window at 2 fps, downscaled to ~480px, one representative file per series.
- **Open question**: concrete clean/light/heavy cut points and their hqdn3d strengths need an empirical sweep on real titles. We ship initial estimates.

Pipeline seam: `pipeline.Run` discovers files → loops `processFile` (probe → `naming.ParseFilename` → `planner.BuildPlan`). `planner.TunePrefilter(cfg)` reads `cfg.Encoder.Tune`. The planner already treats unknown/`none` tune as no prefilter.

## Files to touch

| File | Change |
|---|---|
| `internal/config/config.go` | `TuneAuto` const; default `Tune` → `TuneAuto`; `Validate` accepts auto |
| `internal/config/flags.go` | `tuneValue` accepts `auto`; help text |
| `internal/planner/filter.go` | `TunePrefilter` returns `""` for `TuneAuto` (sentinel; pipeline resolves it before planning) |
| `internal/tune/` (new pkg) | `signal.go` (`DetectContent` ffmpeg pre-pass + pure parsers), `suggest.go` (`Suggest` metric→TuneMode + confidence) |
| `internal/pipeline/autotune.go` (new) | series grouping, representative pick, run pre-pass, per-series TTY prompt → `map[seriesKey]TuneMode` |
| `internal/pipeline/runner.go` | call `resolveSeriesTunes` before the loop (only when `Tune==auto` && stdin is a TTY); thread per-file tune into `processFile` via a cfg copy |
| `AGENTS.md`, `_docs/architecture.md`, `_docs/design/structure.md`, `CHANGELOG.md` | document the new package, dep edge `pipeline → tune → config`, the auto default + TTY gating |

## Ordered steps

1. **Config/flags.** Add `TuneAuto = "auto"`; `DefaultConfig` sets `Tune: TuneAuto`; `Validate` + `tuneValue.Set` accept it; help text. `TunePrefilter(TuneAuto)` → `""` (planner no-op; regression-safe — default planner output unchanged).
2. **`internal/tune` package** (depends on `config` only). `ContentSignal{Grain, EdgeDensity float64, Frames int}`. `DetectContent(ctx, path string, seekSec, windowSec int) (ContentSignal, error)` runs `ffmpeg -ss <seek> -t <window> -i path -an -vf "fps=2,scale=480:-2,format=yuv420p,bitplanenoise=bitplane=1,signalstats,metadata=mode=print:file=-" -f null -` and averages the per-frame metrics. Pure helpers `parseMetadataFloats(out, key) []float64`, `mean([]float64) float64` (unit-tested). `Suggest(sig) (config.TuneMode, Confidence, string)`: grain `<0.20→none`, `0.20–0.55→film`, `>0.55→grain`; a very-clean + low-edge frame yields a low-confidence `anime` suggestion (user confirms). Thresholds are `const` and documented as initial estimates.
3. **Pipeline grouping + prompt.** `autotune.go`: `seriesKey(parsed)` (`tv:<show>` / `movie:<name><year>` / `file:<base>`); `resolveSeriesTunes(ctx, cfg, log, files) map[string]config.TuneMode` groups files, picks the first per group as representative, probes it for duration, calls `tune.DetectContent` (seek ≈20%), `tune.Suggest`, then prompts once per series (reads stdin) with the suggestion pre-filled. Gate the whole thing on `cfg.Encoder.Tune==TuneAuto && term.IsTerminal(os.Stdin)`; otherwise return empty map (→ files fall back to `none`).
4. **Thread into processFile.** `Run` computes `seriesTunes` before the loop; `processFile` computes the file's key, looks up the resolved tune (default `none`), and plans with a `cfg` copy whose `Encoder.Tune` is set to it (shallow copy — only Tune changes; sub-structs are value types).
5. **Tests.** `tune`: table-drive `parseMetadataFloats`/`mean`/`Suggest` (grain bands → tune; clean+flat → anime-low-conf). `pipeline`: `seriesKey` grouping; `resolveSeriesTunes` returns empty when non-TTY (so existing pipeline tests are unaffected); per-file tune lookup applied to the plan. Confirm `--tune none/film/grain/anime` still bypass the pre-pass entirely.
6. **Docs + `make ci`.**

## Done-when

- [x] `--tune auto` (default) on an interactive run groups by series, runs one grain pre-pass per series, prompts once, and applies the choice to every episode. (`resolveSeriesTunes` + `processFile` threading; `groupBySeries`/`promptSeriesTune` unit-tested.)
- [x] Non-interactive (no TTY) `--tune auto` run makes no pre-pass/prompt and behaves as `--tune none` (gated on `term.IsTerminal(os.Stdin)`; all 475 existing tests pass unchanged).
- [x] Explicit `--tune none|film|grain|anime` skips the pre-pass/prompt and applies globally (the auto gate only fires for `TuneAuto`).
- [x] `tune.Suggest` unit-asserts grain→profile bands; `parseMetadataFloats`/`mean`/`seekSeconds` table-tested against canned ffmpeg metadata output.
- [x] Grain pre-pass validated end-to-end against synthetic grainy (0.997→grain) vs clean (0.091→none) encoded samples — suggestion flips correctly.
- [x] Dep map updated (`AGENTS.md`, `architecture.md`): `pipeline → tune → config`; `tune` is near-leaf. `make ci` green.
- [ ] **Remaining:** calibrate `grainLightThreshold`/`grainHeavyThreshold` and the `hqdn3d` strengths on real Blu-ray titles (open question — current values are initial estimates).
