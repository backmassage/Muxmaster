// absolute.go maps absolute-numbered anime releases onto Jellyfin seasons.
//
// Some series are distributed with a single running episode number and no
// season marker in the filename (e.g. "<Show>.253.480p...x264-GRP.mkv"). The
// generic rules in rules.go cannot turn such a name into a Season/Episode pair,
// and a bare "<Title>.<NN>.<quality>" form is ambiguous with movies that happen
// to end in a number ("Apollo.13.1080p", "District.9.1080p"). To stay safe we
// only treat a dotted/spaced bare number as an episode when the title matches a
// known absolute-numbered show, then use that show's season-set boundaries to
// split the running number back into (season, episode).
package naming

import (
	"regexp"
	"strings"
)

// absoluteShow describes a series released with absolute episode numbering and
// the absolute episode at which each Jellyfin season begins. starts[i] is the
// first absolute episode of season i+1; it must be sorted ascending and begin
// at 1.
type absoluteShow struct {
	canonical string
	starts    []int
}

// absoluteShows is the allowlist of absolute-numbered series. Add an entry to
// teach the parser a new show; the matching rule and the season split are both
// derived from it. Boundaries follow TheTVDB / Funimation remastered season
// sets (what Jellyfin matches against).
var absoluteShows = []absoluteShow{
	{
		canonical: "Dragon Ball Z",
		// 9 seasons, 291 episodes. S1 1-39, S2 40-74, S3 75-107, S4 108-139,
		// S5 140-165, S6 166-194, S7 195-219, S8 220-253, S9 254-291.
		starts: []int{1, 40, 75, 108, 140, 166, 195, 220, 254},
	},
}

// absoluteRules builds one ParseRule per allowlisted absolute-numbered show.
func absoluteRules() []ParseRule {
	rules := make([]ParseRule, 0, len(absoluteShows))
	for _, s := range absoluteShows {
		rules = append(rules, newAbsoluteRule(s))
	}
	return rules
}

// newAbsoluteRule returns a ParseRule that matches a dotted/spaced scene name
// for one absolute-numbered show ("<Show><sep><1-3 digit ep>") and splits the
// running episode number into the show's season set. The number is capped at 3
// digits so 4-digit years in movie titles are never captured, and the title is
// matched literally so unrelated movies ending in a number are left alone.
func newAbsoluteRule(s absoluteShow) ParseRule {
	fields := strings.Fields(s.canonical)
	for i, f := range fields {
		fields[i] = regexp.QuoteMeta(f)
	}
	pattern := regexp.MustCompile(
		`(?i)^` + strings.Join(fields, `[._ ]`) + `[._ ]([0-9]{1,3})(?:[._ ]|$)`)
	return ParseRule{
		Name:    "Absolute-scene:" + s.canonical,
		Pattern: pattern,
		Extract: func(_ string, m []string, _, _ string) ParsedName {
			season, episode := splitAbsolute(s.starts, parseIntOr0(m[1]))
			return ParsedName{
				MediaType: MediaTV,
				ShowName:  s.canonical,
				Season:    season,
				Episode:   episode,
			}
		},
	}
}

// splitAbsolute converts an absolute episode number into (season, episode)
// using ascending season-start boundaries. Episodes number from 1 within each
// season. Numbers below the first boundary clamp to season 1; numbers past the
// last boundary fall in the final season.
func splitAbsolute(starts []int, abs int) (season, episode int) {
	season, start := 1, starts[0]
	for i, s := range starts {
		if abs >= s {
			season, start = i+1, s
		} else {
			break
		}
	}
	return season, abs - start + 1
}
