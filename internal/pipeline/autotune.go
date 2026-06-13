// autotune.go resolves the per-series content prefilter for `--tune auto`:
// group files by series, run one grain pre-pass per series, and prompt the user
// once to confirm/override the suggested profile for all its episodes.
package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/naming"
	"github.com/backmassage/muxmaster/internal/probe"
	"github.com/backmassage/muxmaster/internal/term"
	"github.com/backmassage/muxmaster/internal/tune"
)

// seriesGroup is one prompt unit: all files sharing a series key, with a
// representative used for the grain pre-pass.
type seriesGroup struct {
	key   string
	label string
	rep   string // representative file path (first seen)
	count int
}

// seriesKey derives a stable grouping key and display label for a parsed file.
// TV episodes group by (harmonized) show name; movies group individually by
// name+year; anything unparsed is its own group keyed on the filename.
func seriesKey(p naming.ParsedName, path string) (key, label string) {
	switch p.MediaType {
	case naming.MediaTV:
		if p.ShowName != "" {
			return "tv:" + strings.ToLower(p.ShowName), p.ShowName
		}
	case naming.MediaMovie:
		if p.MovieName != "" {
			label := p.MovieName
			if p.Year != "" {
				label += " (" + p.Year + ")"
			}
			return "movie:" + strings.ToLower(p.MovieName) + ":" + p.Year, label
		}
	}
	base := filepath.Base(path)
	return "file:" + strings.ToLower(base), base
}

// seriesKeyForPath parses and harmonizes a path the same way processFile does,
// so the key computed here matches the one looked up during processing.
func seriesKeyForPath(path string, yearIndex naming.YearVariantIndex) (key, label string) {
	parsed := naming.ParseFilename(filepath.Base(path), filepath.Dir(path))
	if parsed.MediaType == naming.MediaTV {
		parsed.ShowName = naming.HarmonizeShowName(parsed.ShowName, yearIndex)
	}
	return seriesKey(parsed, path)
}

// groupBySeries collects files into series groups, preserving first-seen order
// so prompts appear in a stable sequence.
func groupBySeries(files []string, yearIndex naming.YearVariantIndex) []seriesGroup {
	var order []string
	groups := make(map[string]*seriesGroup)
	for _, path := range files {
		key, label := seriesKeyForPath(path, yearIndex)
		g, ok := groups[key]
		if !ok {
			g = &seriesGroup{key: key, label: label, rep: path}
			groups[key] = g
			order = append(order, key)
		}
		g.count++
	}
	out := make([]seriesGroup, 0, len(order))
	for _, key := range order {
		out = append(out, *groups[key])
	}
	return out
}

// resolveSeriesTunes runs the grain pre-pass per series and prompts the user to
// pick a prefilter for each, returning a key→tune map consumed during
// processing. Caller gates this on `--tune auto` and an interactive stdin.
func resolveSeriesTunes(ctx context.Context, log Logger, files []string, yearIndex naming.YearVariantIndex) map[string]config.TuneMode {
	groups := groupBySeries(files, yearIndex)
	result := make(map[string]config.TuneMode, len(groups))
	if len(groups) == 0 {
		return result
	}

	log.Info("Auto-tune: analyzing %d series for grain (pick a prefilter per series)…", len(groups))
	in := bufio.NewReader(os.Stdin)
	for i := range groups {
		g := groups[i]
		if ctx.Err() != nil {
			break
		}
		sug := suggestForRep(ctx, log, g)
		choice := promptSeriesTune(os.Stdout, in, g, sug)
		result[g.key] = choice
		log.Success("  %s → --tune %s", g.label, choice)
	}
	log.Blank()
	return result
}

// suggestForRep probes the representative for duration, runs the grain pre-pass,
// and maps it to a suggestion; failures degrade to a low-confidence none.
func suggestForRep(ctx context.Context, log Logger, g seriesGroup) tune.Suggestion {
	dur := 0.0
	if pr, err := probe.Probe(ctx, g.rep); err == nil && pr.Format.Duration > 0 {
		dur = pr.Format.Duration
	}
	sig, err := tune.DetectContent(ctx, g.rep, dur)
	if err != nil {
		log.Warn("  %s: grain pre-pass failed (%v) — suggesting none", g.label, err)
		return tune.Suggestion{Tune: config.TuneNone, Confidence: tune.ConfidenceLow, Reason: "pre-pass failed"}
	}
	return tune.Suggest(sig)
}

