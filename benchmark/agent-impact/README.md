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

## Result Artifacts

Keep generated result artifacts separate from the harness PR so reviewers can evaluate runner logic and benchmark evidence independently.

For real impact ranking runs, check in result artifacts under `benchmark/agent-impact/results/<run-id>/` on a results branch. A complete result artifact should include:

- reproduction instructions with the exact API/CLI branches and command
- aggregate precision, recall, and F1 tables
- generated `report.md`
- sanitized `summary.json`
- per-case scoped packets when useful for audit
