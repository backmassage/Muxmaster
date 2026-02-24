# Core design concepts

This document summarizes the **core design concepts** for the Muxmaster Go rewrite. It is a short entry point; the full technical plan is in [foundation-plan.md](foundation-plan.md).

---

## Goals

- **Replace** the legacy ~2,666-line Bash script with a single static Go binary.
- **Preserve** full CLI and behavior parity with v1.7.0 for the MVP.
- **Improve** structure: typed domain data, one ffprobe call per file, unified retry logic, clear package boundaries.

## Architecture (MVP)

- **Entrypoint:** `cmd/muxmaster` — parse flags, load config, run either `--check` or the encode/remux pipeline.
- **Orchestration:** `internal/pipeline` discovers files, builds a TV year-variant index, then for each file: validate → probe → parse name → resolve output path → plan (encode/remux/skip) → execute (with retry) → update stats.
- **Key packages:** `config`, `term`, `logging`, `display`, `check`, `probe`, `naming`, `planner`, `ffmpeg`, `pipeline`. Dependencies flow downward; `probe` and `naming` stay dependency-light.

## Technical choices

- **One ffprobe call per file** — JSON with `-show_format` and `-show_streams`; all logic uses typed structs.
- **Unified retry** — Single state machine for both encode and remux (attachment → subtitle → mux queue → timestamp); up to 4 attempts per file.
- **15 naming rules** — Ordered regex-based parser for TV/movie and specials; Jellyfin-style output paths; collision resolution and TV year harmonization.
- **Quality** — Smart per-file QP/CRF with configurable bias; optional fixed override via `--quality` or `--vaapi-qp`/`--cpu-crf`.
- **No subcommands in MVP** — Single invocation `muxmaster [options] <input_dir> <output_dir>`. Subcommands and config file are post-MVP.

## Scope (MVP vs deferred)

**In scope:** Full CLI parity, sequential processing, encode (VAAPI + CPU) and remux paths, dry-run, `--check`, colored logging, log file, temp cleanup on signals.

**Deferred:** Subcommands (`scan`, `plan`, `run`, etc.), persistent state store, config file, JSON logging, atomic rename/verify, parallel workers, exit code 2 for partial failure.

---

For diagrams, type definitions, migration phases, and behavioral gotchas, see **[foundation-plan.md](foundation-plan.md)**.
