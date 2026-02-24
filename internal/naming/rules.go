package naming

import (
	"regexp"
	"strconv"
	"strings"
)

// ParseRule pairs a compiled regex with an extraction function. Rules are
// evaluated in order by [ParseFilename]; first match wins.
type ParseRule struct {
	Name    string
	Pattern *regexp.Regexp
	Extract func(base string, matches []string, parent string) ParsedName
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// --- Helper regexes for show-name extraction in rules 1 & 2 ---

var reStripSxxExx = regexp.MustCompile(
	`(?i)[\s._\-]*[Ss][0-9]{1,2}[Ee][0-9]{1,3}([Vv][0-9]+)?[^\s]*.*`)

var reStrip1x01 = regexp.MustCompile(
	`(?i)[\s._\-]*[0-9]{1,2}[xX][0-9]{1,3}([Vv][0-9]+)?[^\s]*.*`)

var reStripParentSeason = regexp.MustCompile(
	`(?i)[\s._\-]*([Ss]eason[\s_.\-]*[0-9]{1,2}|[Ss][0-9]{1,2}[Ee][0-9]{1,3}([Vv][0-9]+)?|[Ss][0-9]{1,2})([\s].*)?$`)

// extractShowFromBase strips the episode token and everything after it,
// then cleans separators. stripRe should match the episode token pattern.
func extractShowFromBase(base string, stripRe *regexp.Regexp) string {
	name := stripRe.ReplaceAllString(base, "")
	return cleanName(name)
}

// extractShowFromParent derives the show name from the parent directory,
// stripping any season/episode suffix.
func extractShowFromParent(parent string) string {
	name := sepsToSpaces(parent)
	name = reStripParentSeason.ReplaceAllString(name, "")
	name = strings.TrimRight(name, " -")
	return strings.TrimSpace(name)
}

// --- Greedy recovery for anime-dash (rule 9) ---

var reGreedyRecovery = regexp.MustCompile(
	`^(.+)\s-\s([0-9]{1,3})$`)

// --- Year resolution for group-release (rule 12) ---

var reTrailingYear = regexp.MustCompile(
	`^(.+?)\s+(19[0-9]{2}|20[0-9]{2})$`)

var reParentYear = regexp.MustCompile(
	`\(([0-9]{4}(-[0-9]{4})?)\)`)

func resolveGroupReleaseYear(show, parent string) string {
	m := reTrailingYear.FindStringSubmatch(show)
	if m == nil {
		return show
	}
	base := strings.TrimSpace(m[1])
	pm := reParentYear.FindStringSubmatch(parent)
	if pm != nil {
		return base + " (" + pm[1] + ")"
	}
	return base
}

// --- Compiled rule patterns (order matters) ---

var (
	reSxxExx = regexp.MustCompile(
		`(^|[^[:alnum:]])[Ss]([0-9]{1,2})[Ee]([0-9]{1,3})([Vv][0-9]+)?([^[:alnum:]]|$)`)

	re1x01 = regexp.MustCompile(
		`(^|[^0-9])([0-9]{1,2})[xX]([0-9]{1,3})([Vv][0-9]+)?([^0-9]|$)`)

	reSeasonOPED = regexp.MustCompile(
		`(?i)^(.*?)[\s_.\-]*[Ss]([0-9]{1,2})[\s_.\-]*(NC)?(OP|ED)([0-9]{0,2})([^[:alnum:]]|$)`)

	reCreditless = regexp.MustCompile(
		`(?i)^(\[.+\]\s*)?(.+)[\s_.\-]+([0-9]{1,3})\s*-\s+.*\[(Creditless\s+Opening|Creditless\s+Ending)\]`)

	reEpisodeKeyword = regexp.MustCompile(
		`(?i)^(\[.+\]\s*)?(.+)[\s_.\-]+[Ee]pisode[\s_.\-]+([0-9]{1,3})([._]([0-9]{1,2}))?(\s[^-]*)?\s*-\s+(.+)$`)

	reNamedSpecialIdx = regexp.MustCompile(
		`(?i)^(.+)[\s_.\-]+(OP|ED|PV|Special|Menu)[\s_.\-]*-\s*([0-9]{1,3})([^[:alnum:]]|$)`)

	reBareSpecial = regexp.MustCompile(
		`(?i)^(.+)\s*-\s*(Recap|Day\s+Breakers|BTS\s+Documentary|Convention\s+Panel)$`)

	reMoviePart = regexp.MustCompile(
		`(?i)^(.+\s+The\s+Movie)\s+([0-9]{1,2})\s*-\s*(.+)$`)

	reAnimeDash = regexp.MustCompile(
		`^(\[.+\])?\s*(.+)\s+-\s*([0-9]{1,3})(\s|\[|v[0-9]|$)`)

	reEpisodicTitle = regexp.MustCompile(
		`^(\[.+\]\s*)?(.+)[\s_.\-]+([0-9]{1,3})'?\s+-\s+(.+)$`)

	reBareNumberDash = regexp.MustCompile(
		`^([0-9]{1,3})'?\s*-\s*(.+)$`)

	reGroupRelease = regexp.MustCompile(
		`^(\[[^\]]+\]\s+)(.+)[\s_.\-]+([0-9]{1,3})'?([Vv][0-9]+)?(\s.*)?$`)

	reUnderscoreAnime = regexp.MustCompile(
		`^(\[.+\])?(.+)_([0-9]{2,3})(_[^.]*)?$`)

	reMovieYear = regexp.MustCompile(
		`(.+)[._\s]\(?((19[0-9]{2}|20[0-9]{2}))\)?`)
)

// Rules is the ordered parse-rule table. First match wins.
var Rules = []ParseRule{
	{"SxxExx", reSxxExx, extractSxxExx},
	{"1x01", re1x01, extract1x01},
	{"S01-OP/ED", reSeasonOPED, extractSeasonOPED},
	{"Creditless-OP/ED", reCreditless, extractCreditless},
	{"Episode-keyword", reEpisodeKeyword, extractEpisodeKeyword},
	{"Named-special-index", reNamedSpecialIdx, extractNamedSpecialIdx},
	{"Named-special-bare", reBareSpecial, extractBareSpecial},
	{"Movie-part", reMoviePart, extractMoviePart},
	{"Anime-dash", reAnimeDash, extractAnimeDash},
	{"Episodic-title", reEpisodicTitle, extractEpisodicTitle},
	{"Bare-number-dash", reBareNumberDash, extractBareNumberDash},
	{"Group-release", reGroupRelease, extractGroupRelease},
	{"Underscore-anime", reUnderscoreAnime, extractUnderscoreAnime},
	{"Movie-year", reMovieYear, extractMovieYear},
}

// --- Extract functions (one per rule) ---

func extractSxxExx(base string, matches []string, parent string) ParsedName {
	show := extractShowFromBase(base, reStripSxxExx)
	if show == "" {
		show = extractShowFromParent(parent)
	}
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  show,
		Season:    atoi(matches[2]),
		Episode:   atoi(matches[3]),
	}
}

