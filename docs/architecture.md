# CLI Architecture — Vertical Slice

## Guiding principle

Each user-facing feature is a **vertical slice**: a self-contained package that
owns its own command wiring, business logic, API calls, and output formatting.
Slices are allowed to share infrastructure (the kernel) but are **forbidden from
depending on each other**.

This keeps features independently changeable. Adding `blast-radius` cannot break
`dead-code`. Refactoring `auth` cannot affect `analyze`.

## Package map

```
main.go                      entry — calls cmd.Execute(), nothing else

cmd/                         wiring layer — cobra commands that delegate to slice handlers
  root.go                    root command, global flags
  version.go                 version subcommand (trivial, no handler)
  analyze.go   ─────────────────────────────────────────────┐
  deadcode.go  ──────────────────────────────────────────┐  │
  blastradius.go ────────────────────────────────────┐   │  │
  graph.go     ──────────────────────────────────┐   │   │  │
  auth.go      ──────────────────────────────┐   │   │   │  │
                                             │   │   │   │  │
internal/                                    │   │   │   │  │
  ┌──────────────── SHARED KERNEL ───────┐   │   │   │   │  │
  │ api/      HTTP client primitives     │   │   │   │   │  │
  │ config/   ~/.supermodel/config.yaml  │◄──┘   │   │   │  │
  │ cache/    local graph cache          │◄──────┘   │   │  │
  │ ui/       output, tables, spinners   │◄──────────┘   │  │
  │ build/    version/commit/date vars   │◄──────────────┘  │
  └──────────────────────────────────────┘                  │
                                                            │
  ┌──────────────── VERTICAL SLICES ─────────────────────┐  │
  │ analyze/      upload & full analysis pipeline        │◄─┘
  │ deadcode/     dead code detection                    │
  │ blastradius/  downstream impact analysis             │
  │ graph/        graph display and export               │
  │ auth/         login / logout / token storage         │
  └──────────────────────────────────────────────────────┘
```

## Rules

| From → To         | Allowed? |
|-------------------|----------|
| `main.go` → `cmd/`                     | ✅ |
| `cmd/` → any `internal/`               | ✅ (wiring) |
| `internal/<slice>` → `internal/kernel` | ✅ |
| `internal/<slice>` → `internal/<slice>`| ❌ **FORBIDDEN** |
| `internal/kernel` → `internal/<slice>` | ❌ **FORBIDDEN** |

**Shared kernel packages** (`internal/api`, `internal/build`, `internal/cache`,
`internal/config`, `internal/ui`) must contain zero business logic. They are
pure infrastructure — HTTP primitives, config loading, formatting utilities.

Any package under `internal/` that is NOT in the kernel list is treated as a
slice and subject to the cross-slice import ban.

## Adding a new feature

1. Create `internal/<feature>/` with `command.go`, `handler.go`, `types.go`.
2. Register the cobra command in `cmd/<feature>.go` by calling into the slice.
3. Do not import any other slice from the new package.
4. The architecture check in CI will reject the PR if the rule is violated.

## Adding a new kernel package

1. Add the package under `internal/<name>/`.
2. Add `"internal/<name>": true` to `sharedKernel` in
   `scripts/check-architecture/main.go`.
3. Keep it free of business logic.

## Enforcement

The `.github/workflows/architecture.yml` workflow runs on every PR that touches
`internal/`, `cmd/`, or `main.go`. It zips the repository, sends it to the
[Supermodel API](https://api.supermodeltools.com), parses the dependency graph,
and fails the build if any cross-slice `IMPORTS` relationship is found.

Run locally:

```sh
SUPERMODEL_API_KEY=<key> go run ./scripts/check-architecture
```
