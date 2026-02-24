package naming

import (
	"path/filepath"
	"regexp"
	"strings"
)

// YearVariantIndex maps a base show name (without year tag) to the set of
// year-tagged variants seen across the input file list. This allows
// harmonization: if "Show" appears only as "Show (2019)", bare references
// to "Show" can be upgraded to "Show (2019)".
type YearVariantIndex map[string][]string

var reShowYearTag = regexp.MustCompile(
	`^(.+)\s+\((19[0-9]{2}|20[0-9]{2})(-[0-9]{4})?\)$`)

// extractShowBaseAndYear splits "Show Name (2019)" into ("Show Name", "2019")
// or ("Show Name (2019-2020)", "2019-2020"). Returns empty yearTag if no
// year suffix is present.
func extractShowBaseAndYear(showName string) (base, yearTag string) {
	m := reShowYearTag.FindStringSubmatch(showName)
	if m == nil {
		return showName, ""
	}
	return strings.TrimSpace(m[1]), m[2] + m[3]
}

// BuildYearVariantIndex scans all file paths, parses each filename to
// extract the show name, and registers any year-tagged variants. The
// resulting index is used by [HarmonizeShowName].
func BuildYearVariantIndex(files []string) YearVariantIndex {
	idx := make(YearVariantIndex)
	for _, f := range files {
		p := ParseFilename(filepath.Base(f), filepath.Dir(f))
		if p.MediaType != MediaTV || p.ShowName == "" {
			continue
		}
		registerVariant(idx, p.ShowName)
	}
	return idx
}

func registerVariant(idx YearVariantIndex, showName string) {
	base, yearTag := extractShowBaseAndYear(showName)
	if yearTag == "" || base == "" {
		return
	}
	for _, v := range idx[base] {
		if v == showName {
			return
		}
	}
	idx[base] = append(idx[base], showName)
}

// HarmonizeShowName checks whether a bare show name (no year tag) should be
// upgraded to a year-tagged variant. This happens only when exactly one
// year-tagged variant exists in the index.
func HarmonizeShowName(showName string, idx YearVariantIndex) string {
	_, yearTag := extractShowBaseAndYear(showName)
	if yearTag != "" {
		return showName
	}

	variants := idx[showName]
	if len(variants) == 1 {
		return variants[0]
	}
	return showName
}
