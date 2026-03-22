// Package config holds runtime configuration: defaults, CLI flag parsing,
// and validation. All defaults match the legacy Muxmaster.sh v1.7.0.
//
// The Config struct is organized into sub-structs by concern:
//   - EncoderConfig: Video encoder mode, VAAPI/CPU params, HDR, quality curves
//   - AudioConfig:   Audio codec, bitrate, channels, layout normalization
//   - DisplayConfig: Verbosity, FPS display, color mode, log file
//
// Files:
//   - config.go:      Config + sub-structs, DefaultConfig, Validate, ValidatePaths
//   - flags.go:       ParseFlags — CLI flag definitions and quality precedence logic
package config
