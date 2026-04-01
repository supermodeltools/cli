// Package mcp implements a Model Context Protocol server that exposes
// Supermodel graph analysis as tools to AI coding agents (Claude Code,
// Hermes, Codex, and any other MCP-compatible host).
//
// Start with: supermodel mcp
//
// The server communicates via stdio using JSON-RPC 2.0 and implements the
// MCP specification at https://spec.modelcontextprotocol.io/
//
// Exposed tools:
//
//	analyze        — upload the current repo and run the full analysis pipeline
//	dead_code      — list functions with no callers
//	blast_radius   — list files affected by a change to a given file
//	get_graph      — return a filtered slice of the dependency/call graph
//
// This is a vertical slice. It must not import any other slice package.
// It may import from the shared kernel: internal/api, internal/cache,
// internal/config, internal/ui, internal/build.
package mcp
