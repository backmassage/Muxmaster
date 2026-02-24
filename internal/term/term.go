// Package term provides ANSI color state and terminal detection.
//
// Colors are package-level variables because multiple packages (logging,
// display) need them for output formatting. [Configure] sets them once
// during startup; when colors are disabled the variables are empty strings,
// making string concatenation a no-op.
package term

import (
	"os"
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
)

// ANSI color codes. Empty when colors are disabled.
var (
	Red     = ""
	Green   = ""
	Yellow  = ""
	Orange  = ""
	Blue    = ""
	Cyan    = ""
	Magenta = ""
	NC      = "" // Reset sequence.
)

// Configure resolves the color mode and sets the package-level ANSI
// variables. Call once during startup (from [logging.NewLogger]).
func Configure(mode config.ColorMode) {
	if resolve(mode) {
		Red = "\033[1;91m"
		Green = "\033[1;92m"
		Yellow = "\033[1;93m"
		Orange = "\033[1;38;5;208m"
		Blue = "\033[1;94m"
		Cyan = "\033[1;96m"
		Magenta = "\033[1;95m"
		NC = "\033[0m"
	} else {
		Red, Green, Yellow, Orange, Blue, Cyan, Magenta, NC = "", "", "", "", "", "", "", ""
	}
}

// Enabled reports whether ANSI colors are currently active.
func Enabled() bool { return NC != "" }

// resolve determines whether colors should be enabled based on the configured
// mode, TTY detection, and the NO_COLOR env var (https://no-color.org).
func resolve(mode config.ColorMode) bool {
	switch mode {
	case config.ColorAlways:
		return true
	case config.ColorNever:
		return false
	default: // ColorAuto
		return IsTerminal(os.Stdout) &&
			os.Getenv("NO_COLOR") == "" &&
			strings.ToLower(os.Getenv("TERM")) != "dumb"
	}
}

// IsTerminal reports whether f is attached to a TTY (character device).
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
