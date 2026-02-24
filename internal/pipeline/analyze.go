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
	VideoCodec string
	VideoKbps  int64
	AudioCodec string
	AudioKbps  int64
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
	var videoKbpsVals, audioKbpsVals []float64

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
		}
		if len(pr.AudioStreams) > 0 {
			a := pr.AudioStreams[0]
			row.AudioCodec = a.Codec
			row.AudioKbps = a.BitRate / 1000
		}

		rows = append(rows, row)
		if row.VideoKbps > 0 {
			videoKbpsVals = append(videoKbpsVals, float64(row.VideoKbps))
		}
		if row.AudioKbps > 0 {
			audioKbpsVals = append(audioKbpsVals, float64(row.AudioKbps))
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
	aStats := computeStats(audioKbpsVals)

	printAnalysisTable(rows, vStats, aStats)
	printAnalysisSummary(log, rows, vStats, aStats)
}

// iqrBounds holds the IQR-based thresholds for outlier classification.
type iqrBounds struct {
	q1, q3    float64
	outlierLo float64 // Q1 - 1.5*IQR
	outlierHi float64 // Q3 + 1.5*IQR
	extremeLo float64 // Q1 - 3.0*IQR
	extremeHi float64 // Q3 + 3.0*IQR
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

func printAnalysisTable(rows []fileRow, vStats, aStats iqrBounds) {
	nameW := len("File")
	vcW := len("Video Codec")
	vbW := len("Video Kbps")
	acW := len("Audio Codec")
	abW := len("Audio Kbps")

	for _, r := range rows {
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
		if len(r.VideoCodec) > vcW {
			vcW = len(r.VideoCodec)
		}
		vbStr := display.FormatBitrateLabel(r.VideoKbps)
		if len(vbStr) > vbW {
			vbW = len(vbStr)
		}
		if len(r.AudioCodec) > acW {
			acW = len(r.AudioCodec)
		}
		abStr := fmtAudioKbps(r.AudioKbps)
		if len(abStr) > abW {
			abW = len(abStr)
		}
	}

	if nameW > 50 {
		nameW = 50
	}

	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %-*s",
		nameW, "File",
		vcW, "Video Codec",
		vbW, "Video Kbps",
		acW, "Audio Codec",
		abW, "Audio Kbps",
	)
	separator := "  " + strings.Repeat("─", len(header)-2)

	fmt.Println(header)
	fmt.Println(separator)

	for _, r := range rows {
		name := r.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "…"
		}

		vbPlain := display.FormatBitrateLabel(r.VideoKbps)
		abPlain := fmtAudioKbps(r.AudioKbps)

		vClass := vStats.classify(float64(r.VideoKbps))
		aClass := aStats.classify(float64(r.AudioKbps))

		flag := worstFlag(vClass, aClass)
		flagStr := formatFlag(flag)

		// Pad the plain text first, then wrap in ANSI color. This avoids
		// the alignment bug where %-*s counts escape bytes as visible width.
		vbCell := colorPad(vbPlain, vbW, vClass)
		abCell := colorPad(abPlain, abW, aClass)

		fmt.Printf("  %-*s  %-*s  %s  %-*s  %s  %s\n",
			nameW, name,
			vcW, r.VideoCodec,
			vbCell,
			acW, r.AudioCodec,
			abCell,
			flagStr,
		)
	}
	fmt.Println()
}

func printAnalysisSummary(log *logging.Logger, rows []fileRow, vStats, aStats iqrBounds) {
	var outliers, extremes int
	for _, r := range rows {
		vClass := vStats.classify(float64(r.VideoKbps))
		aClass := aStats.classify(float64(r.AudioKbps))
		worst := worstFlag(vClass, aClass)
		switch worst {
		case "extreme":
			extremes++
		case "outlier":
			outliers++
		}
	}

	log.Info("Analyzed %d files", len(rows))
	if vStats.valid {
		log.Info("  Video bitrate IQR: %.0f – %.0f kbps (outlier < %.0f or > %.0f)",
			vStats.q1, vStats.q3, vStats.outlierLo, vStats.outlierHi)
	}
	if aStats.valid {
		log.Info("  Audio bitrate IQR: %.0f – %.0f kbps (outlier < %.0f or > %.0f)",
			aStats.q1, aStats.q3, aStats.outlierLo, aStats.outlierHi)
	}
	if outliers > 0 {
		log.Outlier("  %d outlier(s) flagged [*]", outliers)
	}
	if extremes > 0 {
		log.Error("  %d extreme outlier(s) flagged [!]", extremes)
	}
	if outliers == 0 && extremes == 0 {
		log.Success("  No outliers detected")
	}
}

func fmtAudioKbps(kbps int64) string {
	if kbps <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d kbps", kbps)
}

func worstFlag(classes ...string) string {
	worst := ""
	for _, c := range classes {
		if c == "extreme" {
			return "extreme"
		}
		if c == "outlier" {
			worst = "outlier"
		}
	}
	return worst
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

// colorPad pads a plain string to width, then wraps in ANSI color. This
// ensures %-*s-style alignment works correctly regardless of escape sequences.
func colorPad(s string, width int, class string) string {
	padded := fmt.Sprintf("%-*s", width, s)
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

	// Pad to 80 chars to overwrite previous longer lines, then \r.
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
