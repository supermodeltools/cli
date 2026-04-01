// Package focus implements the `supermodel focus` command.
//
// Given a file or function, it extracts a compact, token-efficient
// representation of the relevant graph slice: direct imports, functions
// defined in the file, callers, and callees. The output is formatted as
// structured markdown for direct injection into LLM context windows.
//
// This addresses two issues:
//   - #11 Token Efficiency Feature — minimal graph slice instead of full dump
//   - #13 Slicing and Typing — type-aware slice extraction
//
// This is a vertical slice. It must not import any other slice package.
package focus
