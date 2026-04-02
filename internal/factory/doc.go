// Package factory implements the supermodel factory command: an AI-native
// SDLC orchestration system that uses the Supermodel code graph API to
// provide health analysis, graph-enriched execution plans, and prioritised
// improvement prompts.
//
// Three sub-commands are exposed:
//
//   - health  — analyse codebase health (circular deps, coupling, blast radius)
//   - run     — generate a graph-enriched 8-phase SDLC prompt for a given goal
//   - improve — generate a prioritised improvement plan from health data
//
// The design is inspired by the Big Iron project (github.com/supermodeltools/bigiron).
package factory
