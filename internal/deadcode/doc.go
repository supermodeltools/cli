// Package deadcode implements the `supermodel dead-code` command.
// It calls the Supermodel API to identify functions and files with no callers,
// then renders the results as a table or JSON.
//
// This is a vertical slice. It must not import any other slice package.
// It may import from the shared kernel: internal/api, internal/cache,
// internal/config, internal/ui, internal/build.
package deadcode
