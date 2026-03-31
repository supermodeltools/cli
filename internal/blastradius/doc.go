// Package blastradius implements the `supermodel blast-radius` command.
// Given a file or function, it queries the Supermodel API call graph to
// show which other files and functions would be affected by a change.
//
// This is a vertical slice. It must not import any other slice package.
// It may import from the shared kernel: internal/api, internal/cache,
// internal/config, internal/ui, internal/build.
package blastradius