func extract1x01(base string, matches []string, parent string) ParsedName {
	show := extractShowFromBase(base, reStrip1x01)
	if show == "" {
		show = extractShowFromParent(parent)
	}
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  show,
		Season:    atoi(matches[2]),
		Episode:   atoi(matches[3]),
	}
}

func extractSeasonOPED(_ string, matches []string, parent string) ParsedName {
	season := atoi(matches[2])
	num := 1
	if matches[5] != "" {
		num = atoi(matches[5])
	}
	kind := strings.ToUpper(matches[4])
	ep := 100 + num
	if kind == "ED" {
		ep = 200 + num
	}

	show := cleanName(matches[1])
	if show == "" {
		show = extractShowFromParent(parent)
	}
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  show,
		Season:    season,
		Episode:   ep,
	}
}

func extractCreditless(_ string, matches []string, _ string) ParsedName {
	show := cleanName(matches[2])
	num := atoi(matches[3])
	kind := strings.ToLower(matches[4])
	ep := 100 + num
	if strings.Contains(kind, "ending") {
		ep = 200 + num
	}
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  show,
		Season:    0,
		Episode:   ep,
	}
}

func extractEpisodeKeyword(_ string, matches []string, _ string) ParsedName {
	show := cleanName(matches[2])
	major := atoi(matches[3])
	minor := matches[5]

	season := 1
	ep := major
	if minor != "" {
		season = 0
		ep = atoi(matches[3] + minor)
	}
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  show,
		Season:    season,
		Episode:   ep,
	}
}

