# Git guidelines

Recommendations for branching, commits, releases, testing, and building. Optimized for **solo workflow**; automation-friendly and machine-readable.

---

## Branching strategy

| Branch | Purpose |
|--------|--------|
| **`main`** | Always stable, release-ready code. Only merge when ready to tag. |
| **`dev`** | Ongoing development and feature integration. Default branch for daily work. |
| **`feature/<short-description>`** | New features. Branch from `dev`; merge back to `dev` when done. Example: `feature/vaapi-encode`. |
| **`bugfix/<short-description>`** | Bug fixes. Branch from `dev` (or `main` for hotfixes); merge back accordingly. Example: `bugfix/collision-resolver`. |
| **`hotfix/<short-description>`** | Urgent fixes against `main`. Branch from `main`; merge to both `main` and `dev`. Example: `hotfix/crash-on-empty-dir`. |

**Workflow:** Develop on `dev` or short-lived `feature/*` / `bugfix/*` branches; merge to `main` when cutting a release.

---

## Commit formatting

Use clear, concise messages in this form:

```text
<type>: <short description>
```

**Types:**

| Type | Use for |
|------|--------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only (README, CHANGELOG, design docs) |
| `chore` | Build, tooling, dependencies, no code/docs change |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `test` | Adding or updating tests |
| `ci` | CI/CD configuration changes |
| `perf` | Performance improvement |

**Examples:**

```text
feat: add basic usage guide to README.md
fix: normalize output path when input has trailing slash
docs: add GIT_GUIDELINES.md
chore: bump Go version in go.mod to 1.23
refactor: split flags into defineEncodingFlags and defineBehaviorFlags
```

Keep the description in lowercase (except names), and omit a period at the end. Optionally add a body after a blank line for more detail.

If you want commit message validation, use a local Git hook in your own environment to enforce the same pattern.

---

## Releases

- **Tag format:** `v<major>.<minor>.<patch>` (e.g. `v1.0.0`, `v2.0.0-dev`).
- **Before tagging:**
  - Update **CHANGELOG.md** with release notes under a new `[X.Y.Z]` heading and date.
  - Ensure `main` builds and (if applicable) tests pass.
- **Create the tag:** e.g. `git tag v1.0.0` (and push: `git push origin v1.0.0`).

Releases are cut from `main`. Merge from `dev` to `main` when ready, then tag.

---

## Testing and automation

- **Tests:** Add basic unit or integration tests where applicable. Place tests alongside code (e.g. `*_test.go`) or in a dedicated test layout if you adopt one.
- **Local automation:**
  - **Build/test:** Use a simple entrypoint such as `make build` and `make test`, or scripts like `./build.sh` if you prefer. Commands should be documented in README and be machine-runnable.
  - **CI/CD (optional):** For a solo project, CI is optional. If you add it, use a single script or Makefile target (e.g. `make ci`) that runs vet + build + test so agents and CI can invoke the same command. This project provides `make ci`.

All instructions and scripts should be **machine-readable** for automation (no ambiguous “run the usual checks” without a concrete command).

---

## Building

- **Keep build simple.** Example:
  ```bash
  go build -o muxmaster ./cmd
  ```
  Or via Makefile:
  ```bash
  make build
  ```
- **Document** the exact build steps (and any required env or tools) in **README.md** so humans and automation can reproduce the build.

---

## Notes for Cursor Agent

- **Machine-readable:** All instructions, branch names, commit types, and tag formats above are intended for programmatic use. Prefer the exact patterns given (e.g. `feat:`, `feature/`, `bugfix/`, `v1.0.0`).
- **Solo workflow:** These guidelines assume a single developer. Skip complex PR/review workflows unless you introduce them explicitly.
- **Efficiency:** Prefer short-lived branches, clear commit types, and one-command build/test so automation and agents can execute and verify steps without ambiguity.