// sampleNameWidth caps the sampled filename shown in the prompt so a long
// release name doesn't bury the grain reading.
const sampleNameWidth = 52

// promptSeriesTune renders the per-series suggestion and reads the user's
// choice. The grain reading and the four options are color-coded by severity,
// and the suggested option is bracketed. An empty line accepts the suggestion;
// an unrecognized entry re-prompts (rather than silently falling back) until a
// valid choice, a blank line, or end-of-input.
func promptSeriesTune(out io.Writer, in *bufio.Reader, g seriesGroup, sug tune.Suggestion) config.TuneMode {
	reasonColor := tuneColor(sug.Tune)
	if sug.Confidence == tune.ConfidenceLow {
		reasonColor = term.Yellow // detection failed → caution, user should decide
	}

	fmt.Fprintf(out, "\n%s%s%s · %d file%s\n",
		term.Cyan, g.label, term.NC, g.count, plural(g.count))
	fmt.Fprintf(out, "  sampled %s  %s%s%s\n",
		truncateName(filepath.Base(g.rep), sampleNameWidth), reasonColor, sug.Reason, term.NC)
	fmt.Fprintf(out, "  prefilter  %s  · Enter = %s\n",
		renderTuneOptions(sug.Tune), sug.Tune)

	for {
		fmt.Fprintf(out, "  %s>%s ", term.Bold, term.NC)
		line, err := in.ReadString('\n')
		if choice, ok := parseTuneChoice(line, sug.Tune); ok {
			return choice
		}
		// Unrecognized, non-empty entry. If stdin is also closed we can't
		// re-prompt, so accept the suggestion; otherwise ask again.
		if err != nil {
			fmt.Fprintln(out)
			return sug.Tune
		}
		fmt.Fprintf(out, "  %s↳ type none / film / grain / anime, or Enter for %s%s\n",
			term.Yellow, sug.Tune, term.NC)
	}
}

// renderTuneOptions lays out the four profiles in a fixed order, coloring each
// by severity and bracketing the suggested one so the default is unmistakable.
func renderTuneOptions(suggested config.TuneMode) string {
	order := []config.TuneMode{config.TuneNone, config.TuneFilm, config.TuneGrain, config.TuneAnime}
	parts := make([]string, len(order))
	for i, o := range order {
		if o == suggested {
			parts[i] = tuneColor(o) + "[" + string(o) + "]" + term.NC
		} else {
			parts[i] = term.Dim + string(o) + term.NC
		}
	}
	return strings.Join(parts, "  ")
}

// tuneColor maps a profile to a severity color: clean→green, light→yellow,
// heavy→orange, deband→blue. Empty (no-op) when colors are disabled.
func tuneColor(t config.TuneMode) string {
	switch t {
	case config.TuneNone:
		return term.Green
	case config.TuneFilm:
		return term.Yellow
	case config.TuneGrain:
		return term.Orange
	case config.TuneAnime:
		return term.Blue
	default:
		return term.NC
	}
}

// truncateName shortens s to at most max runes (rune-safe so a multibyte
// character is never split), appending an ellipsis when it trims.
func truncateName(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// parseTuneChoice maps a user entry to a TuneMode. An empty entry returns the
// suggestion. Accepts full words or first-letter shorthand. The bool is false
// for unrecognized input so the caller can fall back explicitly.
func parseTuneChoice(s string, suggestion config.TuneMode) (config.TuneMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return suggestion, true
	case "none", "n":
		return config.TuneNone, true
	case "film", "f":
		return config.TuneFilm, true
	case "grain", "g":
		return config.TuneGrain, true
	case "anime", "a":
		return config.TuneAnime, true
	default:
		return suggestion, false
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
