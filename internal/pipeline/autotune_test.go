package pipeline

import (
	"bufio"
	"strings"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/naming"
	"github.com/backmassage/muxmaster/internal/tune"
)

func TestParseTuneChoice(t *testing.T) {
	cases := []struct {
		in       string
		sug      config.TuneMode
		wantTune config.TuneMode
		wantOK   bool
	}{
		{"", config.TuneFilm, config.TuneFilm, true},         // empty → suggestion
		{"\n", config.TuneGrain, config.TuneGrain, true},     // bare newline → suggestion
		{"none", config.TuneFilm, config.TuneNone, true},     // full word
		{"n", config.TuneFilm, config.TuneNone, true},        // shorthand
		{"FILM", config.TuneNone, config.TuneFilm, true},     // case-insensitive
		{" grain ", config.TuneNone, config.TuneGrain, true}, // trimmed
		{"a", config.TuneNone, config.TuneAnime, true},
		{"anime", config.TuneNone, config.TuneAnime, true},
		{"bogus", config.TuneGrain, config.TuneGrain, false}, // unrecognized → suggestion, !ok
	}
	for _, c := range cases {
		gotTune, gotOK := parseTuneChoice(c.in, c.sug)
		if gotTune != c.wantTune || gotOK != c.wantOK {
			t.Errorf("parseTuneChoice(%q, %s) = (%s, %v), want (%s, %v)",
				c.in, c.sug, gotTune, gotOK, c.wantTune, c.wantOK)
		}
	}
}

func TestSeriesKey(t *testing.T) {
	cases := []struct {
		name      string
		parsed    naming.ParsedName
		path      string
		wantKey   string
		wantLabel string
	}{
		{
			"tv groups by show",
			naming.ParsedName{MediaType: naming.MediaTV, ShowName: "Breaking Bad", Season: 2, Episode: 5},
			"/m/Breaking Bad/S02E05.mkv",
			"tv:breaking bad", "Breaking Bad",
		},
		{
			"movie groups by name+year",
			naming.ParsedName{MediaType: naming.MediaMovie, MovieName: "Dune", Year: "2021"},
			"/m/Dune (2021).mkv",
			"movie:dune:2021", "Dune (2021)",
		},
		{
			"unparsed tv falls back to filename",
			naming.ParsedName{MediaType: naming.MediaTV, ShowName: ""},
			"/m/mystery.mkv",
			"file:mystery.mkv", "mystery.mkv",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key, label := seriesKey(c.parsed, c.path)
			if key != c.wantKey || label != c.wantLabel {
				t.Errorf("seriesKey = (%q, %q), want (%q, %q)", key, label, c.wantKey, c.wantLabel)
			}
		})
	}
}

// TestSeriesKey_TVEpisodesShareKey is the load-bearing property: every episode
// of a series must collapse to one key so the prompt fires once per series.
func TestSeriesKey_TVEpisodesShareKey(t *testing.T) {
	a, _ := seriesKey(naming.ParsedName{MediaType: naming.MediaTV, ShowName: "The Wire", Season: 1, Episode: 1}, "/m/The Wire/S01E01.mkv")
	b, _ := seriesKey(naming.ParsedName{MediaType: naming.MediaTV, ShowName: "The Wire", Season: 3, Episode: 9}, "/m/The Wire/S03E09.mkv")
	if a != b {
		t.Errorf("episodes of one show must share a key: %q != %q", a, b)
	}
}

func TestGroupBySeries(t *testing.T) {
	// Two real series-shaped names plus a bare filename. Uses the real parser
	// via seriesKeyForPath, so this also guards the parse+group integration.
	files := []string{
		"/m/Show.Name.S01E01.1080p.mkv",
		"/m/Show.Name.S01E02.1080p.mkv",
		"/m/Show.Name.S02E01.1080p.mkv",
		"/m/Other.Show.S01E01.mkv",
	}
	yearIndex := naming.BuildYearVariantIndex(files)
	groups := groupBySeries(files, yearIndex)

	if len(groups) != 2 {
		t.Fatalf("expected 2 series groups, got %d: %+v", len(groups), groups)
	}
	// First group is the 3-episode show; representative is the first file.
	if groups[0].count != 3 {
		t.Errorf("first group count: got %d, want 3", groups[0].count)
	}
	if groups[0].rep != files[0] {
		t.Errorf("representative: got %s, want %s", groups[0].rep, files[0])
	}
	if groups[1].count != 1 {
		t.Errorf("second group count: got %d, want 1", groups[1].count)
	}
}

func TestPromptSeriesTune(t *testing.T) {
	g := seriesGroup{key: "tv:show", label: "Show", rep: "/m/Show.S01E01.mkv", count: 6}
	sug := tune.Suggestion{Tune: config.TuneFilm, Confidence: tune.ConfidenceHigh, Reason: "light grain (grain=0.30)"}

	// Empty line accepts the suggestion.
	if got := promptSeriesTune(&strings.Builder{}, bufio.NewReader(strings.NewReader("\n")), g, sug); got != config.TuneFilm {
		t.Errorf("empty line: got %s, want %s (suggestion)", got, config.TuneFilm)
	}
	// Explicit override wins.
	if got := promptSeriesTune(&strings.Builder{}, bufio.NewReader(strings.NewReader("grain\n")), g, sug); got != config.TuneGrain {
		t.Errorf("override: got %s, want grain", got)
	}
	// Unrecognized falls back to the suggestion.
	if got := promptSeriesTune(&strings.Builder{}, bufio.NewReader(strings.NewReader("xyz\n")), g, sug); got != config.TuneFilm {
		t.Errorf("unrecognized: got %s, want %s (suggestion)", got, config.TuneFilm)
	}
	// Closed stdin (EOF, no newline) falls back to the suggestion.
	if got := promptSeriesTune(&strings.Builder{}, bufio.NewReader(strings.NewReader("")), g, sug); got != config.TuneFilm {
		t.Errorf("EOF: got %s, want %s (suggestion)", got, config.TuneFilm)
	}

	// The prompt surfaces the series label, sample reason, default, and the
	// suggested option bracketed (plain text in tests — colors are unconfigured).
	var out strings.Builder
	promptSeriesTune(&out, bufio.NewReader(strings.NewReader("\n")), g, sug)
	for _, want := range []string{"Show", "light grain", "Enter = film", "[film]", "none", "grain", "anime"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("prompt output missing %q:\n%s", want, out.String())
		}
	}
	// Only the suggested option is bracketed.
	if strings.Contains(out.String(), "[grain]") {
		t.Errorf("non-suggested option should not be bracketed:\n%s", out.String())
	}
}

// TestPromptSeriesTune_RepromptsUntilValid verifies invalid entries re-prompt
// rather than silently accepting the default, and that a later valid entry wins.
func TestPromptSeriesTune_RepromptsUntilValid(t *testing.T) {
	g := seriesGroup{key: "tv:show", label: "Show", rep: "/m/Show.S01E01.mkv", count: 3}
	sug := tune.Suggestion{Tune: config.TuneNone, Confidence: tune.ConfidenceHigh, Reason: "clean source (grain=0.05)"}

	var out strings.Builder
	got := promptSeriesTune(&out, bufio.NewReader(strings.NewReader("huh\n???\ngrain\n")), g, sug)
	if got != config.TuneGrain {
		t.Errorf("got %s, want grain (after two invalid entries)", got)
	}
	// Two invalid entries → two re-prompt hints.
	if n := strings.Count(out.String(), "type none / film / grain / anime"); n != 2 {
		t.Errorf("expected 2 re-prompt hints, got %d:\n%s", n, out.String())
	}
}
