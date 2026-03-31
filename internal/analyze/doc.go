// Package analyze implements the `supermodel analyze` command.
// It uploads a repository ZIP to the Supermodel API, runs the full analysis
// pipeline, and caches the resulting graph locally.
//
// This is a vertical slice. It must not import any other slice package.
// It may import from the shared kernel: internal/api, internal/cache,
// internal/config, internal/ui, internal/build.
package analyze
