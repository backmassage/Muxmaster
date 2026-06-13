// outputpath.go generates Jellyfin-style output directories and file paths.
package naming

import (
	"fmt"
	"path/filepath"
)

// GetOutputPath builds the canonical output file path for a parsed name.
// container is the file extension without dot (e.g. "mkv", "mp4").
//
//	TV:    <outputDir>/<ShowName>/Season XX/<ShowName> - SXXEXX.<ext>
//	       <outputDir>/<ShowName>/Season XX/<ShowName> - SXXEXX-EXX.<ext> for multi-episode files
//	Movie: <outputDir>/<Name (Year)>/<Name (Year)>.<ext>    (or <Name>/<Name>.<ext> if no year)
func GetOutputPath(p ParsedName, outputDir, container string) string {
	if p.MediaType == MediaTV {
		s := fmt.Sprintf("%02d", p.Season)
		e := fmt.Sprintf("%02d", p.Episode)
		dir := filepath.Join(outputDir, p.ShowName, "Season "+s)
		episodeToken := fmt.Sprintf("S%sE%s", s, e)
		if p.EpisodeEnd > p.Episode {
			episodeToken = fmt.Sprintf("%s-E%02d", episodeToken, p.EpisodeEnd)
		}
		file := fmt.Sprintf("%s - %s.%s", p.ShowName, episodeToken, container)
		return filepath.Join(dir, file)
	}

	name := p.MovieName
	if p.Year != "" {
		name = fmt.Sprintf("%s (%s)", p.MovieName, p.Year)
	}
	return filepath.Join(outputDir, name, name+"."+container)
}
