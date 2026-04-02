// Package restore implements the "supermodel restore" command: it builds a
// high-level project summary (a "context bomb") and writes it to stdout so
// that Claude Code can re-establish codebase understanding after a context
// compaction event.
//
// Graph data comes from two sources:
//   - API mode: calls /v1/graphs/supermodel and parses the full SupermodelIR
//     response into a ProjectGraph with semantic domains, critical files, and
//     external dependencies.
//   - Local mode: scans the repository file tree without any network calls,
//     grouping files by directory to produce a minimal ProjectGraph.
//
// The resulting ProjectGraph is rendered as Markdown with a configurable token
// budget (default 2 000 tokens) via Render.
package restore
