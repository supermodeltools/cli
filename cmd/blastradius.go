package cmd

import (
	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/blastradius"
	"github.com/supermodeltools/cli/internal/config"
)

func init() {
	var opts blastradius.Options

	c := &cobra.Command{
		Use:     "blast-radius [file...]",
		Aliases: []string{"br", "impact"},
		Short:   "Analyze the impact of changing a file or function",
		Long: `Uploads the repository to the Supermodel API and runs impact analysis
using call graph and dependency graph reachability.

Results include risk scoring, affected files and functions, and entry
points that would be impacted by changes to the target.

Three usage modes:

  supermodel blast-radius <file>              # analyze a specific file
  supermodel blast-radius --diff changes.diff # analyze from a git diff
  supermodel blast-radius                     # global coupling map`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireAPIKey(); err != nil {
				return err
			}
			return blastradius.Run(cmd.Context(), cfg, ".", args, opts)
		},
	}

	c.Flags().BoolVar(&opts.Force, "force", false, "re-analyze even if a cached result exists")
	c.Flags().StringVar(&opts.Diff, "diff", "", "path to a unified diff file (git diff output)")
	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output format: human|json")

	rootCmd.AddCommand(c)
}
