package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const skillPrompt = `This repository contains Supermodel graph files next to source files that describe code relationships.

## What's in the graph files

Depending on the format, you'll find either three files or one combined file per source file:

**Three-file format** (files ending in .calls.*, .deps.*, .impact.*):
- .calls files: function call relationships — who calls what, with file path and line number
- .deps files: import/dependency relationships — what this file imports and what imports it
- .impact files: blast radius — risk level, affected domains, count of direct and transitive dependents

**Single-file format** (files ending in .graph.*):
- Contains all three sections ([deps], [calls], [impact]) in one file

## How to use them

When you need to understand a file before changing it:
1. Read its .deps (or .graph) file to see what it depends on and what depends on it
2. Read its .calls (or .graph) file to see the full call chain — who calls functions in this file and what they call
3. Read its .impact (or .graph) file to check the blast radius before making changes

When searching the codebase with grep or rg, these files appear in results alongside source code. The relationship data in them tells you where to look next without additional searches.

Example: if you grep for "calculateTotal" and see a .calls file in results, it shows you every caller and callee of that function with exact file paths and line numbers — saving you from grepping for each one individually.`

func init() {
	c := &cobra.Command{
		Use:   "skill",
		Short: "Print agent awareness prompt for graph files",
		Long: `Prints a prompt that teaches AI coding agents how to use Supermodel's
graph files. Pipe into your agent's instructions:

  supermodel skill >> CLAUDE.md
  supermodel skill >> AGENTS.md
  supermodel skill >> .cursorrules`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(skillPrompt)
		},
	}

	rootCmd.AddCommand(c)
}
