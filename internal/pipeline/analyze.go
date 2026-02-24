package pipeline

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/display"
	"github.com/backmassage/muxmaster/internal/logging"
	"github.com/backmassage/muxmaster/internal/probe"
	"github.com/backmassage/muxmaster/internal/term"
)

// fileRow holds the probed per-file data for the analysis table.
type fileRow struct {
	Name       string
	Resolution string
	VideoCodec string
	VideoKbps  int64
	AudioDesc  string // e.g. "aac 2ch" or "ac3 6ch"
}

// Analyze discovers media files, probes each one, and prints a tabular
// codec/bitrate report with statistical outlier highlighting.
func Analyze(ctx context.Context, cfg *config.Config, log *logging.Logger) {
	files, err := Discover(cfg.InputDir)
	if err != nil {
		log.Error("File discovery failed: %v", err)
		return
	}
	if len(files) == 0 {
		log.Warn("No media files found in %s", cfg.InputDir)
		return
	}

	total := len(files)
	log.Info("Analyzing %d files in %s …", total, cfg.InputDir)
	fmt.Println()

	isTTY := term.IsTerminal(os.Stdout)
	var rows []fileRow
	var skipped int
	var videoKbpsVals []float64

	for i, path := range files {
		if ctx.Err() != nil {
			if isTTY {
				clearProgress()
			}
			log.Warn("Interrupted")
			return
		}

		printProgress(isTTY, i+1, total, skipped, filepath.Base(path))

		pr, err := probe.Probe(ctx, path)
		if err != nil {
			skipped++
			if isTTY {
				clearProgress()
			}
			log.Warn("Skip (probe failed): %s", filepath.Base(path))
			continue
		}

		row := fileRow{Name: filepath.Base(path)}

		if pr.PrimaryVideo != nil {
			row.VideoCodec = pr.PrimaryVideo.Codec
			row.VideoKbps = pr.VideoBitRate() / 1000
			row.Resolution = pr.Resolution()
		}
		if len(pr.AudioStreams) > 0 {
			a := pr.AudioStreams[0]
			row.AudioDesc = fmtAudioDesc(a.Codec, a.Channels)
		}

		rows = append(rows, row)
		if row.VideoKbps > 0 {
			videoKbpsVals = append(videoKbpsVals, float64(row.VideoKbps))
		}
	}

	if isTTY {
		clearProgress()
	}

	if len(rows) == 0 {
		log.Warn("No files could be probed")
		return
	}

	vStats := computeStats(videoKbpsVals)

	outliers, extremes := printAnalysisTable(rows, vStats)
	printAnalysisSummary(log, len(rows), skipped, outliers, extremes, vStats)
}

// iqrBounds holds the IQR-based thresholds for outlier classification.
type iqrBounds struct {
	q1, q3    float64
	outlierLo float64 // Q1 - 1.5*IQR.
	outlierHi float64 // Q3 + 1.5*IQR.
	extremeLo float64 // Q1 - 3.0*IQR.
	extremeHi float64 // Q3 + 3.0*IQR.
	valid     bool
}

func computeStats(vals []float64) iqrBounds {
	if len(vals) < 4 {
		return iqrBounds{}
	}

	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)

	q1 := percentile(sorted, 25)
	q3 := percentile(sorted, 75)
	iqr := q3 - q1

	return iqrBounds{
		q1:        q1,
		q3:        q3,
		outlierLo: q1 - 1.5*iqr,
		outlierHi: q3 + 1.5*iqr,
		extremeLo: q1 - 3.0*iqr,
		extremeHi: q3 + 3.0*iqr,
		valid:     iqr > 0,
	}
}

// classify returns "" (normal), "outlier", or "extreme" for a value.
func (b *iqrBounds) classify(v float64) string {
	if !b.valid || v <= 0 {
		return ""
	}
	if v < b.extremeLo || v > b.extremeHi {
		return "extreme"
	}
	if v < b.outlierLo || v > b.outlierHi {
		return "outlier"
	}
	return ""
}

