# Real Impact Ranking Scoped Results

Latest full run:

```bash
node benchmark/agent-impact/run-real-impact-ranking.mjs \
  --out-dir target/real-impact-ranking-primary-precision \
  --scope replay-dirs
```

Full local report:

```text
target/real-impact-ranking-primary-precision/2026-05-06T19-35-21-980Z/report.md
```

This invokes the local Supermodel/data-plane impact implementation. It does not use oracle PR production files except as scoring labels. The run used all 10 labeled post-cutoff PR cases in `benchmark/agent-impact/post-cutoff-prs.json`.

## Headline

The scoped validation ranking beats the naive path/name baseline on the same 10-repo benchmark.

For an executive summary, use only this comparison. "Supermodel best current" means the best fixed strategy we can use across every repo without per-case oracle tuning: scoped `validationFiles`, capped at the top 9 files.

| Method | Precision | Recall | F1 | Correct / Expected | Predicted |
|---|---:|---:|---:|---:|---:|
| Baseline path/name matcher | 0.060 | 0.286 | 0.099 | 6 / 21 | 100 |
| Supermodel best current | 0.274 | 0.952 | 0.426 | 20 / 21 | 73 |

The simple read: Supermodel found 20 of 21 labeled validation files versus 6 of 21 for baseline, while returning fewer total candidates. F1 moved from 0.099 to 0.426, a 4.3x improvement.

This is good enough to claim that scoped impact analysis can recover relevant validation files that simple proximity misses. It is still not enough to claim that the list is exact.

## Case Summary

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

## Interpretation

What works:

- Same-directory and paired-test ranking works well for MUI and Grafana.
- Root-level `test/`, `e2e/`, and `functional/` directories are now considered validation candidates, which fixes the largest false-negative class.
- Distant validation files can now rank when their path/content shares strong scoped diff terms with the changed target.
- Compound path matching catches cases like `draftMode` vs `draftmode` and `unmapped driver` paths.
- The generic-term stoplist keeps framework plumbing words such as `async`, `job`, `task`, and `config` from drowning out more specific evidence.
- `primaryValidationFiles` raises precision from 0.187 to 0.667 in the native/API shape by only emitting validation files with very strong evidence.

What fails:

- Payload still misses `test/queues/payload-types.ts`; that is a generated type file and has weak direct evidence from the production diff.
- Precision remains low on large same-domain test suites because the product intentionally returns an inspection/run queue, not a single exact answer.
- Primary precision comes with low recall. It is a high-confidence tier, not a replacement for the full validation run queue.
- Superset and Terraform still show precision regression in best-scoped mode after widening candidates; their recall was already saturated before this change.

Current conclusion:

- We can now claim Supermodel beats the naive path/name baseline on the same benchmark.
- The strongest product claim is recall: scoped impact analysis finds far-away validation files that simple proximity misses.
- We can also claim a high-confidence primary validation tier with much higher precision.
- The next engineering work should focus on reranking enough of the recall list into the primary tier without losing precision.
