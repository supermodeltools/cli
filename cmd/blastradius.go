package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/blastradius"
	"github.com/supermodeltools/cli/internal/config"
)

func init() {
	var opts blastradius.Options

	c := &cobra.Command{
		Use:     "blast-radius <file>",
		Aliases: []string{"br"},
		Short:   "Show files affected by a change to the given file",
		Long: `Traverses the reverse import graph to find every file that directly
or transitively depends on the target file.

Useful before refactoring to understand the full impact of a change.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireAPIKey(); err != nil {
				return err
			}
			return blastradius.Run(cmd.Context(), cfg, ".", args[0], opts)
		},
	}

	c.Flags().BoolVar(&opts.Force, "force", false, "re-analyze even if a cached result exists")
	c.Flags().IntVar(&opts.Depth, "depth", 0, "max traversal depth (0 = unlimited)")
	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output format: human|json")

	rootCmd.AddCommand(c)
}
