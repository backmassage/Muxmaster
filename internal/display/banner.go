// Package display provides user-facing output: the startup banner,
// human-readable byte/bitrate formatting, and (when implemented)
// render-plan logging and bitrate-outlier detection.
//
// Planned additions:
//   - LogRenderPlan(FilePlan): human-readable "what we will do" output.
//   - AssessBitrateOutlier(ProbeResult): log OUTLIER when source bitrate
//     is unusual for the resolution tier.
package display

import (
	"fmt"
	"os"

	"github.com/backmassage/muxmaster/internal/term"
)

// asciiLogo is the Muxmaster banner art.
const asciiLogo = ` __  __            __  __           _
|  \/  |_   ___  _|  \/  | __ _ ___| |_ ___ _ __
| |\/| | | | \ \/ / |\/| |/ _` + "`" + ` / __| __/ _ \ '__|
| |  | | |_| |>  <| |  | | (_| \__ \ ||  __/ |
|_|  |_|\__,_/_/\_\_|  |_|\__,_|___/\__\___|_|
`

// PrintBanner writes the Muxmaster ASCII art logo to stdout.
// When ANSI colors are enabled, the banner is wrapped in magenta.
func PrintBanner() {
	fmt.Fprint(os.Stdout, term.Magenta+asciiLogo+term.NC)
	if term.Enabled() {
		fmt.Fprintln(os.Stdout)
	}
}
