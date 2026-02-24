package naming

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseFilename(t *testing.T) {
	cases := []struct {
		name      string
		basename  string
		parentDir string

		wantType    MediaType
		wantShow    string
		wantSeason  int
		wantEpisode int
		wantMovie   string
		wantYear    string
	}{
		// Rule 1: SxxExx
		{
			name: "SxxExx standard", basename: "My.Show.S01E05.720p.BluRay.mkv",
			parentDir: "/media/My Show",
			wantType: MediaTV, wantShow: "My Show", wantSeason: 1, wantEpisode: 5,
		},
		{
			name: "SxxExx with version", basename: "Show.S02E10v2.HEVC.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 2, wantEpisode: 10,
		},
		{
			name: "SxxExx show from parent", basename: "S03E01.1080p.mkv",
			parentDir: "/media/Cool Show",
			wantType: MediaTV, wantShow: "Cool Show", wantSeason: 3, wantEpisode: 1,
		},

		// Rule 2: 1x01
		{
			name: "1x01 format", basename: "Show.Name.1x05.mkv",
			parentDir: "/media/Show Name",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 5,
		},

		// Rule 3: Season OP/ED
		{
			name: "S01 NCOP", basename: "Show.S01.NCOP1.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 1, wantEpisode: 101,
		},
		{
			name: "S02 NCED", basename: "Show.S02.NCED2.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 2, wantEpisode: 202,
		},

		// Rule 4: Creditless OP/ED
		{
			name: "Creditless Opening", basename: "[Group] Show - 001 - Title [Creditless Opening].mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 0, wantEpisode: 101,
		},

		// Rule 5: Episode keyword
		{
			name: "Episode keyword", basename: "[SubGroup] Show Name - Episode 16 - Title.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 16,
		},
		{
			name: "Episode keyword fractional", basename: "[SubGroup] Show - Episode 16.5 - Title.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 0, wantEpisode: 165,
		},

		// Rule 6: Named special index
		{
			name: "Named special OP", basename: "Show OP-01.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 0, wantEpisode: 101,
		},
		{
			name: "Named special PV", basename: "Show PV-03.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 0, wantEpisode: 303,
		},

		// Rule 7: Named special bare
		{
			name: "Bare special Recap", basename: "Show Name - Recap.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 0, wantEpisode: 601,
		},

		// Rule 8: Movie part
		{
			name: "Movie part", basename: "Title The Movie 2 - Part Two.mkv",
			parentDir: "/media/Movies",
			wantType: MediaMovie, wantMovie: "Title The Movie 2 - Part Two",
		},

		// Rule 9: Anime dash
		{
			name: "Anime dash standard", basename: "[SubGroup] Anime Name - 12 [1080p].mkv",
			parentDir: "/media/Anime Name",
			wantType: MediaTV, wantShow: "Anime Name", wantSeason: 1, wantEpisode: 12,
		},
		{
			name: "Anime dash greedy recovery", basename: "[SubGroup] Show - 027 - 800 Years of History.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 1, wantEpisode: 27,
		},

		// Rule 10: Episodic title
		{
			name: "Episodic title", basename: "[Group] Show Name 05 - Episode Title.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 5,
		},

		// Rule 11: Bare number dash
		{
			name: "Bare number dash", basename: "03 - Episode Title.mkv",
			parentDir: "/media/Show Name",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 3,
		},

		// Rule 12: Group release
		{
			name: "Group release", basename: "[SubGroup] Show Name 05 [1080p].mkv",
			parentDir: "/media/Show Name",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 5,
		},

		// Rule 13: Underscore anime
		{
			name: "Underscore anime", basename: "[Group]Show_Name_01_BD.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 1,
		},

		// Rule 14: Movie year
		{
			name: "Movie with year", basename: "The.Matrix.1999.mkv",
			parentDir: "/media/Movies",
			wantType: MediaMovie, wantMovie: "The Matrix", wantYear: "1999",
		},

		// Rule 15: Fallback
		{
			name: "Fallback movie", basename: "Random Movie Title.mkv",
			parentDir: "/media/Movies",
			wantType: MediaMovie, wantMovie: "Random Movie Title",
		},

		// Edge: specials folder — bare-number file inside NCOP gets grandparent
		{
			name: "Specials folder grandparent", basename: "01 - Title.mkv",
			parentDir: "/media/Show Name/NCOP",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 1,
		},

		// Edge: season hint from parent overrides default season 1
		{
			name: "Season hint from parent", basename: "[Group] Show 03 [Tags].mkv",
			parentDir: "Season 03",
			wantType: MediaTV, wantShow: "Show", wantSeason: 3, wantEpisode: 3,
		},

		// Edge: release tag stripping
		{
			name: "Release tag stripping", basename: "Show.Name.S01E01.1080p.BluRay.x265.HEVC.mkv",
			parentDir: "/media/Show Name",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseFilename(tc.basename, tc.parentDir)

			if got.MediaType != tc.wantType {
				t.Errorf("type: got %q, want %q", got.MediaType, tc.wantType)
			}
			if tc.wantType == MediaTV {
				if !strings.EqualFold(got.ShowName, tc.wantShow) {
					t.Errorf("show: got %q, want %q", got.ShowName, tc.wantShow)
				}
				if got.Season != tc.wantSeason {
					t.Errorf("season: got %d, want %d", got.Season, tc.wantSeason)
				}
				if got.Episode != tc.wantEpisode {
					t.Errorf("episode: got %d, want %d", got.Episode, tc.wantEpisode)
				}
			}
			if tc.wantType == MediaMovie {
				if !strings.EqualFold(got.MovieName, tc.wantMovie) {
					t.Errorf("movie: got %q, want %q", got.MovieName, tc.wantMovie)
				}
				if got.Year != tc.wantYear {
					t.Errorf("year: got %q, want %q", got.Year, tc.wantYear)
				}
			}
		})
	}
}

