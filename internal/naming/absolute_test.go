package naming

import "testing"

// splitAbsolute is exercised with synthetic boundaries so the test never
// depends on any specific real-world series. starts here describes a 3-season
// show: S1 eps 1-4, S2 eps 5-8, S3 eps 9+.
func TestSplitAbsolute(t *testing.T) {
	starts := []int{1, 5, 9}
	cases := []struct {
		abs        int
		wantSeason int
		wantEp     int
	}{
		{1, 1, 1},  // first episode
		{4, 1, 4},  // last of season 1
		{5, 2, 1},  // first of season 2 (boundary)
		{8, 2, 4},  // last of season 2
		{9, 3, 1},  // first of season 3 (final, open-ended)
		{14, 3, 6}, // well into the final season
		{0, 1, 0},  // below range clamps to season 1
	}
	for _, c := range cases {
		gotS, gotE := splitAbsolute(starts, c.abs)
		if gotS != c.wantSeason || gotE != c.wantEp {
			t.Errorf("splitAbsolute(%v, %d) = S%dE%d, want S%dE%d",
				starts, c.abs, gotS, gotE, c.wantSeason, c.wantEp)
		}
	}
}

// TestNewAbsoluteRule drives the exact code path ParseFilename uses for an
// absolute-numbered show, with a synthetic placeholder show rather than any
// real title.
func TestNewAbsoluteRule(t *testing.T) {
	show := absoluteShow{canonical: "Placeholder Show", starts: []int{1, 5, 9}}
	rule := newAbsoluteRule(show)

	t.Run("matches and splits", func(t *testing.T) {
		cases := []struct {
			base       string
			wantSeason int
			wantEp     int
		}{
			{"Placeholder.Show.01.1080p.BluRay.x264-GRP", 1, 1},
			{"Placeholder.Show.07.720p.WEB", 2, 3},
			{"Placeholder Show 12 [tag]", 3, 4},
			{"placeholder.show.05", 2, 1}, // case-insensitive, number at end
		}
		for _, c := range cases {
			m := rule.Pattern.FindStringSubmatch(c.base)
			if m == nil {
				t.Errorf("%q: expected match, got none", c.base)
				continue
			}
			p := rule.Extract(c.base, m, "", "")
			if p.MediaType != MediaTV || p.ShowName != "Placeholder Show" ||
				p.Season != c.wantSeason || p.Episode != c.wantEp {
				t.Errorf("%q: got type=%s show=%q S%dE%d, want tv/Placeholder Show/S%dE%d",
					c.base, p.MediaType, p.ShowName, p.Season, p.Episode, c.wantSeason, c.wantEp)
			}
		}
	})

	t.Run("guards against false positives", func(t *testing.T) {
		// 4-digit year is not a 1-3 digit episode; a different show does not match.
		for _, base := range []string{
			"Placeholder.Show.2014.1080p.BluRay", // year, not an episode
			"Different.Show.07.1080p",            // not the allowlisted title
			"Placeholder.Show.Movie.1080p",       // no bare number after the title
		} {
			if m := rule.Pattern.FindStringSubmatch(base); m != nil {
				t.Errorf("%q: expected no match, got %v", base, m)
			}
		}
	})
}

// TestAbsoluteRulesWiring confirms the absolute rules are spliced into the live
// table ahead of the broad Movie-year rule (so a dotted scene name is claimed as
// an episode before it can be mistaken for a movie).
func TestAbsoluteRulesWiring(t *testing.T) {
	firstAbsolute, movieYear := -1, -1
	for i, r := range Rules {
		if firstAbsolute == -1 && len(r.Name) >= 9 && r.Name[:9] == "Absolute-" {
			firstAbsolute = i
		}
		if r.Name == "Movie-year" {
			movieYear = i
		}
	}
	if firstAbsolute == -1 {
		t.Fatal("no Absolute-scene rule found in Rules table")
	}
	if movieYear == -1 {
		t.Fatal("Movie-year rule missing from Rules table")
	}
	if firstAbsolute >= movieYear {
		t.Errorf("absolute rules at %d must precede Movie-year at %d", firstAbsolute, movieYear)
	}
}
