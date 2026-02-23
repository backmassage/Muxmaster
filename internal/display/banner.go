package display

import (
	"fmt"
	"os"

	"github.com/backmassage/muxmaster/internal/logging"
)

// PrintBanner prints the ASCII art banner; uses Magenta if colors are enabled.
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
