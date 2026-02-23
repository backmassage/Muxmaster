# Documentation index

Use this page to navigate the design and reference docs.

---

## Design

| Document | Description |
|----------|-------------|
| [**core-design.md**](core-design.md) | Core design concepts: goals, architecture, technical choices, MVP vs deferred. Short summary with link to the full foundation plan. |
| [**final-design.md**](final-design.md) | Final product specification (v2.0): entry point that links to the full 2.0 design doc. |
| [**muxmaster-go-foundation-plan-final.md**](muxmaster-go-foundation-plan-final.md) | Full foundation plan: resolved decisions, scope, package layout, types, diagrams, migration phases, testing, gotchas. Primary reference for implementation. |
| [**Muxmaster2_0.md**](Muxmaster2_0.md) | Full 2.0 product design: purpose, architecture, domain model, interfaces, error model, logging, safety, config, CLI, persistence, deployment. |

## Project and repo

| Document | Description |
|----------|-------------|
| [**PROJECT_STRUCTURE.md**](PROJECT_STRUCTURE.md) | Folder layout, package responsibilities, dependency direction, and “where to change what” for developers. |
| [**AUDIT.md**](AUDIT.md) | Project structure audit and recommendations for clarity and maintainability. |
| [**GUIDELINES_AUDIT.md**](GUIDELINES_AUDIT.md) | Compliance audit against README, CHANGELOG, GIT_GUIDELINES, build/test, and automation. |

## Legacy and reference

| Document | Description |
|----------|-------------|
| [**legacy/Muxmaster.sh**](legacy/Muxmaster.sh) | Legacy Bash script (v1.7.0); source of behavior parity for the Go rewrite. |
| [**legacy/muxmaster-current-behavior-design.md**](legacy/muxmaster-current-behavior-design.md) | Documented current behavior of the legacy script; reference for parity. |

---

## Quick links

- **New to the project?** Start with [core-design.md](core-design.md) and [PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md).
- **Implementing a package?** Use [muxmaster-go-foundation-plan-final.md](muxmaster-go-foundation-plan-final.md) for types, phases, and gotchas.
- **Product vision and post-MVP features?** Use [final-design.md](final-design.md) and [Muxmaster2_0.md](Muxmaster2_0.md).
