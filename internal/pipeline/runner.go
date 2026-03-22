// runner.go implements per-file processing, quality escalation, and batch orchestration.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/ffmpeg"
	"github.com/backmassage/muxmaster/internal/naming"
	"github.com/backmassage/muxmaster/internal/planner"
	"github.com/backmassage/muxmaster/internal/probe"
	"github.com/backmassage/muxmaster/internal/term"
)

const minFileSize = 1000

// Run is the top-level batch entry point. It discovers files, builds the
// TV year-variant index, processes each file sequentially, and returns
// aggregate stats. The run parameter controls how ffmpeg subprocesses are
// launched; production callers pass ffmpeg.NewRunFunc, tests pass a mock.
func Run(ctx context.Context, cfg *config.Config, log Logger, run ffmpeg.RunFunc) RunStats {
	var stats RunStats

	files, err := Discover(cfg.InputDir)
	if err != nil {
		log.Error("File discovery failed: %v", err)
		return stats
	}

	stats.Total = len(files)
	yearIndex := naming.BuildYearVariantIndex(files)
	resolver := naming.NewCollisionResolver()

	logBatchHeader(cfg, log, &stats)

	for i, path := range files {
		stats.Current = i + 1

		if ctx.Err() != nil {
			log.Warn("Interrupted")
			break
		}

		processFile(ctx, cfg, log, path, &stats, yearIndex, resolver, run)
	}

	logSummary(cfg, log, &stats)
	return stats
}

// processFile handles one media file: validate → probe → name → plan → execute.
func processFile(
	ctx context.Context,
	cfg *config.Config,
	log Logger,
	path string,
	stats *RunStats,
	yearIndex naming.YearVariantIndex,
	resolver *naming.CollisionResolver,
	run ffmpeg.RunFunc,
) {
	basename := filepath.Base(path)
	log.Info("[%d/%d] %s%s%s", stats.Current, stats.Total, term.Cyan, basename, term.NC)

	// --- Validate ---
	fi, err := os.Stat(path)
	if err != nil {
		log.Error("File not found: %s", path)
		stats.Failed++
		log.Blank()
		return
	}
	if fi.Size() < minFileSize {
		log.Error("File too small (possibly corrupt): %s", path)
		stats.Failed++
		log.Blank()
		return
	}

	// --- Probe (single JSON call replaces ~10 legacy ffprobe invocations) ---
	pr, err := probe.Probe(ctx, path)
	if err != nil {
		log.Error("Cannot probe file (possibly corrupt): %v", err)
		stats.Failed++
		log.Blank()
		return
	}

	if pr.PrimaryVideo == nil {
		log.Warn("No video stream found, skipping")
		stats.Skipped++
		log.Blank()
		return
	}

	v := pr.PrimaryVideo
	if v.Width <= 0 || v.Height <= 0 {
		log.Error("Invalid video dimensions (%dx%d), skipping", v.Width, v.Height)
		stats.Failed++
		log.Blank()
		return
	}

	logInputMeta(log, pr)

	// --- Parse filename and resolve output path ---
	parsed := naming.ParseFilename(basename, filepath.Dir(path))
	if parsed.MediaType == naming.MediaTV {
		orig := parsed.ShowName
		parsed.ShowName = naming.HarmonizeShowName(parsed.ShowName, yearIndex)
		if parsed.ShowName != orig {
			log.Debug(cfg.Display.Verbose, "Harmonized show name: '%s' -> '%s'", orig, parsed.ShowName)
		}
	}

	container := string(cfg.OutputContainer)
	outputPath := naming.GetOutputPath(parsed, cfg.OutputDir, container)
	outputPath = resolver.Resolve(path, outputPath)

	// --- Log file stats ---
	logBitrateOutlier(log, pr)

	// --- Build plan ---
	plan := planner.BuildPlan(cfg, pr)
	plan.InputPath = path
	plan.OutputPath = outputPath

	if cfg.Display.FileStats {
		logFileStats(log, plan)
	}

	if plan.QualityNote != "" {
		if strings.Contains(plan.QualityNote, "not browser-safe") {
			log.Warn("  %s", plan.QualityNote)
		} else {
			log.Debug(cfg.Display.Verbose, "  Quality: %s", plan.QualityNote)
		}
	}

	// --- Skip-existing check ---
	if cfg.SkipExisting {
		if _, err := os.Stat(outputPath); err == nil {
			log.Warn("Skip (exists): %s", filepath.Base(outputPath))
			stats.Skipped++
			log.Blank()
			return
		}
	}

	// --- Log action ---
	actionLabel := "Encoding"
	if plan.Action == planner.ActionRemux {
		switch {
		case plan.Audio.NoAudio:
			actionLabel = "Remuxing (copy HEVC, no audio)"
		case plan.Audio.CopyAll:
			actionLabel = "Remuxing (copy HEVC, copy audio)"
		default:
			actionLabel = fmt.Sprintf("Remuxing (copy HEVC, encode non-AAC audio via %s)", cfg.Audio.Encoder)
		}
	}
	log.Info("%s: %s", actionLabel, basename)
	log.Info("  -> %s", filepath.Base(outputPath))
	logAudioBitrates(log, pr, plan)

	// --- Dry-run ---
	if cfg.DryRun {
		if plan.Action == planner.ActionRemux {
			log.Success("[DRY] Would remux")
		} else {
			log.Success("[DRY] Would encode")
		}
		stats.Encoded++
		log.Blank()
		return
	}

	// --- Create output directory ---
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		log.Error("Cannot create output directory: %v", err)
		stats.Failed++
		log.Blank()
		return
	}

	// --- Execute with retry ---
	start := time.Now()
	rs := ffmpeg.NewRetryState(plan)
	ok := executeWithRetry(ctx, cfg, log, plan, rs, run)

	if !ok {
		if plan.Action == planner.ActionRemux {
			log.Error("Remux failed")
		} else {
			log.Error("Encode failed")
		}
		os.Remove(outputPath)
		stats.Failed++
		log.Blank()
		return
	}

	// --- Update stats ---
	elapsed := time.Since(start)
	inSize := fi.Size()
	var outSize int64
	if outInfo, err := os.Stat(outputPath); err == nil {
		outSize = outInfo.Size()
	}

	ratio := int64(100)
	if inSize > 0 {
		ratio = outSize * 100 / inSize
	}

	stats.TotalInputBytes += inSize
	stats.TotalOutputBytes += outSize
	stats.Encoded++

	if plan.Action == planner.ActionRemux {
		log.Success("Remuxed in %ds (%d%% of original)", int(elapsed.Seconds()), ratio)
	} else {
		log.Success("Encoded in %ds (%d%% of original)", int(elapsed.Seconds()), ratio)
	}
	log.Blank()
}

