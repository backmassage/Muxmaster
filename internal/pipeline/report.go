// report.go provides batch and per-file logging helpers used by the runner.
// These format probe results, plan details, bitrate outliers, and summary
// statistics for human-readable terminal and log-file output.
package pipeline

import (
	"fmt"
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/display"
	"github.com/backmassage/muxmaster/internal/planner"
	"github.com/backmassage/muxmaster/internal/probe"
	"github.com/backmassage/muxmaster/internal/term"
)

const maxStderrLines = 20

// logSignal reports when ffmpeg was terminated by a signal rather than exiting
// on its own. A signal kill writes nothing to stderr, so without this the
// failure looks like a contentless "no error message captured". SIGKILL on a
// remux/encode is almost always the OOM killer (e.g. unbounded muxer
// interleave buffering on a sparse stream); calling it out points straight at
// the cause instead of leaving an empty error.
func logSignal(log Logger, signal string) {
	if signal == "" {
		return
	}
	if signal == "killed" {
		log.Error("ffmpeg was killed by the kernel (SIGKILL) — almost certainly out of memory")
	} else {
		log.Error("ffmpeg terminated by signal: %s", signal)
	}
}

func logStderr(log Logger, stderr string) {
	if stderr == "" {
		return
	}
	lines := meaningfulStderrLines(stderr)
	if len(lines) == 0 {
		// Only progress output was captured (the real error, if any, never
		// reached stderr). Show the final progress line so the call isn't silent.
		log.Error("Last ffmpeg output: (progress only — no error message captured)")
		if last := lastNonEmptyLine(stderr); last != "" {
			log.Error("  %s", last)
		}
		return
	}
	log.Error("Last ffmpeg output:")
	if len(lines) > maxStderrLines {
		log.Error("  ... %d lines omitted ...", len(lines)-maxStderrLines)
		lines = lines[len(lines)-maxStderrLines:]
	}
	for _, l := range lines {
		log.Error("  %s", l)
	}
}

// meaningfulStderrLines normalizes ffmpeg stderr and drops progress-meter
// noise. ffmpeg rewrites its progress line in place with carriage returns, so
// the raw buffer is one long `\r`-joined string with the real error message
// buried among (and sometimes visually overwritten by) stats lines. We split
// on both `\r` and `\n` and discard progress lines so the actual diagnostic —
// the line the retry classifier and the user both need — survives.
func meaningfulStderrLines(stderr string) []string {
	raw := strings.FieldsFunc(stderr, func(r rune) bool { return r == '\n' || r == '\r' })
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		t := strings.TrimSpace(l)
		if t == "" || isProgressLine(t) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// isProgressLine reports whether a stderr line is an ffmpeg progress/stats
// update (e.g. "frame=120298 fps=2455 ... time=N/A bitrate=N/A speed=N/A")
// rather than a real log message.
func isProgressLine(line string) bool {
	if strings.HasPrefix(line, "frame=") || strings.HasPrefix(line, "size=") {
		return true
	}
	// Frameless audio/remux progress omits the "frame=" prefix but always
	// carries the time/bitrate/speed triple.
	return strings.Contains(line, "time=") &&
		strings.Contains(line, "bitrate=") &&
		strings.Contains(line, "speed=")
}

func lastNonEmptyLine(stderr string) string {
	raw := strings.FieldsFunc(stderr, func(r rune) bool { return r == '\n' || r == '\r' })
	for i := len(raw) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(raw[i]); t != "" {
			return t
		}
	}
	return ""
}

func logBatchHeader(cfg *config.Config, log Logger, stats *RunStats) {
	log.Info("Found %d files", stats.Total)

	profileLabel := cfg.Encoder.CpuProfile
	qualityValue := cfg.Encoder.CpuCRF
	if cfg.Encoder.Mode == config.EncoderVAAPI {
		profileLabel = cfg.Encoder.VaapiProfile
		if profileLabel == "" {
			profileLabel = "main10"
		}
		qualityValue = cfg.Encoder.VaapiQP
	}
	log.Info("Mode: %s (HEVC %s), QP/CRF: %d", cfg.Encoder.Mode, profileLabel, qualityValue)

	if cfg.Encoder.ActiveQualityOverride != "" {
		if cfg.Encoder.Mode == config.EncoderVAAPI {
			log.Info("Quality mode: manual fixed override (VAAPI_QP=%s)", cfg.Encoder.ActiveQualityOverride)
		} else {
			log.Info("Quality mode: manual fixed override (CPU_CRF=%s)", cfg.Encoder.ActiveQualityOverride)
		}
	} else if cfg.Encoder.SmartQuality {
		log.Info("Quality mode: smart per-file adaptation (mode-specific CPU/VAAPI curves)")
	} else {
		log.Info("Quality mode: fixed defaults")
	}

	log.Info("Container: %s", strings.ToUpper(string(cfg.OutputContainer)))
	log.Info("Audio: AAC passthrough, non-AAC encode to AAC via %s at %s", cfg.Audio.Encoder, cfg.Audio.Bitrate)

	if cfg.OutputContainer == config.ContainerMP4 {
		log.Info("Compatibility: hvc1 tag for Apple/browser support")
	}
	if cfg.Encoder.HandleHDR == config.HDRPreserve {
		log.Info("HDR: Preserve metadata when present")
	} else {
		log.Info("HDR: Tonemap to SDR")
	}
	if cfg.Encoder.DeinterlaceAuto {
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

func logInputMeta(log Logger, pr *probe.ProbeResult) {
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

func logFileStats(log Logger, plan *planner.FilePlan) {
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

// bitrateTier defines the expected bitrate range for a resolution bucket,
// used to flag outlier sources before encoding.
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

func logBitrateOutlier(log Logger, pr *probe.ProbeResult) {
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

func logAudioBitrates(log Logger, pr *probe.ProbeResult, plan *planner.FilePlan) {
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

func logSummary(cfg *config.Config, log Logger, stats *RunStats) {
	log.Info("==============================")
	log.Info("Done: %d encoded, %d skipped, %d failed", stats.Encoded, stats.Skipped, stats.Failed)
	log.Info("Summary report:")
	log.Info("  Total files processed: %d", stats.Encoded+stats.Skipped+stats.Failed)

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
