// parser.go parses filenames via ordered regex rule matching.
package naming

import (
	"path/filepath"
	"strings"
)

// MediaType distinguishes TV series from movies.
type MediaType string

const (
	MediaTV    MediaType = "tv"
	MediaMovie MediaType = "movie"
)

// ParsedName holds the structured result of filename parsing.
type ParsedName struct {
	MediaType  MediaType
	ShowName   string
	Season     int
	Episode    int
	EpisodeEnd int
	MovieName  string
	Year       string
}

// ParseFilename parses a media filename into structured naming components.
// basename is the filename (with extension). parentPath is the directory
// path (used for show-name fallback and context-folder detection).
func ParseFilename(basename, parentPath string) ParsedName {
	ext := filepath.Ext(basename)
	base := strings.TrimSuffix(basename, ext)

	parent, seasonHintParent := resolveParentContexts(parentPath)

	for _, rule := range Rules {
		m := rule.Pattern.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		parsed := rule.Extract(base, m, parent, seasonHintParent)
		return postProcess(parsed, seasonHintParent)
	}

	// Rule 15: Fallback — treat entire basename as movie title.
	name := sepsToSpaces(base)
	parsed := ParsedName{
		MediaType: MediaMovie,
		MovieName: strings.TrimSpace(name),
	}
	return postProcess(parsed, seasonHintParent)
}

// resolveParentContext determines the directory name used as naming context.
// If the immediate parent is a context folder (Season NN, Extras, NCOP, etc.),
// the grandparent is returned instead.
func resolveParentContext(parentPath string) string {
	context, _ := resolveParentContexts(parentPath)
	return context
}

func resolveParentContexts(parentPath string) (context, seasonHintParent string) {
	parent := immediateParent(parentPath)
	if parent == "" {
		return "", ""
	}

	if isContextFolder(strings.ToLower(parent)) {
		grandparent := filepath.Base(filepath.Dir(parentPath))
		if grandparent != "" && grandparent != "." && grandparent != string(filepath.Separator) {
			return grandparent, parent
		}
	}
	return parent, parent
}

func immediateParent(parentPath string) string {
	if parentPath == "" {
		return ""
	}
	if !strings.Contains(parentPath, string(filepath.Separator)) &&
		!strings.Contains(parentPath, "/") {
		return parentPath
	}
	return filepath.Base(parentPath)
}

func isContextFolder(name string) bool {
	return isSpecialsFolder(name) || extractParentSeasonHint(name) > 0
}

func isSpecialsFolder(name string) bool {
	switch name {
	case "extras", "extra", "specials", "bonus", "featurettes", "nc":
		return true
	}
	return strings.HasPrefix(name, "ncop") || strings.HasPrefix(name, "nced")
}
