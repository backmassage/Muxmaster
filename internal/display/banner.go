// Package display provides user-facing output: banner, byte/bitrate formatting, and (later) render-plan and outlier logs.
package display

import (
	"fmt"
	"os"

	"github.com/backmassage/muxmaster/internal/logging"
)

// PrintBanner prints the Muxmaster ASCII art logo to stdout.
// If the logging package has enabled colors (Magenta set), the banner is printed in magenta, then reset.
func PrintBanner() {
	if logging.Magenta != "" {
		fmt.Fprint(os.Stdout, "\033[1;95m")
	}
	fmt.Fprint(os.Stdout, ` __  __            __  __           _
|  \/  |_   ___  _|  \/  | __ _ ___| |_ ___ _ __
| |\/| | | | \ \/ / |\/| |/ _` + "`" + ` / __| __/ _ \ '__|
| |  | | |_| |>  <| |  | | (_| \__ \ ||  __/ |
|_|  |_|\__,_/_/\_\_|  |_|\__,_|___/\__\___|_|
`)
	if logging.Magenta != "" {
		fmt.Fprintln(os.Stdout, logging.NC)
	}
}
