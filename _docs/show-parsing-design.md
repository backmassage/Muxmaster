# Show Parsing and Output Mapping Design

Status: Draft  
Scope: `Muxmaster.sh` filename classification and output path mapping

## 1) Purpose

This document defines how Muxmaster classifies input files as TV vs movie, assigns show/movie names, and computes output paths. It also captures the guardrails added to prevent path collisions and misclassification in real-world anime/fansub release naming.

## 2) Problem Statement

Media libraries contain inconsistent naming styles:

- standard episodic forms (`S01E03`, `1x03`)
- anime/fansub forms (`Show - 03`, `[Group] Show 03 [Tags]`)
- specials (`OP`, `ED`, `PV`, `Menu`, `Recap`, `OVA`)
- grouped multi-era catalogs (`Berserk 1997` and `Berserk 2016/2017`)
- movie-part naming (`The Movie 1 - Part Name`)

Without deterministic parsing, output paths can collide or split one series into multiple show names.

## 3) Design Goals

1. Deterministic parsing for common naming patterns.
2. Stable output naming for TV and movie content.
3. No output-path collisions for known datasets.
4. Special content (OP/ED/PV/Menu/Recap/etc.) mapped as TV specials (`Season 00`).
5. Keep logic shell-native and easy to debug.

## 4) Non-Goals

- Perfect semantic understanding of every release name on the internet.
- Metadata API lookups (TMDB/TVDB/Anilist) during parsing.
- Renaming files in-place.

## 5) Current Pipeline

1. Discover files under input tree (supported extensions only).
2. Skip files inside any `extras` directory (case-insensitive prune).
3. Parse each filename with ordered regex rules.
4. Normalize title fields (tag stripping, bracket cleanup, title case).
5. Compute output path:
   - TV: `<out>/<Show>/Season <NN>/<Show> - S<NN>E<NN>.<container>`
   - Movie: `<out>/<Movie (Year)>/<Movie (Year)>.<container>`

## 6) Parsing Strategy (Rule Priority)

Rules are evaluated top-to-bottom. First match wins.

1. `SxxExx` episodic
2. `1x01` episodic
3. OP/ED token patterns (`S01NCED1`, `S01OP`, etc.)
4. `Episode 16.5` style episodic naming
5. Named TV specials with index (`Show OP - 01`, `Show PV - 01`, etc.)
6. Named TV specials without index (`Show - Recap`, `Show - Day Breakers`, etc.)
7. Numbered movie-part naming (`Title The Movie 1 - Part Name`)
8. Anime/fansub episodic forms (`Show - 05`, `[Group] Show 05 ...`)
9. Fallback movie with year
10. Generic movie fallback

## 7) Special Content Mapping

Special assets are intentionally grouped under the parent show as `Season 00`:

- OP -> `E101+`
- ED -> `E201+`
- PV -> `E301+`
- Special -> `E401+`
- Menu -> `E501+`
- Recap / Day Breakers / BTS / Convention -> reserved `E601+`

Reason: this prevents separate pseudo-shows like `Show OP` / `Show ED` and keeps related content discoverable.

## 8) Collision Avoidance Rules

The parser includes disambiguation to prevent identical output targets:

- Year-tagged grouped episodic releases can inherit parent year labels  
  Example: `Berserk 2016 01` under `3. Berserk (2016-2017)` -> `Berserk (2016-2017)`.
- Greedy episode parsing is corrected for names like `Show - 027 - 800 Years...`.
- Movie-part naming is kept in movie namespace (not TV episode namespace).

## 9) Normalization Rules

- Strip common release tags (`1080p`, `BluRay`, codec/audio tags, etc.).
- Remove bracketed release metadata from title fields.
- Collapse separators (`.` / `_`) into spaces for show/movie names.
- Keep parser behavior deterministic and idempotent for identical inputs.

## 10) Validation Approach

Regression validation is done with dataset-driven harnesses:

1. Parse all filenames from uploaded list files.
2. Check:
   - output path collisions
   - mixed show names inside a single source folder
   - suspicious TV/movie classification
3. Verify targeted edge-case filenames explicitly.

## 11) Known Tradeoffs

- Some specials could arguably be movies; current choice favors library grouping consistency.
- Numbered special mapping uses synthetic episode ranges and may differ from source indexing.
- Regex-first parsing is fast and transparent but less semantically rich than metadata-driven matching.

## 12) Future Improvements

1. Optional `--parse-profile` mode (`strict`, `anime`, `western`) for rule tuning.
2. Optional per-source mapping config file (`show aliases`, `forced type`, `forced season`).
3. Optional `--parse-report` debug output to emit parser rule IDs per file.
4. Unit-style parser fixture tests in CI for locked regression coverage.