func TestGetOutputPath(t *testing.T) {
	cases := []struct {
		name string
		p    ParsedName
		want string
	}{
		{
			name: "TV show",
			p:    ParsedName{MediaType: MediaTV, ShowName: "My Show", Season: 1, Episode: 5},
			want: "/output/My Show/Season 01/My Show - S01E05.mkv",
		},
		{
			name: "Movie with year",
			p:    ParsedName{MediaType: MediaMovie, MovieName: "The Matrix", Year: "1999"},
			want: "/output/The Matrix (1999)/The Matrix (1999).mkv",
		},
		{
			name: "Movie without year",
			p:    ParsedName{MediaType: MediaMovie, MovieName: "Cool Film"},
			want: "/output/Cool Film/Cool Film.mkv",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetOutputPath(tc.p, "/output", "mkv")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCollisionResolver(t *testing.T) {
	cr := NewCollisionResolver()

	out1 := cr.Resolve("/input/a.mkv", "/output/Show/Season 01/Show - S01E01.mkv")
	if out1 != "/output/Show/Season 01/Show - S01E01.mkv" {
		t.Errorf("first claim: got %q", out1)
	}

	out2 := cr.Resolve("/input/b.mkv", "/output/Show/Season 01/Show - S01E01.mkv")
	want2 := "/output/Show/Season 01/Show - S01E01 - dup1.mkv"
	if out2 != want2 {
		t.Errorf("dup1: got %q, want %q", out2, want2)
	}

	out3 := cr.Resolve("/input/c.mkv", "/output/Show/Season 01/Show - S01E01.mkv")
	want3 := "/output/Show/Season 01/Show - S01E01 - dup2.mkv"
	if out3 != want3 {
		t.Errorf("dup2: got %q, want %q", out3, want3)
	}

	// Same input claiming same output is idempotent.
	out1b := cr.Resolve("/input/a.mkv", "/output/Show/Season 01/Show - S01E01.mkv")
	if out1b != "/output/Show/Season 01/Show - S01E01.mkv" {
		t.Errorf("re-claim: got %q", out1b)
	}
}

func TestHarmonize(t *testing.T) {
	idx := make(YearVariantIndex)
	idx["Show"] = []string{"Show (2019)"}

	got := HarmonizeShowName("Show", idx)
	if got != "Show (2019)" {
		t.Errorf("single variant: got %q, want %q", got, "Show (2019)")
	}

	idx["Other"] = []string{"Other (2018)", "Other (2020)"}
	got2 := HarmonizeShowName("Other", idx)
	if got2 != "Other" {
		t.Errorf("multiple variants: got %q, want %q", got2, "Other")
	}

	got3 := HarmonizeShowName("Show (2019)", idx)
	if got3 != "Show (2019)" {
		t.Errorf("already tagged: got %q, want %q", got3, "Show (2019)")
	}
}

func TestExtractShowBaseAndYear(t *testing.T) {
	cases := []struct {
		input    string
		wantBase string
		wantYear string
	}{
		{"Show (2019)", "Show", "2019"},
		{"Show (2019-2020)", "Show", "2019-2020"},
		{"Plain Show", "Plain Show", ""},
		{"Show 2019", "Show 2019", ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			base, year := extractShowBaseAndYear(tc.input)
			if base != tc.wantBase || year != tc.wantYear {
				t.Errorf("got (%q, %q), want (%q, %q)", base, year, tc.wantBase, tc.wantYear)
			}
		})
	}
}

func TestRuleMatching(t *testing.T) {
	inputs := []struct {
		name     string
		basename string
		wantRule string
	}{
		{"SxxExx", "My.Show.S01E05.720p.mkv", "SxxExx"},
		{"1x01", "Show.1x05.mkv", "1x01"},
		{"Season OPED", "Show.S01.NCOP1.mkv", "S01-OP/ED"},
		{"Creditless", "[G] Show - 001 - T [Creditless Opening].mkv", "Creditless-OP/ED"},
		{"Episode kw", "[G] Show - Episode 16 - Title.mkv", "Episode-keyword"},
		{"Named special idx", "Show OP-01.mkv", "Named-special-index"},
		{"Named special bare", "Show - Recap.mkv", "Named-special-bare"},
		{"Movie part", "Title The Movie 2 - Part.mkv", "Movie-part"},
		{"Anime dash", "[G] Anime - 12 [Tags].mkv", "Anime-dash"},
		{"Episodic title", "[G] Show 05 - Title.mkv", "Episodic-title"},
		{"Bare number", "03 - Title.mkv", "Bare-number-dash"},
		{"Group release", "[Group] Show 05 [Tags].mkv", "Group-release"},
		{"Underscore", "[G]Show_01_BD.mkv", "Underscore-anime"},
		{"Movie year", "Matrix.1999.mkv", "Movie-year"},
	}

	for _, tc := range inputs {
		t.Run(tc.name, func(t *testing.T) {
			ext := ".mkv"
			base := strings.TrimSuffix(tc.basename, ext)
			matched := false
			for _, rule := range Rules {
				if rule.Pattern.FindStringSubmatch(base) != nil {
					if rule.Name != tc.wantRule {
						t.Errorf("matched rule %q, want %q", rule.Name, tc.wantRule)
					}
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("no rule matched for %q", tc.basename)
			}
		})
	}
}

func TestSpecialsFolder(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/media/Show/NCOP", "Show"},
		{"/media/Show/NCED01", "Show"},
		{"/media/Show/Extras", "Show"},
		{"/media/Show/Season 01", "Season 01"},
		{"JustDir", "JustDir"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := resolveParentContext(tc.path)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTitleCase(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"hello world", "Hello World"},
		{"the-dark-knight", "The-Dark-Knight"},
		{"under_score_test", "Under_Score_Test"},
		{"ALREADY UPPER", "ALREADY UPPER"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := titleCase(tc.input)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSeasonHint(t *testing.T) {
	cases := []struct {
		parent string
		want   int
	}{
		{"Season 03", 3},
		{"Season_2", 2},
		{"S4", 4},
		{"Just A Dir", 0},
	}
	for _, tc := range cases {
		t.Run(tc.parent, func(t *testing.T) {
			got := extractParentSeasonHint(tc.parent)
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestStripReleaseTags(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Show Name 1080p BluRay x265", "Show Name"},
		{"Show Name HEVC", "Show Name"},
		{"Clean Title", "Clean Title"},
		{"720p At Start", ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := stripReleaseTags(tc.input)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStripBrackets(t *testing.T) {
	got := stripBrackets("[Group] Show Name [1080p]")
	want := "Show Name"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Edge-case regression tests identified during debug audit.
func TestEdgeCases(t *testing.T) {
	cases := []struct {
		name      string
		basename  string
		parentDir string

		wantType    MediaType
		wantShow    string
		wantSeason  int
		wantEpisode int
		wantMovie   string
		wantYear    string
	}{
		// Movie year: greedy (.+) captures last year in filename.
		{
			name: "Movie double year picks last", basename: "Blade Runner 2049 2017.mkv",
			parentDir: "/media/Movies",
			wantType: MediaMovie, wantMovie: "Blade Runner 2049", wantYear: "2017",
		},
		// Movie year with parens: "(2019)".
		{
			name: "Movie year in parens", basename: "Title (2019).mkv",
			parentDir: "/media/Movies",
			wantType: MediaMovie, wantMovie: "Title", wantYear: "2019",
		},
		// Season 0 rules must NOT apply season hint (e.g. OP/ED specials).
		{
			name: "Season 0 ignores hint", basename: "Show OP-01.mkv",
			parentDir: "Season 03",
			wantType: MediaTV, wantShow: "Show", wantSeason: 0, wantEpisode: 101,
		},
		// Group release with trailing year + parent year range.
		{
			name: "Group release year from parent", basename: "[Group] Show 2019 05 [Tags].mkv",
			parentDir: "Show (2019-2020)",
			wantType: MediaTV, wantShow: "Show (2019-2020)", wantSeason: 1, wantEpisode: 5,
		},
		// Group release with trailing year + parent without year tag.
		{
			name: "Group release year stripped", basename: "[Group] Show 2019 05 [Tags].mkv",
			parentDir: "Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 1, wantEpisode: 5,
		},
		// SxxExx: show name extracted from text before token.
		{
			name: "SxxExx complex name", basename: "The.100.S03E05.720p.mkv",
			parentDir: "/media/The 100",
			wantType: MediaTV, wantShow: "The 100", wantSeason: 3, wantEpisode: 5,
		},
		// Underscore anime without group tag.
		{
			name: "Underscore anime no group", basename: "Show_Name_05_BD.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 5,
		},
		// Specials folder: extras.
		{
			name: "Extras folder grandparent", basename: "05 - Behind the Scenes.mkv",
			parentDir: "/media/Show Name/Extras",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 5,
		},
		// NCED with numeric suffix.
		{
			name: "NCED folder grandparent", basename: "01 - Ending Theme.mkv",
			parentDir: "/media/Show Name/NCED01",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 1,
		},
		// Non-specials folder stays as-is.
		{
			name: "Regular folder no grandparent", basename: "01 - Title.mkv",
			parentDir: "/media/Show Name/Season 01",
			wantType: MediaTV, wantShow: "Season 01", wantSeason: 1, wantEpisode: 1,
		},
		// Episodic title with apostrophe.
		{
			name: "Episodic with apostrophe", basename: "[Group] Show Name 21' - Title.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 1, wantEpisode: 21,
		},
		// Bare-number dash with version.
		{
			name: "Bare number with apostrophe", basename: "03' - Title.mkv",
			parentDir: "My Show",
			wantType: MediaTV, wantShow: "My Show", wantSeason: 1, wantEpisode: 3,
		},
		// Creditless Ending variant.
		{
			name: "Creditless Ending", basename: "[Group] Show - 002 - Title [Creditless Ending].mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 0, wantEpisode: 202,
		},
		// Named special: Menu.
		{
			name: "Named special Menu", basename: "Show Menu-02.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show", wantSeason: 0, wantEpisode: 502,
		},
		// Bare special: Convention Panel.
		{
			name: "Convention panel special", basename: "Show Name - Convention Panel.mkv",
			parentDir: "/media/Show",
			wantType: MediaTV, wantShow: "Show Name", wantSeason: 0, wantEpisode: 604,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseFilename(tc.basename, tc.parentDir)

			if got.MediaType != tc.wantType {
				t.Errorf("type: got %q, want %q", got.MediaType, tc.wantType)
			}
			if tc.wantType == MediaTV {
				if !strings.EqualFold(got.ShowName, tc.wantShow) {
					t.Errorf("show: got %q, want %q", got.ShowName, tc.wantShow)
				}
				if got.Season != tc.wantSeason {
					t.Errorf("season: got %d, want %d", got.Season, tc.wantSeason)
				}
				if got.Episode != tc.wantEpisode {
					t.Errorf("episode: got %d, want %d", got.Episode, tc.wantEpisode)
				}
			}
			if tc.wantType == MediaMovie {
				if !strings.EqualFold(got.MovieName, tc.wantMovie) {
					t.Errorf("movie: got %q, want %q", got.MovieName, tc.wantMovie)
				}
				if got.Year != tc.wantYear {
					t.Errorf("year: got %q, want %q", got.Year, tc.wantYear)
				}
			}
		})
	}
}

// Verbose debug output for manual inspection.
func TestDebugSampleFiles(t *testing.T) {
	samples := []struct {
		basename  string
		parentDir string
	}{
		{"My.Show.S01E05.720p.BluRay.x265.HEVC-Group.mkv", "/media/My Show/Season 01"},
		{"[SubGroup] Anime Name - 12 [1080p][HEVC].mkv", "/media/Anime Name"},
		{"The.Matrix.1999.BluRay.mkv", "/media/Movies"},
		{"03 - Episode Title.mkv", "/media/Cool Show/Season 03"},
		{"[Group] Show - 027 - 800 Years of History [1080p].mkv", "/media/Show"},
		{"Show.S01.NCOP1.mkv", "/media/Show/NCOP"},
		{"[Group]Show_Name_01_BD.mkv", "/media/Show Name"},
		{"[SubGroup] Show Name 05 [1080p][HEVC].mkv", "/media/Show Name (2019)"},
		{"Random Movie Title.mkv", "/media/Movies"},
	}

	for _, s := range samples {
		p := ParseFilename(s.basename, s.parentDir)
		out := GetOutputPath(p, "/output", "mkv")
		t.Logf("%-60s → %s | show=%q movie=%q S%02dE%02d year=%s",
			fmt.Sprintf("%s [%s]", s.basename, s.parentDir),
			p.MediaType, p.ShowName, p.MovieName,
			p.Season, p.Episode, p.Year)
		t.Logf("  output: %s", out)
	}
}
