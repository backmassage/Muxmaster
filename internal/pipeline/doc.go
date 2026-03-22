// Package pipeline orchestrates file discovery, per-file processing, and
// batch summary reporting. It wires together probe, naming, planner, and
// ffmpeg into the sequential processing loop.
//
// All logging goes through the Logger interface (logger.go), and all ffmpeg
// execution goes through an injected ffmpeg.RunFunc, so the orchestration
// logic, retry loops, and quality escalation can be tested in isolation
// without real subprocesses or stdout/stderr.
//
// Files:
//   - logger.go:      Logger — interface for dependency-injected logging
//   - discover.go:    Discover — recursive media file discovery with extras pruning
//   - runner.go:      Run, processFile — per-file orchestration and post-encode quality escalation
//   - report.go:      Batch header, per-file metadata, outlier, and summary logging helpers
//   - analyze.go:     Analyze — probe-only mode with tabular codec/bitrate report
//   - stats.go:       RunStats — aggregate batch statistics
package pipeline
