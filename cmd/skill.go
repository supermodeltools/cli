package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const skillPrompt = `This repository has .graph.* files next to source files containing code relationship data from Supermodel.

The naming convention: for src/Foo.py the graph file is src/Foo.graph.py (insert .graph before the extension). Each graph file has up to three sections:
- [deps] — what this file imports and what imports it
- [calls] — function call relationships with file paths and line numbers
- [impact] — blast radius: risk level, affected domains, direct/transitive dependents

**Read the .graph file before the source file.** It shows the full dependency and call picture in far fewer tokens. Construct the path directly — don't ls the directory to discover it.

Before grepping to understand how code connects, check the relevant .graph files. They already answer most structural navigation questions: what calls what, what imports what, and what breaks if you change something. When you grep for a function name, .graph files appear in results showing every caller and callee — use this to navigate instead of searching for each one individually.`

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
