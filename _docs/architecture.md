# Architecture

Package dependency map and design overview for the Muxmaster Go project.

---

## Goals

- **Replace** the legacy ~2,666-line Bash script with a single static Go binary.
- **Preserve** full CLI and behavior parity with v1.7.0 for the MVP.
- **Improve** structure: typed domain data, one ffprobe call per file, unified retry logic, clear package boundaries.

## Technical choices

- **One ffprobe call per file** — JSON with `-show_format` and `-show_streams`; all logic uses typed structs.
- **VAAPI hardware decode** — Full GPU pipeline (decode + encode on same device) when no CPU-only filters are needed; automatic software-decode fallback for HDR tonemapping.
- **Unified retry** — Single state machine for both encode and remux (attachment → subtitle → mux queue → timestamp); up to 4 attempts per file.
- **14 naming rules** — Ordered regex-based parser for TV/movie and specials; Jellyfin-style output paths; collision resolution and TV year harmonization.
- **Quality** — Smart per-file QP/CRF with configurable bias; optional fixed override via `--quality` or `--vaapi-qp`/`--cpu-crf`.
- **No subcommands in MVP** — Single invocation `muxmaster [options] <input_dir> <output_dir>`. Subcommands and config file are post-MVP.

## Scope (MVP vs deferred)

**In scope:** Full CLI parity, sequential processing, encode (VAAPI + CPU) and remux paths, dry-run, `--check`, colored logging, log file, temp cleanup on signals.

**Deferred:** Subcommands (`scan`, `plan`, `run`, etc.), persistent state store, config file, JSON logging, atomic rename/verify, parallel workers, exit code 2 for partial failure.

---

## Package dependency map

Dependencies flow top-down; leaf packages have zero internal dependencies.

```text
cmd (CLI entrypoint)
  -> config
  -> logging
  -> check
  -> display
  -> pipeline
  -> ffmpeg

term
  -> config

logging
  -> config
  -> term

display
  -> term

check
  -> config

planner
  -> config
  -> probe

ffmpeg
  -> config
  -> planner

pipeline (orchestrator)
  -> config
  -> probe
  -> naming
  -> planner
  -> ffmpeg
  -> display
  -> term
```

Leaf packages (no internal imports): `config`, `probe`, `naming`. Near-leaf: `term` (config), `check` (config), `display` (term).

---

## Dependency rules

- **config** depends on nothing internal.
- **term** depends only on config (reads ColorMode enum).
- **logging** depends on config + term. It calls `term.Configure` once; thereafter reads color variables at log-write time.
- **display** depends on term (for banner coloring) — **not** on logging. This decouples presentation from the logger.
- **probe** and **naming** stay dependency-free (pure logic + external tool wrappers).
- **planner** combines config + probe data to produce a `FilePlan`.
- **ffmpeg** depends on config and planner (consumes `FilePlan`).
- **pipeline** is the sole orchestrator — it wires probe, naming, planner, ffmpeg, display, and term into the per-file processing loop and analysis feature.
- **check** depends only on config; it accepts a small Logger interface.

---

## Per-file processing flow

```
pipeline.Run
  │
  ├─ 1. Validate input file (readable, not too small)
  ├─ 2. probe.Probe(path) → ProbeResult
  ├─ 3. logInputMeta (codec, resolution, bitrate, HDR/interlace flags)
  ├─ 4. naming.ParseFilename(basename, parentDir) → ParsedName
  ├─ 5. naming.GetOutputPath(ParsedName, outputDir, container) → output path
  ├─ 6. CollisionResolver.Resolve(input, output) → final path
  ├─ 7. logBitrateOutlier (internal to runner)
  ├─ 8. planner.BuildPlan(Config, ProbeResult) → FilePlan (sets HWDecode for VAAPI)
  ├─ 9. logFileStats (internal to runner)
  ├─ 10. Skip-existing check → skip if output exists
  ├─ 11. ffmpeg.Execute(FilePlan) with retry loop
  ├─ 12. Post-encode quality escalation (if output > input)
  └─ 13. Update RunStats (encoded / skipped / failed)
```

For full type definitions and behavioral detail, see [design/foundation-plan.md](design/foundation-plan.md).
