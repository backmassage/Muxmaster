// Package naming provides filename parsing, output path building, TV show
// year-variant harmonization, and collision resolution.
//
// Planned implementation (see docs/design/foundation-plan.md §4.6):
//
// Types:
//   - ParsedName (MediaType, ShowName, Season, Episode, Year, etc.)
//   - ParseRule (ordered regex table with extract functions)
//
// Functions:
//   - ParseFilename(basename, dir) → ParsedName
//     Rule evaluation loop with specials-folder context. 15 ordered regex
//     rules: SxxExx, 1x01, OP/ED specials, anime dash, movie year, fallback.
//   - PostProcess(ParsedName) → ParsedName
//     Tag stripping, title-casing, season hints.
//   - OutputPath(ParsedName, container) → string
//     TV: ShowName/Season XX/ShowName - SXXEXX.ext
//     Movie: Name (Year)/Name.ext
//   - ResolveCollision(input, requestedPath) → string
//     In-run duplicate path resolver with owner map and counter.
//   - BuildTVYearVariantIndex(files) → index
//     HarmonizeShowName(name, yearIndex) for consistent naming.
//
// When implementing, split along these boundaries: parser.go, rules.go,
// postprocess.go, outputpath.go, collision.go, harmonize.go.
package naming
