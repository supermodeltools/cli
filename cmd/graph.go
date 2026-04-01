package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/graph"
)

func init() {
	var opts graph.Options

	c := &cobra.Command{
		Use:   "graph [path]",
		Short: "Display the repository graph",
		Long: `Fetches or loads the cached graph and renders it.

Output formats:
  human  — aligned table of nodes (default)
  json   — full graph as JSON
  dot    — Graphviz DOT for use with dot/graphviz`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireAPIKey(); err != nil {
				return err
			}
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return graph.Run(cmd.Context(), cfg, dir, opts)
		},
	}

	c.Flags().BoolVar(&opts.Force, "force", false, "re-analyze even if a cached result exists")
	c.Flags().StringVarP(&opts.Output, "output", "o", "human", "output format: human|json|dot")
	c.Flags().StringVar(&opts.Filter, "label", "", "filter nodes by label (File, Function, Class, …)")

	rootCmd.AddCommand(c)
}
