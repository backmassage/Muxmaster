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
// directories named "extras" (case-insensitive), and returns the paths
// sorted lexicographically for deterministic processing order.
func Discover(inputDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), "extras") {
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
