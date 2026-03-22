// Package probe wraps ffprobe to extract structured media metadata from
// a single JSON call per file. It classifies streams, detects HDR transfer
// functions, identifies interlaced content, and validates HEVC edge-safety.
//
// Files:
//   - types.go:            ProbeResult, VideoStream, AudioStream, SubtitleStream, FormatInfo
//   - prober.go:           Probe — single ffprobe JSON call, stream classification
//   - hdr.go:              HDR detection, HDR10 static metadata formatting (mastering display, MaxCLL)
//   - interlace.go:        Interlace detection from field_order
package probe
