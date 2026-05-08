# Agent Impact A/B Benchmark

This benchmark answers a different question than the compiler-only impact benchmark:

> Does impact-analysis context make a top agent repair real breakage with fewer tokens, less wall time, fewer tool calls, or a higher success rate?

The comparison must isolate one variable:

- **control:** agent receives the broken repository and task prompt.
- **impact:** agent receives the same broken repository and task prompt, plus `IMPACT_ANALYSIS.md` and `impact-analysis.json`.

Everything else is held constant: model, container image, repository commit, mutation, verifier, timeout, and prompt wording.

## Protocol

For each case:

1. Clone a pinned public repository commit.
2. Install dependencies inside Docker.
3. Run the verifier once to prove the clean checkout is green.
4. Apply a configured mutation or deletion.
5. Run the verifier again and require that it fails with at least one real source/test/type failure.
6. Run the control agent in a fresh Docker container.
7. Run the impact-context agent in another fresh Docker container.
8. Run the verifier after each agent.
9. Record success, wall time, token usage, tool calls, files changed, final diff, stdout/stderr, and raw agent JSONL.

The agent prompt forbids simply reverting the target mutation. The harness also checks that the configured `mutation.mustContain` text still exists after the agent run.

## Ten-Repository Set

The initial manifest is [agent-impact-repos.json](./agent-impact-repos.json). It uses 10 pinned public repositories:

- `tinylibs/tinyspy`
- `tinylibs/tinybench`
- `sindresorhus/p-queue`
- `sindresorhus/ky`
- `sindresorhus/p-map`
- `sindresorhus/p-retry`
- `sindresorhus/p-timeout`
- `sindresorhus/p-throttle`
- `chalk/chalk`
- `sindresorhus/yoctocolors`

The set intentionally mixes implementation TypeScript, declaration-heavy packages, test/type-test repairs, class constructors, default exports, and shared helpers.

## Build

```bash
docker build -t supermodel-agent-impact:local benchmark/agent-impact
```

## Dry Run

```bash
node benchmark/agent-impact/run-agent-impact-ab.mjs --dry-run
```

This validates the manifest shape and writes the prompts/run plan without invoking the model.

## Implementation Tests

The API ranking implementation is tested in the public API repository. From the paired public API checkout:

```bash
export SUPERMODEL_PUBLIC_API_REPO=/path/to/supermodel-public-api
cd "$SUPERMODEL_PUBLIC_API_REPO/src/data-plane"
npm test -- --runInBand \
  impact-validation-ranking-regression.test.js
```

Those tests cover scoped validation ranking behavior. The benchmark harness itself can be checked without invoking a model using the dry-run command above.

## Full Run

```bash
node benchmark/agent-impact/run-agent-impact-ab.mjs \
  --image supermodel-agent-impact:local \
  --model gpt-5.5 \
  --codex-home ~/.codex
```

The runner writes one directory per case and arm under `target/agent-impact/`.

The summary records:

- `agentModel`, expected to be `gpt-5.5` for this run
- `agentRunner`, expected to be `codex-cli 0.128.0`
- per-arm aggregate success, time, tool calls, token usage, and agent file-level F1

Each arm directory contains:

- `prompt.md`
- `agent.jsonl`
- `agent.stdout`
- `agent.stderr`
- `metrics.json`
- `final.diff`
- `verify-before.log`
- `verify-after.log`
- `impact-analysis.json` and `IMPACT_ANALYSIS.md` for the impact arm

## Metrics

Primary:

- success rate
- agent file-level F1
- wall-clock seconds
- input tokens
- output tokens
- total tokens
- tool calls

Secondary:

- verifier failure category
- files changed
- diff line count
- whether changed files overlap predicted impact files
- whether the mutation was illegally reverted

Agent file-level F1 is computed as:

