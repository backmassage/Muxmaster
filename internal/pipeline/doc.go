// Package pipeline orchestrates file discovery, per-file processing, and
// batch summary reporting.
//
// Planned implementation:
//
// Types:
//   - RunStats (Total, Encoded, Skipped, Failed, TotalInputBytes,
//     TotalOutputBytes; SpaceSaved method)
//
// Functions:
//   - Run(ctx, cfg, log) → RunStats
//     Batch runner: discover → build TV year index → for each file:
//     validate → probe → parse name → resolve output path →
//     plan (encode/remux/skip) → execute with retry → update stats.
//   - Discover(inputDir) → []string
//     Walk directory, filter by extension (mkv, mp4, avi, …),
//     exclude extras dirs, sort deterministically.
//
// When implementing, split into runner.go, discover.go, stats.go.
package pipeline
