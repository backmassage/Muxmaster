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
	"github.com/backmassage/muxmaster/internal/display"
	"github.com/backmassage/muxmaster/internal/ffmpeg"
	"github.com/backmassage/muxmaster/internal/logging"
	"github.com/backmassage/muxmaster/internal/naming"
	"github.com/backmassage/muxmaster/internal/planner"
	"github.com/backmassage/muxmaster/internal/probe"
	"github.com/backmassage/muxmaster/internal/term"
)

const minFileSize = 1000

// Run is the top-level batch entry point. It discovers files, builds the
// TV year-variant index, processes each file sequentially, and returns
// aggregate stats.
func Run(ctx context.Context, cfg *config.Config, log *logging.Logger) RunStats {
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

		processFile(ctx, cfg, log, path, &stats, yearIndex, resolver)
	}

	logSummary(cfg, log, &stats)
	return stats
}

// processFile handles one media file: validate → probe → name → plan → execute.
func processFile(
	ctx context.Context,
	cfg *config.Config,
	log *logging.Logger,
	path string,
	stats *RunStats,
	yearIndex naming.YearVariantIndex,
	resolver *naming.CollisionResolver,
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
			log.Debug(cfg.Verbose, "Harmonized show name: '%s' -> '%s'", orig, parsed.ShowName)
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

	if cfg.ShowFileStats {
		logFileStats(log, plan)
	}

	if plan.QualityNote != "" {
		if strings.Contains(plan.QualityNote, "not browser-safe") {
			log.Warn("  %s", plan.QualityNote)
		} else {
			log.Debug(cfg.Verbose, "  Quality: %s", plan.QualityNote)
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
			actionLabel = fmt.Sprintf("Remuxing (copy HEVC, encode non-AAC audio via %s)", cfg.AudioEncoder)
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
	ok := executeWithRetry(ctx, cfg, log, plan, rs)

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
	log *logging.Logger,
	plan *planner.FilePlan,
	rs *ffmpeg.RetryState,
) bool {
	if ctx.Err() != nil {
		return false
	}

	if !attemptWithErrorRetry(ctx, cfg, log, plan, rs) {
		return false
	}

	if plan.Action != planner.ActionEncode {
		return true
	}

	canEscalate := cfg.SmartQuality && cfg.ActiveQualityOverride == ""
	bumpsApplied := 0

	for bump := 0; bump < maxQualityBumps && canEscalate; bump++ {
		pct, ok := outputPct(plan)
		if !ok || pct <= 100 {
			break
		}

		if cfg.EncoderMode == config.EncoderVAAPI {
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
		if !attemptWithErrorRetry(ctx, cfg, log, plan, rs) {
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
	log *logging.Logger,
	plan *planner.FilePlan,
	rs *ffmpeg.RetryState,
) bool {
	retryLabels := map[ffmpeg.RetryAction]string{
		ffmpeg.RetryDropAttach:    "skip attachments",
		ffmpeg.RetryDropSubs:      "skip subtitles",
		ffmpeg.RetryIncreaseMux:   "increase mux queue",
		ffmpeg.RetryFixTimestamps: "fix timestamps",
	}

	for {
		result := ffmpeg.Execute(ctx, cfg, plan, rs)
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

const maxStderrLines = 20

func logStderr(log *logging.Logger, stderr string) {
	if stderr == "" {
		return
	}
	log.Error("Last ffmpeg output:")
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) > maxStderrLines {
		log.Error("  ... %d lines omitted ...", len(lines)-maxStderrLines)
		lines = lines[len(lines)-maxStderrLines:]
	}
	for _, l := range lines {
		log.Error("  %s", l)
	}
}

// --- Logging helpers ---

func logBatchHeader(cfg *config.Config, log *logging.Logger, stats *RunStats) {
	log.Info("Found %d files", stats.Total)

	profileLabel := cfg.CpuProfile
	qualityValue := cfg.CpuCRF
	if cfg.EncoderMode == config.EncoderVAAPI {
		profileLabel = cfg.VaapiProfile
		if profileLabel == "" {
			profileLabel = "main10"
		}
		qualityValue = cfg.VaapiQP
	}
	log.Info("Mode: %s (HEVC %s), QP/CRF: %d", cfg.EncoderMode, profileLabel, qualityValue)

	if cfg.ActiveQualityOverride != "" {
		if cfg.EncoderMode == config.EncoderVAAPI {
			log.Info("Quality mode: manual fixed override (VAAPI_QP=%s)", cfg.ActiveQualityOverride)
		} else {
			log.Info("Quality mode: manual fixed override (CPU_CRF=%s)", cfg.ActiveQualityOverride)
		}
	} else if cfg.SmartQuality {
		log.Info("Quality mode: smart per-file adaptation (mode-specific CPU/VAAPI curves)")
	} else {
		log.Info("Quality mode: fixed defaults")
	}

	log.Info("Container: %s", strings.ToUpper(string(cfg.OutputContainer)))
	log.Info("Audio: AAC passthrough, non-AAC encode to AAC via %s at %s", cfg.AudioEncoder, cfg.AudioBitrate)

	if cfg.OutputContainer == config.ContainerMP4 {
		log.Info("Compatibility: hvc1 tag for Apple/browser support")
	}
	if cfg.HandleHDR == config.HDRPreserve {
		log.Info("HDR: Preserve metadata when present")
	} else {
		log.Info("HDR: Tonemap to SDR")
	}
	if cfg.DeinterlaceAuto {
		log.Info("Deinterlace: Auto-detect and apply yadif")
	}
	if cfg.KeepSubtitles {
		if cfg.OutputContainer == config.ContainerMP4 {
			log.Info("Subtitles: Text subs only (mov_text for MP4)")
		} else {
			log.Info("Subtitles: Copy all streams")
		}
	}
	if cfg.KeepAttachments && cfg.OutputContainer != config.ContainerMP4 {
		log.Info("Attachments: Copy fonts/images")
	}
	if cfg.SkipHEVC {
		log.Info("HEVC sources: Remux (copy video, copy/encode audio)")
	}
	if cfg.StrictMode {
		log.Info("Retry policy: Strict mode (no auto-retry)")
	}
	log.Blank()
}

func logInputMeta(log *logging.Logger, pr *probe.ProbeResult) {
	v := pr.PrimaryVideo
	if v == nil {
		return
	}

	codec := strings.ToUpper(v.Codec)
	res := pr.Resolution()
	bitrate := display.FormatBitrateLabel(pr.VideoBitRate() / 1000)

	hdr := pr.HDRType()
	var flags []string
	if hdr == "hdr10" {
		flags = append(flags, "HDR10")
	}
	if pr.IsInterlaced() {
		flags = append(flags, "interlaced")
	}

	tag := fmt.Sprintf("%s[Input]%s", term.Magenta, term.NC)
	if len(flags) > 0 {
		log.Info("  %s %s | %s | %s | %s", tag, codec, res, bitrate, strings.Join(flags, ", "))
	} else {
		log.Info("  %s %s | %s | %s", tag, codec, res, bitrate)
	}
}

func logFileStats(log *logging.Logger, plan *planner.FilePlan) {
	if plan.Action == planner.ActionSkip {
		return
	}

	codec := plan.VideoCodec
	if codec == "" || codec == "copy" {
		log.Info("  Video: copy (remux)")
		return
	}

	method := "CPU"
	qLabel := fmt.Sprintf("CRF %d", plan.CpuCRF)
	if strings.Contains(codec, "vaapi") {
		method = "VAAPI"
		qLabel = fmt.Sprintf("QP %d", plan.VaapiQP)
	}

	if plan.PreflightBumps > 0 {
		log.Info("  Video: %s | %s (preflight +%d) | %s", codec, qLabel, plan.PreflightBumps, method)
	} else {
		log.Info("  Video: %s | %s | %s", codec, qLabel, method)
	}

	if plan.MaxRateKbps > 0 {
		log.Info("  Maxrate: %d kb/s (bitrate ceiling)", plan.MaxRateKbps)
	}

	if plan.OptimalBitrateKbps > 0 {
		log.Info("  Optimal target: %d kb/s", plan.OptimalBitrateKbps)
	}

	if plan.Estimate.Known {
		log.Info("  Estimate: %d-%d kb/s (%d-%d%% of input)",
			plan.Estimate.LowKbps, plan.Estimate.HighKbps,
			plan.Estimate.LowPct, plan.Estimate.HighPct)
	}
}

// Bitrate outlier thresholds by resolution tier (pixels → low/high kbps).
type bitrateTier struct {
	maxPixels int
	lowKbps   int64
	highKbps  int64
	label     string
}

var bitrateTiers = []bitrateTier{
	{640 * 360, 250, 1800, "<=360p"},
	{854 * 480, 500, 2500, "<=480p"},
	{1280 * 720, 1000, 5000, "<=720p"},
	{1920 * 1080, 2500, 10000, "<=1080p"},
	{2560 * 1440, 5000, 18000, "<=1440p"},
	{3840 * 2160, 10000, 45000, "<=2160p"},
}

func logBitrateOutlier(log *logging.Logger, pr *probe.ProbeResult) {
	v := pr.PrimaryVideo
	if v == nil || v.Width <= 0 || v.Height <= 0 {
		return
	}

	bitrateKbps := pr.VideoBitRate() / 1000
	if bitrateKbps <= 0 {
		return
	}

	pixels := v.Width * v.Height
	var low, high int64
	var label string
	for _, t := range bitrateTiers {
		if pixels <= t.maxPixels {
			low, high, label = t.lowKbps, t.highKbps, t.label
			break
		}
	}
	if label == "" {
		low, high, label = 15000, 65000, ">2160p"
	}

	if bitrateKbps < low {
		log.Outlier("  Bitrate outlier (low): %d kb/s for %s; expected %d-%d kb/s (%s)",
			bitrateKbps, pr.Resolution(), low, high, label)
	} else if bitrateKbps > high {
		log.Outlier("  Bitrate outlier (high): %d kb/s for %s; expected %d-%d kb/s (%s)",
			bitrateKbps, pr.Resolution(), low, high, label)
	}
}

// logAudioBitrates logs per-stream input and planned output bitrates. It is
// always shown (not gated by ShowFileStats) so audio handling is visible
// for every processed file.
func logAudioBitrates(log *logging.Logger, pr *probe.ProbeResult, plan *planner.FilePlan) {
	ap := plan.Audio
	if ap.NoAudio || len(pr.AudioStreams) == 0 {
		return
	}

	for i, a := range pr.AudioStreams {
		inKbps := a.BitRate / 1000
		inStr := "unknown"
		if inKbps > 0 {
			inStr = fmt.Sprintf("%d kbps", inKbps)
		}

		outStr := "n/a"
		switch {
		case ap.CopyAll:
			outStr = "copy"
		case i < len(ap.Streams):
			if ap.Streams[i].Copy {
				outStr = "copy"
			} else {
				outStr = ap.Streams[i].Bitrate
			}
		}

		log.Info("  Audio[%d]: %s | in: %s | out: %s", a.Index, a.Codec, inStr, outStr)
	}
}

func logSummary(cfg *config.Config, log *logging.Logger, stats *RunStats) {
	log.Info("==============================")
	log.Info("Done: %d encoded, %d skipped, %d failed", stats.Encoded, stats.Skipped, stats.Failed)
	log.Info("Summary report:")
	log.Info("  Total files processed: %d", stats.Current)

	if cfg.DryRun {
		log.Info("  Total space saved: n/a (dry run)")
		return
	}

	saved := stats.SpaceSaved()
	if saved >= 0 {
		log.Success("  Total space saved: %s (input %s -> output %s)",
			display.FormatBytes(saved),
			display.FormatBytes(stats.TotalInputBytes),
			display.FormatBytes(stats.TotalOutputBytes))
	} else {
		log.Warn("  Total space saved: -%s (overall output is larger)",
			display.FormatBytes(-saved))
	}
}
