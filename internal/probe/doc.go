// Package probe provides ffprobe-based media inspection and typed result
// structures. A single JSON call per file replaces the multiple subprocess
// calls in the legacy shell script.
//
// Planned implementation (see _docs/design/foundation-plan.md §6.3):
//
// Types:
//   - FormatInfo, VideoStream, AudioStream, SubtitleStream, ProbeResult
//
// Functions:
//   - Probe(path) → *ProbeResult
//     Runs ffprobe -print_format json -show_format -show_streams.
//   - (*ProbeResult).HDRType() → "hdr10" | "sdr"
//     Detects HDR from color transfer/primaries/space metadata.
//   - (*ProbeResult).IsInterlaced() → bool
//     Checks field_order for tt, bb, tb, bt.
//
// When implementing, split into probe.go (types + Probe), hdr.go, and
// interlace.go along these boundaries.
package probe
