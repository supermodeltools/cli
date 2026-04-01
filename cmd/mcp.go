package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/build"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/mcp"
)

func init() {
	var repoDir string

	c := &cobra.Command{
		Use:   "mcp",
		Short: "Start the Model Context Protocol server",
		Long: `Starts a stdio MCP server that exposes Supermodel graph analysis
as tools to AI coding agents (Claude Code, Hermes, Codex, and any
other MCP-compatible host).

Add to Claude Code (~/.claude/config.json):
  {
    "mcpServers": {
      "supermodel": {
        "command": "supermodel",
        "args": ["mcp"]
      }
    }
  }

Exposed tools:
  analyze       — upload repo and run full analysis
  dead_code     — find functions with no callers
  blast_radius  — find files affected by a change
  get_graph     — return a filtered graph slice`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireAPIKey(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "supermodel mcp %s — ready\n", build.Version)
			return mcp.Serve(cmd.Context(), cfg, repoDir)
		},
	}

	c.Flags().StringVar(&repoDir, "repo", ".", "path to the repository root")

	rootCmd.AddCommand(c)
}
