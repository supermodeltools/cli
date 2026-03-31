// Package graph implements the `supermodel graph` command.
// It fetches or reads a cached display graph and renders or exports it
// in the requested format (human-readable, JSON, DOT, etc.).
//
// This is a vertical slice. It must not import any other slice package.
// It may import from the shared kernel: internal/api, internal/cache,
// internal/config, internal/ui, internal/build.
package graph
