package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/shards"
	"github.com/supermodeltools/cli/internal/ui"
)

func init() {
	var strategyName string
	var seed string
	var narrate bool
	var budgetTokens int
	var dryRun bool

	c := &cobra.Command{
		Use:   "tour [path]",
		Short: "Emit a linearized reading order over the code graph",
		Long: `Generates .supermodel/TOUR.md — a single-file reading spine that walks the
repository in a strategy-chosen order, grouped by domain/subdomain, with each
entry linking to its per-file shard. This gives agents a deterministic path
through the codebase instead of N independent shards with no order.

Strategies:
  topo         reverse-topological over imports (leaves first, roots last)
  bfs-seed     breadth-first from --seed outward (focused tours)
  dfs-seed     depth-first from --seed outward
  centrality   files with the largest blast radius first

When --narrate is set, each existing .graph.* shard is rewritten with a prose
preamble describing the file's role as sentences (rather than only structured
arrows). Same data, different rendering targeted at LLM reading style.

When --budget-tokens is set and the tour exceeds the budget, TOUR.md becomes an
index linking to TOUR.01.md, TOUR.02.md, ... sized to fit one chapter per turn.

Reads .supermodel/shards.json produced by 'supermodel analyze'. No API call.
See docs/linearization.md for the design rationale.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			repoDir, err := filepath.Abs(dir)
			if err != nil {
				return fmt.Errorf("resolving path: %w", err)
			}
			cacheFile := filepath.Join(repoDir, ".supermodel", "shards.json")
			data, err := os.ReadFile(cacheFile)
			if err != nil {
				return fmt.Errorf("reading cache %s: %w (run `supermodel analyze` first)", cacheFile, err)
			}
			var ir api.ShardIR
			if err := json.Unmarshal(data, &ir); err != nil {
				return fmt.Errorf("parsing cache: %w", err)
			}
			cache := shards.NewCache()
			cache.Build(&ir)

			strategy, err := shards.ResolveStrategy(strategyName, seed)
			if err != nil {
				return err
			}

			out, err := shards.WriteTour(repoDir, cache, strategy, budgetTokens, dryRun)
			if err != nil {
				return err
			}
			if !dryRun {
				ui.Success("Wrote tour to %s (strategy: %s)", out, strategy.Name())
			}

			if narrate {
				files := cache.SourceFiles()
				written, rerr := shards.RenderAll(repoDir, cache, files, true, dryRun)
				if rerr != nil {
					return fmt.Errorf("re-rendering shards with narrative: %w", rerr)
				}
				if !dryRun {
					ui.Success("Re-wrote %d shards with narrative preamble", written)
				}
			}
			return nil
		},
	}

	c.Flags().StringVar(&strategyName, "strategy", "topo",
		"linearization strategy: topo | bfs-seed | dfs-seed | centrality")
	c.Flags().StringVar(&seed, "seed", "", "seed file path (required for bfs-seed / dfs-seed)")
	c.Flags().BoolVar(&narrate, "narrate", false, "also rewrite existing .graph.* shards with a prose narrative preamble")
	c.Flags().IntVar(&budgetTokens, "budget-tokens", 0, "chunk tour into chapters of this token budget (0 = single file)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be written without touching disk")

	rootCmd.AddCommand(c)
}
