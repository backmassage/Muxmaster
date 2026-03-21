// Package config holds runtime configuration: defaults, CLI flag parsing,
// and validation. All defaults match the legacy Muxmaster.sh v1.7.0.
//
// Files:
//   - config.go:      Config struct, DefaultConfig, Validate, ValidatePaths
//   - flags.go:       ParseFlags — CLI flag definitions and quality precedence logic
package config
