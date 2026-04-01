package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/deadcode"
)

func init() {
	var opts deadcode.Options

	c := &cobra.Command{
		Use:     "dead-code [path]",
		Aliases: []string{"dc"},
		Short:   "Find functions with no callers",
		Long: `Analyses the call graph and reports functions that are never called
from anywhere in the repository.

Exported functions, entry points (main, init), and test functions are
excluded by default because they are reachable by external callers.
Pass --include-exports to include them.`,
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
			return deadcode.Run(cmd.Context(), cfg, dir, opts)
		},
	}

	c.Flags().BoolVar(&opts.Force, "force", false, "re-analyze even if a cached result exists")
	c.Flags().BoolVar(&opts.IncludeExports, "include-exports", false, "include exported functions in results")
	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output format: human|json")

	rootCmd.AddCommand(c)
}
