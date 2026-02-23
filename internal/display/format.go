package display

import (
	"fmt"
)

// FormatBytes returns a short human-readable size string using 1024-based units (B, KiB, MiB, GiB, TiB, PiB, EiB).
// Used for file sizes and summary stats.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
		div = 1
		for i := 0; i <= exp; i++ {
			div *= unit
		}
	}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), suffixes[exp])
}

// FormatBytesWithSign formats a size delta with a leading "+ " or "- " for saved/used space in summaries.
func FormatBytesWithSign(bytes int64) string {
	sign := ""
	if bytes > 0 {
		sign = "+ "
	} else if bytes < 0 {
		sign = "- "
		bytes = -bytes
	}
	return sign + FormatBytes(bytes)
}

// FormatBitrateLabel returns a short bitrate string (e.g. "800 kbps" or "1.2 Mbps") for display in file stats.
func FormatBitrateLabel(kbps int64) string {
	if kbps < 1000 {
		return fmt.Sprintf("%d kbps", kbps)
	}
	return fmt.Sprintf("%.1f Mbps", float64(kbps)/1000)
}
