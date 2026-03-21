// Package check provides system diagnostics (--check mode) and pre-pipeline
// dependency validation. It verifies ffmpeg, ffprobe, VAAPI device access,
// x265, and libfdk_aac availability.
//
// Files:
//   - check.go:       RunCheck (--check diagnostics), CheckDeps (pre-pipeline validation)
package check
