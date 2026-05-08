# Post-Cutoff PR Benchmark

The small-repo agent benchmark did not show value from impact context. The control agent already had enough signal from `tsc` and `tsd` verifier output, so impact context added tokens and tool calls without improving success or file F1.

The next benchmark should use large repositories and real merged PRs that landed after the model knowledge cutoff boundary.

## Current Boundary

This run treats `2024-06` as the cutoff boundary. The initial candidate set uses PRs merged in 2026, so every selected patch is well outside that boundary.

## Candidate Set

The candidate manifest is [post-cutoff-prs.json](./post-cutoff-prs.json). It currently includes:

- `vercel/next.js#93417`
- `microsoft/vscode#314217`
- `mui/material-ui#48472`
- `grafana/grafana#123935`
- `facebook/react#36047`
- `angular/angular#68512`
- `prisma/prisma#29512`
- `payloadcms/payload#16465`
- `apache/superset#39504`
- `hashicorp/terraform#38338`

These are not all equal. Some are excellent benchmark cases; some may be too expensive or too UI-specific once we try to run their focused verifiers. The manifest is a candidate slate, not a final benchmark suite.

## Replay Shape

For each PR:

1. Clone the repository at the PR base SHA.
2. Apply only the PR test files or test hunks where feasible.
3. Withhold the production fix.
4. Run a focused verifier and require failure.
5. Run the control agent.
6. Reset to the same prepared failing state.
7. Generate Supermodel impact context from the relevant target files before showing verifier output.
8. Run the impact-context agent.
9. Compare both agent diffs against the PR's production-file reference diff.

This is closer to SWE-bench than the first mutation benchmark. It uses a real post-cutoff fix as the hidden reference patch, but still keeps the arms isolated.

## Metrics

Primary:

- verifier success
- file-level F1 against reference PR production files
- time
- tool calls
- input tokens
- output tokens
- reasoning tokens
- time to first reference file touched or inspected

Secondary:

- first verifier run duration
- number of non-reference files inspected
- number of non-reference files edited
- whether the agent copied the test expectation instead of fixing behavior
- whether impact context caused a false lead

## Why This Is Better Than The Small-Repo Run

The first agent benchmark was too easy:

- small repositories
- compiler/type failures
- verifier output named the broken files
- frontier model had enough context to solve without graph help

Large post-cutoff PRs should test the actual Supermodel claim:

> Can graph context shorten the search path before an agent knows where to edit?

If the impact arm still uses more tokens and takes longer on this suite, we should stop claiming agent-efficiency gains until the impact packet is improved.

## Initial Replay Results

Two large PR replays are now runnable end to end:

- `hashicorp/terraform#38338`
- `grafana/grafana#123935`

Both used `codex-cli 0.128.0` with `gpt-5.5`. Both arms ran in separate Docker containers. The impact arm used an oracle reference-production-file packet, so this is an upper-bound file-ranking result, not a Supermodel-generated-context result yet.

| Case | Arm | Success | File F1 | Time | Tool calls | Input tokens | First reference file |
|---|---|---:|---:|---:|---:|---:|---|
| Terraform #38338 | control | yes | 1.000 | 302s | 80 | 2,327,265 | event 13 |
| Terraform #38338 | impact | yes | 1.000 | 202s | 52 | 2,203,134 | event 4 |
| Grafana #123935 | control | yes | 1.000 | 245s | 30 | 722,856 | event 4 |
| Grafana #123935 | impact | yes | 1.000 | 303s | 35 | 1,270,987 | event 5 |

The result is mixed. Terraform supports the thesis that a good file packet can reduce search cost in a large repo. Grafana shows that if the verifier and test name already lead straight to the single production file, impact context can add overhead without improving F1.

## Impact Packet Improvements To Test

The next impact arm should not receive a generic file list. It should receive an agent-ready packet:

- ranked production files
- ranked tests
- symbols and line ranges
- direct callers/importers
- transitive context separated from likely edits
- entry points
- commands to run first
- files likely safe to ignore
- confidence and reason per file

The small-repo result suggests that broad context alone is not enough. The packet has to reduce search, not merely add information.

## Real Impact Ranking Follow-Up

The oracle packet result was not enough. We then invoked the local Supermodel/data-plane impact implementation against the same two replay cases and scored its ranked output against the regression test files.

Latest run:

- checked-in summary: `benchmark/agent-impact/real-impact-ranking-scoped-results.md`
- full local reports and agent-ready packets are generated artifacts under `target/real-impact-ranking-scoped/`
- regenerate them with `node benchmark/agent-impact/run-real-impact-ranking.mjs --out-dir target/real-impact-ranking-scoped`
- scope: replay-relevant directories only, not full repositories
- output scored: new `validationFiles` field, not structural `affectedFiles`

| Case | Scoped target mode | Validation@1 | Validation@5 | Validation precision | Reference test rank |
|---|---|---:|---:|---:|---:|
| Grafana #123935 | changed file/function | 1.000 | 1.000 | 0.100 | 1 |
| Terraform #38338 | all PR targets together | 0.000 | 1.000 | 0.071 | 4 |
| Terraform #38338 | `node_resource_plan_instance.go` only | 1.000 | 1.000 | 0.100 | 1 |
| Terraform #38338 | `generateHCLResourceDef` only | 1.000 | 1.000 | 0.100 | 1 |

Learning:

- "Impact of everything in the PR" is the wrong product shape for this claim.
- The useful query is scoped: changed file/function/diff -> production impact + validation files.
- `affectedFiles` should stay structural. Tests belong in `validationFiles` with reasons and confidence.
- The next agent A/B should feed the agent this scoped packet, not the old oracle file list and not an aggregate PR-wide list.
