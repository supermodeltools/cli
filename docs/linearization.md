# Graph Linearization for Sharding

## Thesis

LLMs are one-dimensional. They consume a token stream and attend to positions
within it. Graphs are multi-dimensional: nodes are connected by edges that
don't live on the token axis. A model handed a blob of JSON nodes and edges has
to do pointer-chasing on UUIDs inside a single attention pass — work that scales
badly with graph size and burns context.

**Graph linearization** is the deliberate serialization of a graph into a
reading order the model can consume left-to-right, with local neighborhoods
kept close in the token stream and adjacency rendered as prose rather than
identifiers. See Xypolopoulos et al., *Graph Linearization Methods for Reasoning
on Graphs with Large Language Models* (arXiv:2410.19494) for the underlying
principles: centrality and degeneracy-based orderings substantially beat random
serialization on LLM graph-reasoning tasks.

## Where the CLI stands today

`supermodel analyze` already writes per-file sidecar shards (`.graph.ext` or
`.calls / .deps / .impact`). Those shards are **file-level linearization**:
each sidecar collapses a subgraph into a `[deps] / [calls] / [impact]` text
layout the model reads before touching the source file.

Two things are missing:

1. **No reading order across files.** Agents see N independent shards and have
   to guess which to read first. There is no spine.
2. **No prose adjacency inside a shard.** Call relationships are rendered as
   `name ← other    path:line` arrows. Accurate and terse, but the model
   reconstructs sentences on the fly every time.

Sharding produces the units. Linearization produces the **order and
narrative** over those units.

## Design: the Tour

A *tour* is a single markdown file — `.supermodel/TOUR.md` — that serializes
the whole repository graph into a linear walk. It is the spine that makes the
existing shards navigable.

```
TOUR.md                         ← linear walk (this feature)
src/auth/session.go             ← source file
src/auth/session.graph.go       ← existing shard (per-file linearization)
```

Agents read `TOUR.md` once to get the layout, then open shards + source in the
order the tour presents them.

### Structure of TOUR.md

```markdown
# Repository Tour — supermodel-cli

**Strategy:** reverse-topological over the import graph
(leaves → roots). Read top-to-bottom to see dependencies before dependents.

## Domain: Analyze
### Subdomain: Pipeline
- **internal/analyze/handler.go** — orchestrates upload + render
  reads: api, config, shards · read by: cmd/analyze.go
  risk: MEDIUM · [shard](../internal/analyze/handler.graph.go)

## Domain: Shards
### Subdomain: Rendering
- **internal/shards/render.go** — emits .graph sidecars per source file
  reads: api · read by: internal/shards/handler.go
  risk: LOW · [shard](../internal/shards/render.graph.go)
...
```

One prose line per file — name, domain, adjacency, risk, shard pointer. Linear
order is the strategy's output. The agent reads prefix-to-suffix.

### Linearization strategies

Strategies are interchangeable. The default is `topo` because it matches how
humans read codebases ("what are the leaves, then what depends on them").

| Strategy     | Ordering                                                | Best for                                |
|--------------|---------------------------------------------------------|-----------------------------------------|
| `topo`       | reverse-topological over imports (leaves first)         | whole-codebase onboarding               |
| `bfs-seed`   | BFS from `--seed <file>` outward                        | focused tasks, blast radius walks       |
| `dfs-seed`   | DFS from `--seed <file>` — depth-first exploration      | tracing a request through layers        |
| `centrality` | PageRank-like over importers (most-depended-on first)   | "what's the core of this codebase"      |

Cycles are broken by file-path lexicographic order (deterministic, boring).

### Prose narrative preamble (opt-in)

Tour generation also lets you inject a prose preamble into each existing shard
with `--narrate`:

```go
// @generated supermodel-shard — do not edit
//
// Narrative: parseConfig (Domain Config / Loading) is called by main
// (cmd/root.go:42) and serverInit (cmd/server.go:18). It calls readFile
// and json.Unmarshal. Imports: os, encoding/json. Risk: LOW.
//
// [deps]
// imports     os
// imports     encoding/json
// ...
```

The preamble is a one-paragraph summary derived from the same cache used for
the structured sections — no new data, just a second rendering targeted at the
model's native reading style. Flag-gated so users can A/B.

## CLI surface

```
supermodel tour [--strategy topo|bfs-seed|dfs-seed|centrality]
                [--seed <file>]
                [--narrate]
                [--budget-tokens <N>]
                [path]
```

- Reads `.supermodel/shards.json` (errors if absent — prompts `analyze` first).
- Writes `.supermodel/TOUR.md`.
- With `--narrate`, rewrites existing `.graph.*` shards in place to include
  the narrative preamble.
- `--budget-tokens` chunks the tour into `TOUR.01.md`, `TOUR.02.md`, ... so a
  single chapter fits in a single turn.

No API call. No new cache. Pure reshaping of what `analyze` already produced.

## Why this shape

- **Same vertical slice.** Tour lives inside `internal/shards/` — it consumes
  the shard cache and emits a companion artifact. No cross-slice dependency.
- **Additive.** Default behavior of `analyze` is unchanged. Tour is opt-in.
- **Deterministic.** Lexicographic tiebreaks, stable sort; tour file is safe to
  commit or diff.
- **Strategy-pluggable.** The `Strategy` interface is small (one method:
  `Order(cache) []string`), so we can add more orderings without touching the
  renderer.

## Open questions

- Should tour output default-render inline snippets of each shard, or strictly
  link to them? Inline is self-contained (one file to read) but duplicates
  content; linked is DRY but requires the agent to follow pointers.
- Should there be a `--focus <glob>` filter so tours scope to a subtree?
- Does `arch-docs` want to consume TOUR.md as its entry point (replacing its
  own traversal)?

## References

- Xypolopoulos et al., *Graph Linearization Methods for Reasoning on Graphs
  with Large Language Models*, arXiv:2410.19494
- `supermodeltools/codegraph-graphrag` — BFS narrative walks, the thesis doc
  in the org
- `supermodeltools/graph2md` — per-node markdown emission (another
  linearization strategy)
- `supermodeltools/mcp/src/tools/explore-function.ts` — `describeNode()`
  prose format, cross-subsystem markers
