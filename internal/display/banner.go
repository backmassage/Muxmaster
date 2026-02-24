// Package display provides user-facing output: the startup banner and
// human-readable byte/bitrate formatting.
package display

import (
	"fmt"
	"os"
	"strings"

	"github.com/backmassage/muxmaster/internal/term"
)

// bannerLines are the rows of the Muxmaster ASCII art logo.
var bannerLines = []string{
	` __  __            __  __           _`,
	`|  \/  |_   ___  _|  \/  | __ _ ___| |_ ___ _ __`,
	"| |\\/| | | | \\ \\/ / |\\/| |/ _` / __| __/ _ \\ '__|",
	`| |  | | |_| |>  <| |  | | (_| \__ \ ||  __/ |`,
	`|_|  |_|\__,_/_/\_\_|  |_|\__,_|___/\__\___|_|`,
}

// rainbow cycles through bold ANSI colors, one per banner line.
var rainbow = []string{
	"\033[1;91m",       // red
	"\033[1;38;5;208m", // orange
	"\033[1;93m",       // yellow
	"\033[1;92m",       // green
	"\033[1;94m",       // blue
}

// PrintBanner writes the Muxmaster ASCII art logo to stdout.
// When ANSI colors are enabled, each line gets a different rainbow color.
func PrintBanner() {
	if !term.Enabled() {
		fmt.Fprintln(os.Stdout, strings.Join(bannerLines, "\n"))
		return
	}
	for i, line := range bannerLines {
		color := rainbow[i%len(rainbow)]
		fmt.Fprintf(os.Stdout, "%s%s%s\n", color, line, term.NC)
	}
	fmt.Fprintln(os.Stdout)
}
