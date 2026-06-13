# Muxmaster docs

All documentation for the Muxmaster project. Start with [architecture.md](architecture.md) for a high-level overview, then follow the links below.

---

## Architecture and design

| Document | Purpose |
|----------|---------|
| [architecture.md](architecture.md) | Design goals, package dependency map, per-file processing flow |
| [design/quality-system.md](design/quality-system.md) | Smart quality pipeline stages, estimation model, constants reference |
| [design/foundation-plan.md](design/foundation-plan.md) | Full implementation reference: types, phases, behavioral gotchas |
| [design/product-spec.md](design/product-spec.md) | Product-level requirements and feature spec |

## Project reference

| Document | Purpose |
|----------|---------|
| [design/structure.md](design/structure.md) | Folder layout, package table, "where to change what" quick finder |

## Research and plans

| Document | Purpose |
|----------|---------|
| [plans/anime-4k-upscaling-research.md](plans/anime-4k-upscaling-research.md) | WIP feasibility plan for opt-in anime 4K upscaling/remaster experiments |
| [plans/hevc-vaapi-quality-maximization.md](plans/hevc-vaapi-quality-maximization.md) | Draft plan: content-adaptive quality-first `hevc_vaapi` tuning (QP ceiling, per-vendor gating, denoise calibration) pending the rate–QP sweep |
