// Package auth implements the `supermodel login` and `supermodel logout` commands.
// It handles GitHub OAuth, API key retrieval, and secure token storage in
// the user's config file.
//
// This is a vertical slice. It must not import any other slice package.
// It may import from the shared kernel: internal/api, internal/cache,
// internal/config, internal/ui, internal/build.
package auth
