package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/find"
)

func init() {
	var opts find.Options

	c := &cobra.Command{
		Use:   "find <symbol>",
		Short: "Find usages and callers of a symbol across the codebase",
		Long: `Searches the graph for all nodes matching the given symbol name
(substring match, case-insensitive) and prints their call relationships.

Similar to "Find Usages" in IDEs — without requiring a language server.

Examples:
  supermodel find handleRequest          # find any symbol containing this name
  supermodel find Client --kind Class    # find classes named Client
  supermodel find --output json parse    # JSON output for tool consumption`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireAPIKey(); err != nil {
				return err
			}
			return find.Run(cmd.Context(), cfg, ".", args[0], opts)
		},
	}

	c.Flags().BoolVar(&opts.Force, "force", false, "re-analyze even if cache is fresh")
	c.Flags().StringVar(&opts.Kind, "kind", "", "filter by node label: Function, File, Class, …")
	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output format: human|json")

	rootCmd.AddCommand(c)
}
