package analyze

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Options configures the analyze command.
type Options struct {
	Force  bool   // bypass cache and re-upload
	Output string // "human" | "json"
}

// Run archives dir, uploads it to the Supermodel API, caches the result, and
// prints a summary. Uses the cache when available unless Force is set.
func Run(ctx context.Context, cfg *config.Config, dir string, opts Options) error {
	g, _, err := GetGraph(ctx, cfg, dir, opts.Force)
	if err != nil {
		return err
	}
	return printSummary(os.Stdout, g, ui.ParseFormat(opts.Output))
}

// GetGraph returns the display graph for dir, running analysis if the cache
// is cold or force is true. It returns the graph and the zip hash used as the
// cache key (useful for downstream commands).
func GetGraph(ctx context.Context, cfg *config.Config, dir string, force bool) (*api.Graph, string, error) {
	spin := ui.Start("Creating repository archive…")
	zipPath, err := createZip(dir)
	spin.Stop()
	if err != nil {
		return nil, "", fmt.Errorf("create archive: %w", err)
	}
	defer os.Remove(zipPath)

	hash, err := cache.HashFile(zipPath)
	if err != nil {
		return nil, "", err
	}

	if !force {
		if g, _ := cache.Get(hash); g != nil {
			ui.Success("Using cached analysis (repoId: %s)", g.RepoID())
			return g, hash, nil
		}
	}

	client := api.New(cfg)
	spin = ui.Start("Uploading and analyzing repository…")
	g, err := client.Analyze(ctx, zipPath, "analyze-"+hash[:16])
	spin.Stop()
	if err != nil {
		return nil, hash, err
	}

	if err := cache.Put(hash, g); err != nil {
		ui.Warn("could not write cache: %v", err)
	}

	ui.Success("Analysis complete (repoId: %s)", g.RepoID())
	return g, hash, nil
}

type summary struct {
	RepoID        string `json:"repo_id"`
	Files         int    `json:"files"`
	Functions     int    `json:"functions"`
	Relationships int    `json:"relationships"`
}

func printSummary(w io.Writer, g *api.Graph, fmt_ ui.Format) error {
	s := summary{
		RepoID:        g.RepoID(),
		Files:         len(g.NodesByLabel("File")),
		Functions:     len(g.NodesByLabel("Function")),
		Relationships: len(g.Rels()),
	}
	if fmt_ == ui.FormatJSON {
		return ui.JSON(w, s)
	}
	ui.Table(w, []string{"FIELD", "VALUE"}, [][]string{
		{"Repo ID", s.RepoID},
		{"Files", fmt.Sprintf("%d", s.Files)},
		{"Functions", fmt.Sprintf("%d", s.Functions)},
		{"Relationships", fmt.Sprintf("%d", s.Relationships)},
	})
	return nil
}
