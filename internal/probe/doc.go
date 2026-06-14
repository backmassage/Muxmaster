// Package probe wraps ffprobe to extract structured media metadata from
// a single JSON call per file. It classifies streams, detects HDR transfer
// functions and Dolby Vision, identifies interlaced and 10-bit content, and
// validates HEVC edge-safety.
//
// Files:
//   - types.go:            ProbeResult, VideoStream, AudioStream, SubtitleStream, FormatInfo
//   - prober.go:           Probe — single ffprobe JSON call, stream classification, Dolby Vision detection
//   - hdr.go:              HDR detection, HDR10 static metadata formatting (mastering display, MaxCLL)
//   - interlace.go:        Interlace detection from field_order
//   - pixfmt.go:           PixFmtIs10Bit — 10-bit pixel format detection
package probe
