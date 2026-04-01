// Package find implements the `supermodel find` command — code navigation
// features inspired by enterprise IDE tools (find usages, call hierarchy,
// type hierarchy).
//
// Given a symbol name, it searches the graph for all nodes matching that
// symbol and returns their usages, callers, and definitions — without
// requiring a running language server or IDE.
//
// This is a vertical slice. It must not import any other slice package.
// Closes #10 (Steal BigIron Features).
package find
