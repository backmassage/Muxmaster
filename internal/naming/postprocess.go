package naming

import (
	"regexp"
	"strings"
	"unicode"
)

var sepReplacer = strings.NewReplacer(".", " ", "_", " ")

// sepsToSpaces replaces dots and underscores with spaces (equivalent to
// bash: tr '._' ' ').
func sepsToSpaces(s string) string { return sepReplacer.Replace(s) }

// cleanName applies separator-to-space conversion, trims trailing separators
// and whitespace. Used during per-rule extraction.
func cleanName(s string) string {
	s = sepsToSpaces(s)
	s = strings.TrimRight(s, " -")
	return strings.TrimSpace(s)
}

// Release tag pattern — case-insensitive, matches from the first known tag
// through end-of-string. The leading boundary ensures we don't match inside
// a word (e.g. "x264" inside "box264" won't match because box264 wouldn't
// have a boundary before x264). Mirrors the legacy 40+ tag list.
var reReleaseTags = regexp.MustCompile(
	`(?i)(^|[\s._\-])(` +
		`720p|1080p|2160p|4K|UHD|` +
		`WEB-DL|WEBRip|BluRay|BDRip|BD|DVDRip|HDTV|` +
		`x264|x265|HEVC|H\.?264|H\.?265|` +
		`AAC|AC3|DTS|DTS-HD|TrueHD|FLAC|EAC3|DD\+?|Atmos|` +
		`10bit|HDR|HDR10|HDR10\+|DV|DoVi|` +
		`Dual\.?Audio|MULTI|REMUX|PROPER|REPACK|` +
		`EMBER|NF|AMZN|DSNP|HMAX|ATVP` +
		`)([\s._\-]|$)`)

// stripReleaseTags removes the first matching release tag and everything
// after it. Returns the prefix before the tag.
func stripReleaseTags(s string) string {
	loc := reReleaseTags.FindStringIndex(s)
	if loc == nil {
		return s
	}
	return strings.TrimSpace(s[:loc[0]])
}

// reBrackets matches square-bracket groups like [SubGroup] or [1080p].
var reBrackets = regexp.MustCompile(`\[[^\]]*\]`)

// stripBrackets removes all [bracketed] content.
func stripBrackets(s string) string {
	return strings.TrimSpace(reBrackets.ReplaceAllString(s, ""))
}

// titleCase capitalizes the first letter of each word. Word boundaries are
// spaces, hyphens, and underscores. Matches legacy bash: sed 's/\b\(.\)/\u\1/g'.
func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) && (prev == ' ' || prev == '-' || prev == '_') {
			prev = r
			return unicode.ToUpper(r)
		}
		prev = r
		return r
	}, s)
}

// reSeasonHintFull matches "Season 02" or "Season_03" in a directory name.
var reSeasonHintFull = regexp.MustCompile(
	`(?i)(^|[^[:alnum:]])[Ss]eason[\s_.\-]*([0-9]{1,2})([^[:alnum:]]|$)`)

// reSeasonHintShort matches "S02" in a directory name.
var reSeasonHintShort = regexp.MustCompile(
	`(?i)(^|[^[:alnum:]])[Ss]([0-9]{1,2})([^[:alnum:]]|$)`)

// extractParentSeasonHint extracts a season number from a parent directory
// name like "Season 02" or "S2". Returns 0 if no season hint is found.
func extractParentSeasonHint(parent string) int {
	if m := reSeasonHintFull.FindStringSubmatch(parent); m != nil {
		return atoi(m[2])
	}
	if m := reSeasonHintShort.FindStringSubmatch(parent); m != nil {
		return atoi(m[2])
	}
	return 0
}

// postProcess applies universal cleaning to a parsed name: strip release
// tags, remove brackets, title-case, apply season hints, and set fallback
// names. Called after the rule-specific extraction.
func postProcess(p ParsedName, parent string) ParsedName {
	p.ShowName = stripReleaseTags(p.ShowName)
	p.ShowName = stripBrackets(p.ShowName)
	p.ShowName = strings.TrimSpace(p.ShowName)
	p.ShowName = titleCase(p.ShowName)

	p.MovieName = stripReleaseTags(p.MovieName)
	p.MovieName = stripBrackets(p.MovieName)
	p.MovieName = strings.TrimSpace(p.MovieName)
	p.MovieName = titleCase(p.MovieName)

	if p.MediaType == MediaTV && p.Season >= 1 {
		hint := extractParentSeasonHint(parent)
		if hint > 1 && p.Season == 1 {
			p.Season = hint
		}
	}

	if p.MediaType == MediaTV && p.ShowName == "" {
		p.ShowName = "Unknown"
	}
	if p.MediaType == MediaMovie && p.MovieName == "" {
		p.MovieName = "Unknown"
	}
	return p
}