```text
actual files = files implicated by the broken verifier output after mutation
agent files  = files changed by the agent after the mutation is committed as the baseline
precision    = changed files that were actually implicated / changed files
recall       = implicated files changed by the agent / implicated files
F1           = harmonic mean of precision and recall
```

This is not a substitute for verifier success. It measures whether the agent edited the right files, while verifier success measures whether the repair actually worked.

## Impact Context

For production-quality runs, `impact-analysis.json` should come from the Supermodel impact endpoint or from the same local graph implementation used by that endpoint.

The context file must not include compiler ground truth from after the mutation. It can include:

- target file and symbol
- direct callers
- transitive callers
- affected files with confidence tiers
- entry points
- risk score

It must not include:

- actual compiler errors from the mutated repository
- the verifier output
- hand-labeled files that were discovered after running the mutation

## Reading Results

The benchmark supports three conclusions:

- **Positive:** impact arm succeeds more often or uses less time/tokens/tool calls with similar quality.
- **Neutral:** impact context changes little; product should not claim agent-efficiency gains yet.
- **Negative:** impact context increases distraction, false edits, or cost.

The per-case logs matter more than the aggregate. A single false-positive-heavy impact file can make the agent inspect or edit the wrong place; those failures should be visible in `agent.jsonl`, `final.diff`, and verifier logs.

## First Full Result

The first 10-repository Codex run used small TypeScript libraries, not large application repos. The median repository was roughly 11 source/type/test files and 2k lines of code. The largest case was `sindresorhus/ky` at roughly 52 source/type/test files and 17.5k lines.

Result:

- Model: `gpt-5.5`
- Runner: `codex-cli 0.128.0`
- Control: 10/10 success, file F1 0.968, average 83.2s, average 16.3 tool calls
- Impact context: 10/10 success, file F1 0.968, average 87.6s, average 20.3 tool calls

Conclusion:

> On small repositories with compiler or `tsd` failures, the current impact-context packet does not help a frontier agent. The verifier output is already a strong map, so the control agent can run the verifier, read the named files, and repair the issue without graph guidance.

This is a negative result for this harness, not a verdict on impact analysis as a product. The next benchmark needs larger repos and harder, less compiler-directed failures where the agent has to search before it knows where to edit.

The next benchmark should use:

- larger repositories, ideally 50k-500k lines
- post-June-2024 merged PRs so the target diffs are outside this model's stated knowledge cutoff
- real PR base commits and merge commits
- hidden reference files from the PR diff
- runtime/test failures where logs do not already name every affected file
- fixed token/time/tool-call budgets to measure whether impact context helps under pressure

The first large-repo candidate slate is tracked in [post-cutoff-prs.json](./post-cutoff-prs.json), with the replay design in [post-cutoff-pr-benchmark.md](./post-cutoff-pr-benchmark.md).

## First Large Post-Cutoff PR Replays

The first large-repo replay used real merged PRs from 2026, with the PR test changes applied to the base checkout and the production fix withheld. Both arms ran in separate Docker containers with `codex-cli 0.128.0` and `gpt-5.5`.

Important caveat: the impact arm currently uses oracle reference production files as an upper-bound file-ranking packet. This proves whether a good file packet can help an agent, not whether Supermodel can generate that packet yet.

| Case | Arm | Success | File F1 | Time | Tool calls | Input tokens | Output tokens | Reasoning tokens | First reference file |
|---|---|---:|---:|---:|---:|---:|---:|---:|---|
| Terraform #38338 | control | yes | 1.000 | 302s | 80 | 2,327,265 | 11,172 | 4,303 | event 13 |
| Terraform #38338 | impact | yes | 1.000 | 202s | 52 | 2,203,134 | 6,718 | 2,718 | event 4 |
| Grafana #123935 | control | yes | 1.000 | 245s | 30 | 722,856 | 8,362 | 5,610 | event 4 |
| Grafana #123935 | impact | yes | 1.000 | 303s | 35 | 1,270,987 | 10,224 | 6,696 | event 5 |

Interpretation:

