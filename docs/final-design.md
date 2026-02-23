# Final design specification

This document is the **final product specification** for Muxmaster v2.0. It describes the target product: domain model, interfaces, CLI, logging, safety, and deployment.

The full specification is maintained in:

- **[Muxmaster2_0.md](Muxmaster2_0.md)** — Complete 2.0 design document (purpose, architecture, domain model, interfaces, error model, logging, safety, configuration, CLI, persistence, deployment, extensibility, MVP scope).

Use this file as the stable entry point for “final design”; the linked document is the authoritative source.

---

## Short summary

- **Product:** Deterministic, idempotent CLI that scans media dirs, probes files, evaluates policy, and transcodes/remuxes via ffmpeg with optional verification and persistent state.
- **Architecture:** CLI → Scanner → MediaFile (Probe, Evaluate, Transcode, Verify, Persist) → State Store; interfaces for Prober, Encoder, Policy, StateStore.
- **MVP (current focus):** Parity with legacy script in Go; subcommands, config file, state store, and atomic output are post-MVP.

For full details, read [Muxmaster2_0.md](Muxmaster2_0.md).
