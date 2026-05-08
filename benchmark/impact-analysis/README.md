# Impact Analysis Benchmark

This benchmark is designed to falsify the claim that Supermodel impact analysis predicts useful blast radius. It compares a Supermodel-style `/v1/analysis/impact` prediction against files that actually surface new typecheck or test failures after a controlled mutation.

There are two benchmark modes:

- `run-impact-benchmark.mjs` uses an intentionally small TypeScript fixture so the ground truth is inspectable.
- `run-real-repo-impact-benchmark.mjs` clones pinned public repositories and scores graph predictions against compiler failures in real code.

## What `/v1/analysis/impact` Currently Predicts

The current data-plane implementation builds impact results in the public API repository at `src/data-plane/src/services/job-worker.service.ts` with reverse reachability:

1. Build node lookup maps from parsed graph nodes and extracted functions.
2. Build reverse call adjacency: callee -> callers.
3. Build reverse file import adjacency: imported file -> importing files.
4. Resolve the requested target file or `file:function`.
5. Seed reverse BFS from target functions.
6. Also seed from file-level importers so side-effect imports and re-export importers are included.
7. Collect affected functions/files, entry points, domains, and a risk score.

That design should favor recall over precision. File-level import seeding can include transitive importers that do not fail for a specific API-breaking mutation. This benchmark is meant to make that visible.

## Fixture

`fixtures/typescript-basic` contains:

- `src/pricing.ts` with target `calculateSubtotal`
- direct callers in `src/cart.ts` and `src/checkout.ts`
- transitive importers in `src/reports.ts` and `src/api.ts`
- `prediction.supermodel-impact.json`, a checked-in Supermodel-style prediction that lists the two direct caller files as affected

The mutation adds a required second argument to `calculateSubtotal`. TypeScript should report real call-site breakage in only the direct callers.

## Run

From the repo root:

```bash
node benchmark/impact-analysis/run-impact-benchmark.mjs
```

If this worktree does not have data-plane dependencies installed, point the harness at any local TypeScript compiler:

```bash
export SUPERMODEL_PUBLIC_API_REPO=/path/to/supermodel-public-api
TSC_BIN="$SUPERMODEL_PUBLIC_API_REPO/src/data-plane/node_modules/.bin/tsc" \
  node benchmark/impact-analysis/run-impact-benchmark.mjs
```

Generated reports are written to `target/impact-analysis-benchmark/`:

- `impact-benchmark-report.json`
- `impact-benchmark-report.md`

Use `--keep-workdir` to preserve the temporary mutated fixture directories for inspection.

### Real Repository Benchmark

From the repo root:

```bash
export SUPERMODEL_PUBLIC_API_REPO=/path/to/supermodel-public-api
node benchmark/impact-analysis/run-real-repo-impact-benchmark.mjs
```

The real-repo harness reads `real-repos.json`, fetches each pinned commit into `target/impact-analysis-real-repos/repo-cache/`, builds a local TypeScript call graph, mutates the configured target, runs the configured compiler check, and scores predicted files against files that emit new TypeScript errors.

Generated reports are written to `target/impact-analysis-real-repos/`:

- `real-repo-impact-report.json`
- `real-repo-impact-report.md`

### Scoring A Live Endpoint Result

The fixture includes a checked-in Supermodel-style prediction so the harness is runnable without a live dev stack. To score an actual `/v1/analysis/impact` response, save the completed result JSON and pass it explicitly:

```bash
node benchmark/impact-analysis/run-impact-benchmark.mjs \
  --prediction-file /path/to/live-impact-result.json
```

The override file must use the public response shape with `impacts[0].affectedFiles[]` entries.

## Metrics

- **Precision:** predicted affected files that actually surfaced new typecheck/test failures.
- **Recall:** actual broken files that were predicted.
- **F1:** harmonic mean of precision and recall.
- **Recall@K:** actual broken files found within the first K predicted files.
- **Risk calibration:** predicted risk score compared to an intentionally simple actual severity rubric based on broken file count.

The rubric is deliberately conservative:

| Actual broken files | Actual risk |
|---:|---|
| 0 | none |
| 1 | low |
| 2-4 | medium |
| 5-9 | high |
| 10+ | critical |

## How To Interpret Results

Good recall with weak precision means impact analysis may still be useful as a high-recall triage aid, but the product should not claim it knows exactly what will break.

Weak recall means impact analysis is missing real blast radius and should not be used as the core product promise.

Risk mismatch means the current scoring thresholds do not match observed breakage for the mutation shape under test.

This benchmark does not prove semantic impact. It only measures impact visible through the selected mutation, typecheck, and test commands. Add more fixtures and mutation types before using the numbers in any external claim.

## Verified Local Run

Run from a CLI benchmark branch paired with a public API checkout.

```bash
node benchmark/impact-analysis/run-impact-benchmark.mjs
```

Generated reports:

- `target/impact-analysis-benchmark/impact-benchmark-report.json`
- `target/impact-analysis-benchmark/impact-benchmark-report.md`

Observed result for `typescript-basic` after scoping function targets to reverse call edges:

| Mutation | Precision | Recall | Recall@3 | Predicted risk | Actual risk | TP | FP | FN |
|---|---:|---:|---:|---|---|---:|---:|---:|
| `add-required-currency-argument` | 1.000 | 1.000 | 1.000 | medium | medium | 2 | 0 | 0 |
| `rename-exported-function` | 1.000 | 1.000 | 1.000 | medium | medium | 2 | 0 | 0 |

Actual broken files:

- `src/cart.ts`
- `src/checkout.ts`

False positives: none.

Finding: the earlier broad prediction had high recall but weak precision because it included transitive importers (`src/reports.ts`, `src/api.ts`) that did not surface new TypeScript errors for signature-breaking mutations. Function-level targets should use reverse call edges for affected files/functions; file-level targets can still use importer traversal for module-level side effects and re-exports.

### Real Repositories

```bash
node benchmark/impact-analysis/run-real-repo-impact-benchmark.mjs
```

Generated reports:

- `target/impact-analysis-real-repos/real-repo-impact-report.json`
- `target/impact-analysis-real-repos/real-repo-impact-report.md`

Observed aggregate result across pinned public repositories:

| Cases | Micro precision | Micro recall | Micro F1 | Macro F1 | TP | FP | FN |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 2 | 0.857 | 1.000 | 0.923 | 0.929 | 6 | 1 | 0 |

Case results:

| Case | Repo | Target | Precision | Recall | F1 | TP | FP | FN |
|---|---|---|---:|---:|---:|---:|---:|---:|
| `tinyspy-define-required-parameter` | `tinylibs/tinyspy` | `src/utils.ts:define` | 0.750 | 1.000 | 0.857 | 3 | 1 | 0 |
| `tinybench-assert-required-parameter` | `tinylibs/tinybench` | `src/utils.ts:assert` | 1.000 | 1.000 | 1.000 | 3 | 0 | 0 |

The one false positive is `tinyspy/src/spy.ts`. It is a real transitive caller through `createInternalSpy`, but the required-parameter mutation only surfaces compiler errors at direct call sites. That supports splitting API output into "likely required updates" and "possible affected context" rather than treating every transitive caller as the same kind of affected file.

## Goal Prompt Used

The harness was created for this objective:

> Build a falsifiable benchmark for Supermodel impact analysis. Create a local benchmark harness that tests whether Supermodel impact analysis actually predicts useful blast radius. Prefer honest metrics over flattering output.
