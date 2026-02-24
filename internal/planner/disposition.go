package planner

import (
	"fmt"

	"github.com/backmassage/muxmaster/internal/probe"
)

// BuildDispositions produces the ffmpeg -disposition flags that set the
// primary video stream and first audio stream as default, clearing default
// on all subsequent audio streams. This matches the legacy behavior where
// stream 0 of each type is marked default.
func BuildDispositions(pr *probe.ProbeResult) []string {
	opts := []string{"-disposition:v:0", "default"}

	if len(pr.AudioStreams) > 0 {
		opts = append(opts, "-disposition:a:0", "default")
		for i := 1; i < len(pr.AudioStreams); i++ {
			opts = append(opts, fmt.Sprintf("-disposition:a:%d", i), "0")
		}
	}

	return opts
}
