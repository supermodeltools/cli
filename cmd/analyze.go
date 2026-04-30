package cmd

import (
	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/shards"
)

func init() {
	var opts analyze.Options
	var noShards bool
	var threeFile bool

	c := &cobra.Command{
		Use:   "analyze [path]",
		Short: "Upload a repository and run the full analysis pipeline",
		Long: `Archives the repository, uploads it to the Supermodel API, and runs
call graph generation, dependency analysis, and domain classification.

Results are cached locally by content hash. Subsequent commands
(dead-code, blast-radius, graph) reuse the cache automatically.

By default, .graph.* shard files are written next to each source file.
Use --no-shards to skip writing graph files.`,
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
			if cfg.ShardsEnabled() && !noShards {
				// Shard mode: Generate handles the full pipeline (API call +
				// cache + shards) in a single upload. Running analyze.Run
				// first would duplicate the API call.
				return shards.Generate(cmd.Context(), cfg, dir, shards.GenerateOptions{Force: opts.Force})
			}
			return analyze.Run(cmd.Context(), cfg, dir, opts)
		},
	}

	c.Flags().BoolVar(&opts.Force, "force", false, "re-analyze even if a cached result exists")
	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output format: human|json")
	c.Flags().BoolVar(&noShards, "no-shards", false, "skip writing .graph.* shard files")
	c.Flags().BoolVar(&threeFile, "three-file", false, "deprecated: standard .graph.* shards are now the only supported file mode")
	_ = c.Flags().MarkDeprecated("three-file", "standard .graph.* shards are now the only supported file mode")
	_ = c.Flags().MarkHidden("three-file")

	rootCmd.AddCommand(c)
}
