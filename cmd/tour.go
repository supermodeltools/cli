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
	var dryRun bool

	c := &cobra.Command{
		Use:   "tour [path]",
		Short: "Emit a linearized reading order over the code graph",
		Long: `Generates .supermodel/TOUR.md — a single-file reading spine that walks the
repository in a strategy-chosen order (default: reverse-topological over
imports). The tour groups files by domain/subdomain and links to each file's
shard, giving agents a deterministic path through the codebase instead of N
independent shards with no order.

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

			strategy, err := resolveStrategy(strategyName)
			if err != nil {
				return err
			}

			out, err := shards.WriteTour(repoDir, cache, strategy, dryRun)
			if err != nil {
				return err
			}
			if !dryRun {
				ui.Success("Wrote tour to %s (strategy: %s)", out, strategy.Name())
			}
			return nil
		},
	}

	c.Flags().StringVar(&strategyName, "strategy", "topo",
		"linearization strategy: topo (more coming: bfs-seed, dfs-seed, centrality)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be written without touching disk")

	rootCmd.AddCommand(c)
}

func resolveStrategy(name string) (shards.TourStrategy, error) {
	switch name {
	case "", "topo":
		return shards.TopoStrategy{}, nil
	default:
		return nil, fmt.Errorf("unknown strategy %q (supported: topo)", name)
	}
}
