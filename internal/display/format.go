package display

import (
	"fmt"
)

// FormatBytes returns a human-readable size (B, KiB, MiB, GiB, TiB, PiB).
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

// FormatBytesWithSign prefixes with + or - for delta display (e.g. "- 1.2 GiB").
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

// FormatBitrateLabel returns a short label for bitrate in kbps (e.g. "1200 kbps").
func FormatBitrateLabel(kbps int64) string {
	if kbps < 1000 {
		return fmt.Sprintf("%d kbps", kbps)
	}
	return fmt.Sprintf("%.1f Mbps", float64(kbps)/1000)
}