func printAnalysisTable(rows []fileRow, vStats iqrBounds) (outliers, extremes int) {
	// Column headers.
	const (
		hFile   = "File"
		hRes    = "Resolution"
		hVCodec = "Video"
		hVBR    = "Video Kbps"
		hADesc  = "Audio"
	)

	nameW := len(hFile)
	resW := len(hRes)
	vcW := len(hVCodec)
	vbW := len(hVBR)
	adW := len(hADesc)

	for _, r := range rows {
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
		if len(r.Resolution) > resW {
			resW = len(r.Resolution)
		}
		if len(r.VideoCodec) > vcW {
			vcW = len(r.VideoCodec)
		}
		vbStr := display.FormatBitrateLabel(r.VideoKbps)
		if len(vbStr) > vbW {
			vbW = len(vbStr)
		}
		if len(r.AudioDesc) > adW {
			adW = len(r.AudioDesc)
		}
	}

	if nameW > 45 {
		nameW = 45
	}

	// Measure the plain header width for the separator.
	plainHeader := fmt.Sprintf("  %-*s  %-*s  %-*s  %*s  %-*s",
		nameW, hFile,
		resW, hRes,
		vcW, hVCodec,
		vbW, hVBR,
		adW, hADesc,
	)
	separator := "  " + strings.Repeat("─", len(plainHeader)-2)

	// Print colored header and dim separator.
	fmt.Printf("  %s%-*s%s  %s%-*s%s  %s%-*s%s  %s%*s%s  %s%-*s%s\n",
		term.Bold, nameW, hFile, term.NC,
		term.Bold, resW, hRes, term.NC,
		term.Bold, vcW, hVCodec, term.NC,
		term.Bold, vbW, hVBR, term.NC,
		term.Bold, adW, hADesc, term.NC,
	)
	fmt.Printf("%s%s%s\n", term.Dim, separator, term.NC)

	for _, r := range rows {
		name := r.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "…"
		}

		vbPlain := display.FormatBitrateLabel(r.VideoKbps)
		vClass := vStats.classify(float64(r.VideoKbps))

		flag := vClass
		flagStr := formatFlag(flag)

		// Per-column coloring.
		nameCell := fmt.Sprintf("%-*s", nameW, name)
		resCell := colorResolution(fmt.Sprintf("%-*s", resW, r.Resolution), r.Resolution)
		vcCell := colorCodec(fmt.Sprintf("%-*s", vcW, r.VideoCodec), r.VideoCodec)
		vbCell := colorRightAlign(vbPlain, vbW, vClass)
		adCell := colorAudioDesc(fmt.Sprintf("%-*s", adW, r.AudioDesc), r.AudioDesc)

		switch flag {
		case "extreme":
			extremes++
		case "outlier":
			outliers++
		}

		fmt.Printf("  %s  %s  %s  %s  %s  %s\n",
			nameCell,
			resCell,
			vcCell,
			vbCell,
			adCell,
			flagStr,
		)
	}

	fmt.Printf("%s%s%s\n", term.Dim, separator, term.NC)
	fmt.Printf("  %s%d file(s)%s\n", term.Dim, len(rows), term.NC)
	fmt.Println()
	return outliers, extremes
}

func printAnalysisSummary(log *logging.Logger, probed, skipped, outliers, extremes int, vStats iqrBounds) {
	log.Info("Results: %d probed, %d skipped", probed, skipped)

	if vStats.valid {
		log.Info("  Video kbps — Q1: %.0f  Q3: %.0f  (outlier < %.0f or > %.0f)",
			vStats.q1, vStats.q3, vStats.outlierLo, vStats.outlierHi)
	} else {
		log.Info("  Not enough data for outlier detection (need >= 4 files)")
	}

	if outliers > 0 {
		log.Outlier("  %d outlier(s) flagged [*]", outliers)
	}
	if extremes > 0 {
		log.Error("  %d extreme outlier(s) flagged [!]", extremes)
	}
	if outliers == 0 && extremes == 0 && vStats.valid {
		log.Success("  No outliers detected")
	}

	fmt.Println()
	log.Info("  Legend: [*] outlier (1.5× IQR)  [!] extreme (3× IQR)")
}

func fmtAudioDesc(codec string, channels int) string {
	if codec == "" {
		return ""
	}
	return fmt.Sprintf("%s %dch", codec, channels)
}

func formatFlag(flag string) string {
	switch flag {
	case "extreme":
		return term.Red + "[!]" + term.NC
	case "outlier":
		return term.Orange + "[*]" + term.NC
	default:
		return ""
	}
}

// --- Per-column color helpers ---

