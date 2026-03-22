// Package ffmpeg builds and executes ffmpeg commands from a planner.FilePlan.
// It handles command construction, stderr capture, regex-based error
// classification, and a retry state machine for recoverable failures.
//
// Execute accepts a RunFunc so tests can inject a mock subprocess runner
// and verify the builder→executor path without spawning real ffmpeg.
//
// Files:
//   - builder.go:     Build — constructs the full ffmpeg argument list from plan + retry state
//   - executor.go:    Execute, RunFunc, NewRunFunc — injectable subprocess execution
//   - errors.go:      Error pattern regexes and ClassifyError — maps stderr to RetryAction
//   - retry.go:       RetryState, NewRetryState, Advance — state machine for error recovery
package ffmpeg
