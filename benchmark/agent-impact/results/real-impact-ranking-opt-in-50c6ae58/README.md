# Real Impact Ranking Result: API 50c6ae58

This result records the 10-repository scoped impact-ranking run against the public API implementation at `50c6ae58e408fabcd8a4b36f960b0988f5804432`.

It is intentionally separate from the benchmark harness PR. The harness lives in `benchmark/agent-impact`; this directory is the evidence bundle for one run.

## Run Identity

- Generated: `2026-05-08T20:09:31.845Z`
- CLI branch: `impact-analysis-benchmark`
- CLI commit used for this artifact branch base: `3738a6381c156f3aef19261a88b38337653296ab`
- API branch: `impact-analysis-api-fixes`
- API commit: `50c6ae58e408fabcd8a4b36f960b0988f5804432`
- Node: `v24.3.0`
- npm: `11.4.2`
- Manifest: `benchmark/agent-impact/post-cutoff-prs.json`
- Scope: `replay-dirs`
- Cases: 10

The ranking runner imports the local public API data-plane implementation directly through `SUPERMODEL_PUBLIC_API_REPO`. It does not call the hosted API. Public endpoint compatibility is covered by the API PR tests and smoke scripts.

## Reproduce

Check out the API branch and install data-plane dependencies:

```bash
git clone git@github.com:supermodeltools/supermodel-public-api.git
cd supermodel-public-api
git checkout impact-analysis-api-fixes
git rev-parse HEAD

cd src/data-plane
npm install
```

Check out the CLI benchmark harness:

```bash
git clone git@github.com:supermodeltools/cli.git
cd cli
git checkout impact-analysis-benchmark
git rev-parse HEAD
```

Run the benchmark:

```bash
export SUPERMODEL_PUBLIC_API_REPO=/absolute/path/to/supermodel-public-api

node benchmark/agent-impact/run-real-impact-ranking.mjs \
  --out-dir target/real-impact-ranking-opt-in-50c6ae58 \
  --scope replay-dirs
```

The runner clones the pinned public repositories from `post-cutoff-prs.json`, checks out each PR base SHA, applies the PR regression files from the merge commit, withholds the production fix, builds a scoped graph, and scores the generated `validationFiles` against the hidden PR regression-file labels.

The full local output includes cloned repositories under `target/`, which is intentionally not checked in. This artifact keeps only:

- `report.md`
- sanitized `summary.json`
- per-case `IMPACT_ANALYSIS_SCOPED.md`
- per-case `impact-analysis.scoped.json`
- per-case `reference-production.diff`

## Headline Result

The headline comparison uses the same 10 post-cutoff PR cases and the same 21 labeled regression files.

| Method | Precision | Recall | F1 | Correct / Expected | Predicted |
|---|---:|---:|---:|---:|---:|
| Baseline path/name matcher | 0.060 | 0.286 | 0.099 | 6 / 21 | 100 |
| Supermodel best scoped packet | 0.274 | 0.952 | 0.426 | 20 / 21 | 73 |

Interpretation: Supermodel recovered 20 of 21 labeled validation files versus 6 of 21 for the naive path/name baseline, while returning fewer candidates. The strongest claim here is recall: scoped impact analysis finds relevant validation files that simple proximity misses. Precision still needs work.

## Per-Repo Summary

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

## Notes

The benchmark is scoped by design. It asks: "if this production file or function changes, which validation files should be inspected or run first?" It is not trying to return the impact of every file in the repository.

The checked-in `summary.json` has local absolute paths replaced with `<cli-repo>` and `<public-api-repo>`. The raw cloned repositories and local caches are excluded from the artifact.