func extractNamedSpecialIdx(_ string, matches []string, _ string) ParsedName {
	show := cleanName(matches[1])
	show = strings.ReplaceAll(show, " - ", " ")
	kind := strings.ToUpper(matches[2])
	num := atoi(matches[3])

	var offset int
	switch kind {
	case "OP":
		offset = 100
	case "ED":
		offset = 200
	case "PV":
		offset = 300
	case "SPECIAL":
		offset = 400
	case "MENU":
		offset = 500
	default:
		offset = 900
	}
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  show,
		Season:    0,
		Episode:   offset + num,
	}
}

func extractBareSpecial(_ string, matches []string, _ string) ParsedName {
	show := cleanName(matches[1])
	show = strings.ReplaceAll(show, " - ", " ")
	kind := strings.ToLower(matches[2])

	var ep int
	switch kind {
	case "recap":
		ep = 601
	case "day breakers":
		ep = 602
	case "bts documentary":
		ep = 603
	case "convention panel":
		ep = 604
	default:
		ep = 699
	}
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  show,
		Season:    0,
		Episode:   ep,
	}
}

func extractMoviePart(_ string, matches []string, _ string) ParsedName {
	name := matches[1] + " " + matches[2] + " - " + matches[3]
	name = sepsToSpaces(name)
	name = stripBrackets(name)
	return ParsedName{
		MediaType: MediaMovie,
		MovieName: strings.TrimSpace(name),
	}
}

func extractAnimeDash(_ string, matches []string, _ string) ParsedName {
	show := strings.TrimSpace(matches[2])
	ep := atoi(matches[3])

	if m := reGreedyRecovery.FindStringSubmatch(show); m != nil {
		show = strings.TrimSpace(m[1])
		ep = atoi(m[2])
	}
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  cleanName(show),
		Season:    1,
		Episode:   ep,
	}
}

func extractEpisodicTitle(_ string, matches []string, _ string) ParsedName {
	show := sepsToSpaces(matches[2])
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  strings.TrimSpace(show),
		Season:    1,
		Episode:   atoi(matches[3]),
	}
}

func extractBareNumberDash(_ string, matches []string, parent string) ParsedName {
	show := sepsToSpaces(parent)
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  strings.TrimSpace(show),
		Season:    1,
		Episode:   atoi(matches[1]),
	}
}

func extractGroupRelease(_ string, matches []string, parent string) ParsedName {
	show := sepsToSpaces(matches[2])
	show = strings.TrimRight(show, " -")
	show = strings.TrimSpace(show)
	show = resolveGroupReleaseYear(show, parent)
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  show,
		Season:    1,
		Episode:   atoi(matches[3]),
	}
}

func extractUnderscoreAnime(_ string, matches []string, _ string) ParsedName {
	show := strings.ReplaceAll(matches[2], "_", " ")
	return ParsedName{
		MediaType: MediaTV,
		ShowName:  strings.TrimSpace(show),
		Season:    1,
		Episode:   atoi(matches[3]),
	}
}

func extractMovieYear(_ string, matches []string, _ string) ParsedName {
	name := sepsToSpaces(matches[1])
	return ParsedName{
		MediaType: MediaMovie,
		MovieName: strings.TrimSpace(name),
		Year:      matches[2],
	}
}