// colorResolution colors a resolution string by pixel tier.
//
//	4K/2160p  → Cyan
//	1440p     → Blue
//	1080p     → Green
//	720p      → Yellow
//	SD/other  → Orange
func colorResolution(padded, raw string) string {
	switch {
	case strings.HasPrefix(raw, "3840") || strings.HasPrefix(raw, "4096"):
		return term.Cyan + padded + term.NC
	case strings.HasPrefix(raw, "2560"):
		return term.Blue + padded + term.NC
	case strings.HasPrefix(raw, "1920"):
		return term.Green + padded + term.NC
	case strings.HasPrefix(raw, "1280"):
		return term.Yellow + padded + term.NC
	case raw == "" || raw == "unknown":
		return term.Dim + padded + term.NC
	default:
		return term.Orange + padded + term.NC
	}
}

// colorCodec colors a video codec name by generation. Modern codecs that
// need no re-encode get cooler tones; legacy codecs that will be transcoded
// get warmer tones to draw attention.
//
//	HEVC/AV1  → Green  (modern, efficient)
//	VP9       → Cyan   (modern, web-native)
//	H.264     → Blue   (common, decent)
//	Legacy    → Orange (will be re-encoded)
func colorCodec(padded, codec string) string {
	switch strings.ToLower(codec) {
	case "hevc", "h265", "av1":
		return term.Green + padded + term.NC
	case "vp9":
		return term.Cyan + padded + term.NC
	case "h264", "avc", "avc1":
		return term.Blue + padded + term.NC
	case "mpeg2video", "mpeg4", "wmv3", "vc1", "msmpeg4v3":
		return term.Orange + padded + term.NC
	case "":
		return term.Dim + padded + term.NC
	default:
		return term.Yellow + padded + term.NC
	}
}

// colorAudioDesc colors the audio description by codec family.
//
//	AAC          → Green   (passthrough, no re-encode needed)
//	FLAC/PCM     → Cyan    (lossless source)
//	AC3/DTS/EAC3 → Yellow  (common surround, will be transcoded)
//	Other        → Magenta
func colorAudioDesc(padded, desc string) string {
	lower := strings.ToLower(desc)
	switch {
	case strings.HasPrefix(lower, "aac"):
		return term.Green + padded + term.NC
	case strings.HasPrefix(lower, "flac") || strings.HasPrefix(lower, "pcm"):
		return term.Cyan + padded + term.NC
	case strings.HasPrefix(lower, "ac3") || strings.HasPrefix(lower, "eac3") ||
		strings.HasPrefix(lower, "dts") || strings.HasPrefix(lower, "truehd"):
		return term.Yellow + padded + term.NC
	case desc == "":
		return term.Dim + padded + term.NC
	default:
		return term.Magenta + padded + term.NC
	}
}

// colorRightAlign right-aligns a plain string to width (for numeric columns),
// then wraps in ANSI color for outlier classification.
func colorRightAlign(s string, width int, class string) string {
	padded := fmt.Sprintf("%*s", width, s)
	switch class {
	case "extreme":
		return term.Red + padded + term.NC
	case "outlier":
		return term.Orange + padded + term.NC
	default:
		return padded
	}
}

// printProgress shows a live probe counter. On a TTY it writes an
// inline \r-overwritten line; otherwise it is a no-op (the skip warnings
// already provide enough breadcrumbs in piped/logged output).
func printProgress(isTTY bool, current, total, skipped int, name string) {
	if !isTTY {
		return
	}
	pct := current * 100 / total
	status := fmt.Sprintf("  Probing [%d/%d] %d%% ", current, total, pct)
	if skipped > 0 {
		status += fmt.Sprintf("(%d skipped) ", skipped)
	}

	maxName := 40
	if len(name) > maxName {
		name = name[:maxName-1] + "…"
	}
	status += name

	if len(status) < 80 {
		status += strings.Repeat(" ", 80-len(status))
	}
	fmt.Fprintf(os.Stdout, "\r%s", status)
}

// clearProgress erases the inline progress line on a TTY.
func clearProgress() {
	fmt.Fprintf(os.Stdout, "\r%s\r", strings.Repeat(" ", 80))
}

// percentile computes the p-th percentile using linear interpolation.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p / 100) * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi || hi >= len(sorted) {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}
