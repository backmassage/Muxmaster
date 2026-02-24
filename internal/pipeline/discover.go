package pipeline

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Supported media file extensions (lowercase, with leading dot).
var mediaExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".m4v":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".webm": true,
	".ts":   true,
	".m2ts": true,
	".mpg":  true,
	".mpeg": true,
	".vob":  true,
	".ogv":  true,
}

// Discover walks inputDir, collects files with media extensions, prunes
// directories containing bonus/extras content (case-insensitive), and returns
// the paths sorted lexicographically for deterministic processing order.
//
// Pruned directories: extras, extra, bonus, featurettes. These contain
// behind-the-scenes and supplemental content that should not be batch-encoded.
//
// NOT pruned: specials, nc, ncop*, nced*. These contain actual episodes
// (openings, endings, specials) and are processed normally — the naming
// module derives the correct show name from the grandparent directory.
func Discover(inputDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if isExtrasDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if mediaExtensions[ext] {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// isExtrasDir returns true for directory names that contain bonus/supplemental
// content which should be excluded from batch encoding.
func isExtrasDir(name string) bool {
	switch strings.ToLower(name) {
	case "extras", "extra", "bonus", "featurettes":
		return true
	}
	return false
}
