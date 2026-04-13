package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const skillPrompt = `This repository has Supermodel graph shard files next to source files.

Files ending in .calls.* contain function call relationships (who calls what, with file and line number).
Files ending in .deps.* contain dependency relationships (imports and imported-by).
Files ending in .impact.* contain blast radius data (risk level, affected domains, direct/transitive dependents).

When navigating the codebase:
- Read .deps files to understand what a module depends on and what depends on it
- Read .calls files to trace call chains across files
- Read .impact files to assess the blast radius before making changes
- These files appear in grep/rg results alongside source code — use the data to navigate faster`

func init() {
	c := &cobra.Command{
		Use:   "skill",
		Short: "Print agent awareness prompt for graph shard files",
		Long: `Prints a prompt that teaches AI coding agents how to use Supermodel's
graph shard files. Pipe into your agent's instructions:

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
