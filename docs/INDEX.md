# Documentation index

Use this page to navigate all project documentation.

---

## Start here

- **New to the project?** Read [architecture.md](architecture.md) and [project/structure.md](project/structure.md).
- **Implementing a package?** Use [design/foundation-plan.md](design/foundation-plan.md) for types, phases, and gotchas.
- **Product vision and post-MVP features?** Read [design/product-spec.md](design/product-spec.md).
- **Contributing?** See [CONTRIBUTING.md](../CONTRIBUTING.md) for setup, workflow, and conventions.

---

## Architecture

| Document | Description |
|----------|-------------|
| [**architecture.md**](architecture.md) | Package dependency diagram (Mermaid), dependency rules, per-file processing flow. |

## Design

| Document | Description |
|----------|-------------|
| [**design/core-design.md**](design/core-design.md) | Core design concepts: goals, architecture, technical choices, MVP vs deferred. |
| [**design/foundation-plan.md**](design/foundation-plan.md) | Full implementation reference: resolved decisions, scope, package layout, types, diagrams, migration phases, testing, gotchas. |
| [**design/product-spec.md**](design/product-spec.md) | Full 2.0 product specification: purpose, architecture, domain model, interfaces, error model, logging, safety, config, CLI, persistence, deployment. |

## Project

| Document | Description |
|----------|-------------|
| [**project/structure.md**](project/structure.md) | Folder layout (10 packages), dependency direction, and quick finder for common changes. |
| [**project/audit.md**](project/audit.md) | Project compliance audit: structure, documentation, Git, build, naming. |
| [**project/git-guidelines.md**](project/git-guidelines.md) | Branching strategy, commit formatting, releases, testing, and build conventions. |

## Legacy reference

| Document | Description |
|----------|-------------|
| [**legacy/Muxmaster.sh**](legacy/Muxmaster.sh) | Legacy Bash script (v1.7.0); source of behavior parity for the Go rewrite. |
| [**legacy/legacy-behavior.md**](legacy/legacy-behavior.md) | Documented current behavior of the legacy script; reference for parity. |
