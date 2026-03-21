// Package ffmpeg builds and executes ffmpeg commands from a planner.FilePlan.
// It handles command construction, stderr capture, regex-based error
// classification, and a retry state machine for recoverable failures.
//
// Files:
//   - builder.go:     BuildArgs — constructs the full ffmpeg argument list from plan + retry state
//   - executor.go:    Execute — runs ffmpeg, captures stderr, optionally shows FPS progress
//   - errors.go:      Error pattern regexes and ClassifyError — maps stderr to RetryAction
//   - retry.go:       RetryState, NewRetryState, Advance — state machine for error recovery
package ffmpeg
