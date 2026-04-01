package cmd

import (
	"github.com/spf13/cobra"
	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/config"
)

func init() {
	var opts analyze.Options

	c := &cobra.Command{
		Use:   "analyze [path]",
		Short: "Upload a repository and run the full analysis pipeline",
		Long: `Archives the repository, uploads it to the Supermodel API, and runs
call graph generation, dependency analysis, and domain classification.

Results are cached locally by content hash. Subsequent commands
(dead-code, blast-radius, graph) reuse the cache automatically.`,
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
			return analyze.Run(cmd.Context(), cfg, dir, opts)
		},
	}

	c.Flags().BoolVar(&opts.Force, "force", false, "re-analyze even if a cached result exists")
	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output format: human|json")

	rootCmd.AddCommand(c)
}
