package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/shards"
)

func init() {
	var opts analyze.Options
	var noShards bool
	var threeFile bool
	var narrate bool
	var tour bool
	var tourStrategy string
	var tourSeed string
	var tourBudget int

	c := &cobra.Command{
		Use:   "analyze [path]",
		Short: "Upload a repository and run the full analysis pipeline",
		Long: `Archives the repository, uploads it to the Supermodel API, and runs
call graph generation, dependency analysis, and domain classification.

Results are cached locally by content hash. Subsequent commands
(dead-code, blast-radius, graph) reuse the cache automatically.

By default, .graph.* shard files are written next to each source file.
Use --no-shards to skip writing graph files.

Linearization flags:
  --narrate          prefix each shard with a prose narrative preamble
  --tour             also emit .supermodel/TOUR.md (the reading spine)
  --tour-strategy    topo | bfs-seed | dfs-seed | centrality (default: topo)
  --tour-seed        seed file for bfs-seed/dfs-seed
  --tour-budget      chunk tour into chapters of this token budget

See docs/linearization.md for design.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireAPIKey(); err != nil {
				return err
			}
			if noShards && threeFile {
				return fmt.Errorf("--three-file cannot be used with --no-shards")
			}
			if noShards && (narrate || tour) {
				return fmt.Errorf("--narrate and --tour require shards (cannot combine with --no-shards)")
			}
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			if cfg.ShardsEnabled() && !noShards {
				// Shard mode: Generate handles the full pipeline (API call +
				// cache + shards) in a single upload. Running analyze.Run
				// first would duplicate the API call.
				return shards.Generate(cmd.Context(), cfg, dir, shards.GenerateOptions{
					Force:        opts.Force,
					ThreeFile:    threeFile,
					Narrate:      narrate,
					Tour:         tour,
					TourStrategy: tourStrategy,
					TourSeed:     tourSeed,
					TourBudget:   tourBudget,
				})
			}
			return analyze.Run(cmd.Context(), cfg, dir, opts)
		},
	}

	c.Flags().BoolVar(&opts.Force, "force", false, "re-analyze even if a cached result exists")
	c.Flags().StringVarP(&opts.Output, "output", "o", "", "output format: human|json")
	c.Flags().BoolVar(&noShards, "no-shards", false, "skip writing .graph.* shard files")
	c.Flags().BoolVar(&threeFile, "three-file", false, "generate .calls/.deps/.impact files instead of single .graph")
	c.Flags().BoolVar(&narrate, "narrate", false, "prefix each shard with a prose narrative preamble")
	c.Flags().BoolVar(&tour, "tour", false, "also emit .supermodel/TOUR.md — the linear reading spine")
	c.Flags().StringVar(&tourStrategy, "tour-strategy", "topo", "tour ordering: topo | bfs-seed | dfs-seed | centrality")
	c.Flags().StringVar(&tourSeed, "tour-seed", "", "seed file for bfs-seed / dfs-seed strategies")
	c.Flags().IntVar(&tourBudget, "tour-budget", 0, "chunk tour into chapters of this token budget (0 = single file)")

	rootCmd.AddCommand(c)
}