const (
	maxQualityBumps = 2
	qualityBumpStep = 1
)

// executeWithRetry runs ffmpeg with the error-retry inner loop, then checks
// for output size overshoot. If the encode produces a file larger than the
// input (smart quality enabled, no manual override), QP/CRF is bumped and
// the encode is re-attempted up to maxQualityBumps times.
func executeWithRetry(
	ctx context.Context,
	cfg *config.Config,
	log Logger,
	plan *planner.FilePlan,
	rs *ffmpeg.RetryState,
	run ffmpeg.RunFunc,
) bool {
	if ctx.Err() != nil {
		return false
	}

	if !attemptWithErrorRetry(ctx, cfg, log, plan, rs, run) {
		return false
	}

	if plan.Action != planner.ActionEncode {
		return true
	}

	canEscalate := cfg.Encoder.SmartQuality && cfg.Encoder.ActiveQualityOverride == ""
	bumpsApplied := 0

	for bump := 0; bump < maxQualityBumps && canEscalate; bump++ {
		pct, ok := outputPct(plan)
		if !ok || pct <= 100 {
			break
		}

		if cfg.Encoder.Mode == config.EncoderVAAPI {
			next := rs.VaapiQP + qualityBumpStep
			if next > planner.VaapiQPMax {
				log.Warn("Output larger than input (%d%%) — QP %d already at max", pct, rs.VaapiQP)
				break
			}
			rs.VaapiQP = next
			log.Warn("Output larger than input (%d%%), re-encoding at QP %d", pct, rs.VaapiQP)
		} else {
			next := rs.CpuCRF + qualityBumpStep
			if next > planner.CpuCRFMax {
				log.Warn("Output larger than input (%d%%) — CRF %d already at max", pct, rs.CpuCRF)
				break
			}
			rs.CpuCRF = next
			log.Warn("Output larger than input (%d%%), re-encoding at CRF %d", pct, rs.CpuCRF)
		}

		os.Remove(plan.OutputPath)
		rs.Attempt = 0
		bumpsApplied++

		if ctx.Err() != nil {
			return false
		}
		if !attemptWithErrorRetry(ctx, cfg, log, plan, rs, run) {
			return false
		}
	}

	if pct, ok := outputPct(plan); ok && pct > 100 {
		if bumpsApplied > 0 {
			log.Warn("Output still larger than input (%d%%) after %d quality bump(s)", pct, bumpsApplied)
		} else {
			log.Warn("Output larger than input (%d%% of original)", pct)
		}
	}

	return true
}

func outputPct(plan *planner.FilePlan) (int, bool) {
	outInfo, err := os.Stat(plan.OutputPath)
	if err != nil {
		return 0, false
	}
	inInfo, err := os.Stat(plan.InputPath)
	if err != nil || inInfo.Size() <= 0 {
		return 0, false
	}
	return int(outInfo.Size() * 100 / inInfo.Size()), true
}

// attemptWithErrorRetry runs the inner retry loop: execute ffmpeg, classify
// stderr on failure, apply the first matching fix, and retry. Returns true
// if ffmpeg eventually succeeds.
func attemptWithErrorRetry(
	ctx context.Context,
	cfg *config.Config,
	log Logger,
	plan *planner.FilePlan,
	rs *ffmpeg.RetryState,
	run ffmpeg.RunFunc,
) bool {
	retryLabels := map[ffmpeg.RetryAction]string{
		ffmpeg.RetryDropAttach:    "skip attachments",
		ffmpeg.RetryDropSubs:      "skip subtitles",
		ffmpeg.RetryIncreaseMux:   "increase mux queue",
		ffmpeg.RetryFixTimestamps: "fix timestamps",
	}

	for {
		result := ffmpeg.Execute(ctx, cfg, plan, rs, run)
		if result.Err == nil {
			return true
		}

		// Stop retrying if the context has been cancelled (e.g. SIGINT).
		if ctx.Err() != nil {
			log.Warn("Interrupted, aborting retries")
			return false
		}

		if cfg.StrictMode {
			log.Error("ffmpeg failed (strict mode, no retry)")
			logStderr(log, result.Stderr)
			return false
		}

		action := rs.Advance(result.Stderr)
		if action == ffmpeg.RetryNone {
			log.Error("ffmpeg failed (no applicable retry)")
			logStderr(log, result.Stderr)
			return false
		}

		log.Warn("Retry %d: %s", rs.Attempt, retryLabels[action])
		os.Remove(plan.OutputPath)
	}
}
