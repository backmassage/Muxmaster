// Package pipeline orchestrates file discovery, per-file processing, and
// batch summary reporting. It wires together probe, naming, planner, and
// ffmpeg into the sequential processing loop.
//
// Files:
//   - discover.go:    Discover — recursive media file discovery with extras pruning
//   - runner.go:      Run, processFile — per-file orchestration and post-encode quality escalation
//   - report.go:      Batch header, per-file metadata, outlier, and summary logging helpers
//   - analyze.go:     Analyze — probe-only mode with tabular codec/bitrate report
//   - stats.go:       RunStats — aggregate batch statistics
package pipeline
