package blastradius

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Options configures the blast-radius command.
type Options struct {
	Force  bool   // bypass cache
	Output string // "human" | "json"
	Diff   string // path to a unified diff file (optional)
}

// Run uploads the repo and runs impact analysis via the dedicated API endpoint.
func Run(ctx context.Context, cfg *config.Config, dir string, targets []string, opts Options) error {
	spin := ui.Start("Creating repository archive…")
	zipPath, err := createZip(dir)
	spin.Stop()
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer os.Remove(zipPath)

	hash, err := cache.HashFile(zipPath)
	if err != nil {
		return err
	}

	client := api.New(cfg)
	spin = ui.Start("Analyzing impact…")
	result, err := client.Impact(ctx, zipPath, "impact-"+hash[:16], strings.Join(targets, ","), opts.Diff)
	spin.Stop()
	if err != nil {
		return err
	}

	return printResults(os.Stdout, result, ui.ParseFormat(opts.Output))
}

func printResults(w io.Writer, result *api.ImpactResult, fmt_ ui.Format) error {
	if fmt_ == ui.FormatJSON {
		return ui.JSON(w, result)
	}

	if len(result.Impacts) == 0 {
		// Global coupling map mode.
		if len(result.GlobalMetrics.MostCriticalFiles) > 0 {
			fmt.Fprintln(w, "Most critical files (by dependent count):")
			rows := make([][]string, len(result.GlobalMetrics.MostCriticalFiles))
			for i, f := range result.GlobalMetrics.MostCriticalFiles {
				rows[i] = []string{f.File, fmt.Sprintf("%d", f.DependentCount)}
			}
			ui.Table(w, []string{"FILE", "DEPENDENTS"}, rows)
			return nil
		}
		fmt.Fprintln(w, "No impact detected.")
		return nil
	}

	for i := range result.Impacts {
		impact := &result.Impacts[i]
		br := &impact.BlastRadius
		fmt.Fprintf(w, "Target: %s", impact.Target.File)
		if impact.Target.Name != "" {
			fmt.Fprintf(w, ":%s", impact.Target.Name)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Risk: %s  |  Direct: %d  |  Transitive: %d  |  Files: %d\n",
			br.RiskScore, br.DirectDependents, br.TransitiveDependents, br.AffectedFiles)

		if len(br.RiskFactors) > 0 {
			for _, rf := range br.RiskFactors {
				fmt.Fprintf(w, "  → %s\n", rf)
			}
		}

		if len(impact.AffectedFiles) > 0 {
			fmt.Fprintln(w)
			rows := make([][]string, len(impact.AffectedFiles))
			for i, f := range impact.AffectedFiles {
				rows[i] = []string{f.File, fmt.Sprintf("%d", f.DirectDependencies), fmt.Sprintf("%d", f.TransitiveDependencies)}
			}
			ui.Table(w, []string{"AFFECTED FILE", "DIRECT", "TRANSITIVE"}, rows)
		}

		if len(impact.EntryPointsAffected) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Entry points affected:")
			rows := make([][]string, len(impact.EntryPointsAffected))
			for i, ep := range impact.EntryPointsAffected {
				rows[i] = []string{ep.File, ep.Name, ep.Type}
			}
			ui.Table(w, []string{"FILE", "NAME", "TYPE"}, rows)
		}

		fmt.Fprintln(w)
	}

	meta := result.Metadata
	fmt.Fprintf(w, "%d target(s) analyzed across %d files and %d functions.\n",
		meta.TargetsAnalyzed, meta.TotalFiles, meta.TotalFunctions)
	return nil
}
