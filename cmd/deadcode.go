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
		Short:   "Find unreachable functions using static analysis",
		Long: `Uploads the repository to the Supermodel API and runs multi-phase dead
code analysis including call graph reachability, entry point detection,
and transitive propagation.

Results include confidence levels (high/medium/low), line numbers, and
explanations for why each function was flagged.`,
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
	c.Flags().StringVar(&opts.MinConfidence, "min-confidence", "", "minimum confidence: high, medium, or low")
	c.Flags().IntVar(&opts.Limit, "limit", 0, "maximum number of candidates to return")
	c.Flags().StringArrayVar(&opts.Ignore, "ignore", nil, "glob pattern to exclude from results (repeatable, supports **)")
	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output format: human|json")

	rootCmd.AddCommand(c)
}
