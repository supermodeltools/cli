// Package ui provides output formatting primitives: progress spinners, tables,
// JSON output, and human-readable formatting helpers. It centralises all
// terminal rendering so slice packages do not import rendering libraries directly.
//
// This is a shared kernel package. It must contain no business logic.
// Slice packages under internal/ may import it freely.
package ui
