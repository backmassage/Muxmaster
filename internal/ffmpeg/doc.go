// Package ffmpeg builds and executes ffmpeg commands with a shared argument
// skeleton and unified retry logic.
//
// Planned implementation:
//
// Types:
//   - RetryState (tracks which fixes have been applied)
//   - 4 compiled regexes for stderr classification: attachment tag,
//     subtitle mux, mux queue overflow, timestamp discontinuity.
//
// Functions:
//   - Build(Config, FilePlan) → []string
//     Shared skeleton (-probesize, -analyzeduration, -max_muxing_queue_size,
//     map, -dn) plus encode/remux codec-specific args.
//   - Execute(ctx, args, opts) → error
//     Run ffmpeg, capture stderr, optional tee to os.Stderr.
//     Supports verbose mode and stats_period for ShowFfmpegFPS.
//   - (*RetryState).Advance(stderr) → ([]string, bool)
//     One fix per attempt: attachment → subtitle → mux queue → timestamp.
//     Max 4 attempts per file.
//
// When implementing, split into builder.go, executor.go, errors.go, retry.go.
package ffmpeg
