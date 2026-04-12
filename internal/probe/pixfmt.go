// Pixel format helpers derived from ffprobe pix_fmt strings.
package probe

import "strings"

// PixFmtIs10Bit reports whether pix_fmt uses 10 bits per component (or the
// packed 10-bit layouts p010/p012/p016 and similar). Used to detect Hi10p AVC
// and other 10-bit sources where naming follows ffmpeg conventions.
func PixFmtIs10Bit(pixFmt string) bool {
	p := strings.ToLower(strings.TrimSpace(pixFmt))
	if p == "" {
		return false
	}
	if strings.Contains(p, "10le") || strings.Contains(p, "10be") {
		return true
	}
	switch p {
	case "p010", "p012", "p016", "p210", "p212", "p216", "p410", "p412", "p416":
		return true
	default:
		return false
	}
}
