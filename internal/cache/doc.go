// Package cache manages the local graph cache stored under ~/.supermodel/cache/.
// Graphs are keyed by the SHA-256 hash of the uploaded repository ZIP, matching
// the content-addressed scheme used by the Supermodel API.
//
// This is a shared kernel package. It must contain no business logic.
// Slice packages under internal/ may import it freely.
package cache
