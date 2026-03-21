// Package naming parses media filenames into structured metadata and
// generates Jellyfin-friendly output paths. It handles TV shows, movies,
// specials (OP/ED/PV), and collision resolution.
//
// Files:
//   - parser.go:      ParseFilename — ordered regex rule matching
//   - rules.go:       ParseRule definitions — 14 regex rules with priority ordering
//   - postprocess.go: Title-casing, bracket stripping, release tag removal
//   - outputpath.go:  GetOutputPath — Jellyfin-style directory/file naming
//   - collision.go:   CollisionResolver — deduplicates output paths with -dupN suffixes
//   - harmonize.go:   HarmonizeShowName — normalizes TV show year variants across a batch
package naming
