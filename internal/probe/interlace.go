package probe

import "strings"

// IsInterlaced returns true if the primary video stream's field_order
// indicates interlaced content (tt, bb, tb, bt). Mirrors the legacy
// is_interlaced function.
func (p *ProbeResult) IsInterlaced() bool {
	if p.PrimaryVideo == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(p.PrimaryVideo.FieldOrder)) {
	case "tt", "bb", "tb", "bt":
		return true
	}
	return false
}

// IsEdgeSafeHEVC returns true if the primary video stream has an HEVC
// profile and pixel format that are safe for browser/Jellyfin playback.
// Safe profiles: main, main 10, main10. Safe pix_fmts: yuv420p, yuv420p10le.
// Mirrors the legacy is_edge_safe_hevc_stream function.
func (p *ProbeResult) IsEdgeSafeHEVC() bool {
	if p.PrimaryVideo == nil {
		return false
	}

	profile := strings.ToLower(strings.TrimSpace(p.PrimaryVideo.Profile))
	switch profile {
	case "main", "main 10", "main10":
		// ok
	default:
		return false
	}

	pf := strings.ToLower(strings.TrimSpace(p.PrimaryVideo.PixFmt))
	return pf == "yuv420p" || pf == "yuv420p10le"
}