- Terraform is the positive signal we were looking for: the impact packet got the agent into the right production files earlier and reduced time, tool calls, and tokens.
- Grafana is the counterexample: the control agent found the single reference file immediately from the failing runtime test, so the context packet added no advantage and the impact arm spent more.
- File F1 is saturated at 1.000 in both cases, so the useful metric is efficiency under realistic search pressure, not final file accuracy.

The next step is to replace the oracle file packet with real Supermodel-generated impact output for these same PR replays. If Supermodel ranks the Terraform files high and avoids adding distracting Grafana context, the product claim gets stronger. If it cannot, the benchmark tells us exactly where the graph ranking needs work.

## Real Supermodel Scoped Ranking

The next run invoked the local data-plane implementation instead of the oracle packet:

```bash
export SUPERMODEL_PUBLIC_API_REPO=/path/to/supermodel-public-api
node benchmark/agent-impact/run-real-impact-ranking.mjs \
  --cases terraform-38338-import-provider-local,grafana-123935-alert-rule-pagination \
  --out-dir target/real-impact-ranking-scoped
```

Checked-in summary: [real-impact-ranking-scoped-results.md](./real-impact-ranking-scoped-results.md). Latest full local report: `target/real-impact-ranking-primary-precision/2026-05-06T19-35-21-980Z/report.md`.

The harness now also emits agent-ready scoped packets at `target/real-impact-ranking-scoped/<run>/<case>/impact-analysis.scoped.json` and `IMPACT_ANALYSIS_SCOPED.md`.

This exposed an important product distinction. `affectedFiles` is structural production impact. Regression tests should be returned separately as scoped validation context. The API now returns `validationFiles` per impact target, with score, confidence, and evidence. The precision follow-up adds `primaryValidationFiles`, a high-confidence subset for immediate inspection.

Executive result after the `validationFiles` changes on the 10-repo post-cutoff benchmark:

| Method | Precision | Recall | F1 | Correct / Expected | Predicted |
|---|---:|---:|---:|---:|---:|
| Baseline path/name matcher | 0.060 | 0.286 | 0.099 | 6 / 21 | 100 |
| Supermodel best current | 0.274 | 0.952 | 0.426 | 20 / 21 | 73 |

The Supermodel row is the best fixed strategy across all repos: scoped `validationFiles`, capped at the top 9 files. It does not use per-case oracle tuning.

Per-repo performance:

| Repo / PR | Expected | Baseline F1 | Supermodel F1 | Supermodel Correct | Supermodel Candidates |
|---|---:|---:|---:|---:|---:|
| Next.js #93417 | 4 | 0.000 | 0.615 | 4 / 4 | 9 |
| VS Code #314217 | 1 | 0.182 | 0.333 | 1 / 1 | 5 |
| MUI #48472 | 1 | 0.182 | 1.000 | 1 / 1 | 1 |
| Grafana #123935 | 1 | 0.182 | 0.200 | 1 / 1 | 9 |
| React #36047 | 1 | 0.000 | 0.200 | 1 / 1 | 9 |
| Angular #68512 | 1 | 0.000 | 0.400 | 1 / 1 | 4 |
| Prisma #29512 | 5 | 0.133 | 0.714 | 5 / 5 | 9 |
| Payload #16465 | 5 | 0.000 | 0.571 | 4 / 5 | 9 |
| Superset #39504 | 1 | 0.182 | 0.200 | 1 / 1 | 9 |
| Terraform #38338 | 1 | 0.182 | 0.200 | 1 / 1 | 9 |

Interpretation:

- The product should answer scoped questions: "if this file/function/diff changes, what should I inspect or test?"
- Aggregating every changed target in a PR can be noisy because unrelated changed functions produce their own plausible validation tests.
- The useful packet is not "impact of everything." It is target-level production impact plus target-level validation files.
- Supermodel finds 20 of 21 labeled validation files versus 6 of 21 for baseline, while returning fewer total candidates.
