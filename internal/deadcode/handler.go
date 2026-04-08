package deadcode

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/build"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Options configures the dead-code command.
type Options struct {
	Force         bool     // bypass cache
	Output        string   // "human" | "json"
	MinConfidence string   // "high" | "medium" | "low"
	Limit         int      // max candidates to return; 0 = all
	Ignore        []string // glob patterns to exclude (supports **)
	Timeout       int      // max seconds; 0 = no limit (context deadline applied by cmd layer)
}

// Run uploads the repo and runs dead code analysis via the dedicated API endpoint.
func Run(ctx context.Context, cfg *config.Config, dir string, opts *Options) error {
	// Fast-path: check cache by git fingerprint before creating the zip.
	if !opts.Force {
		if fp, err := cache.RepoFingerprint(dir); err == nil {
			key := cache.AnalysisKey(fp, "dead-code", build.Version)
			var cached api.DeadCodeResult
			if hit, _ := cache.GetJSON(key, &cached); hit {
				ui.Success("Using cached dead-code analysis")
				if len(opts.Ignore) > 0 {
					cached.DeadCodeCandidates = filterIgnored(cached.DeadCodeCandidates, opts.Ignore)
				}
				return printResults(os.Stdout, &cached, ui.ParseFormat(opts.Output))
			}
		}
	}

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
	spin = ui.Start("Analyzing dead code…")
	result, err := client.DeadCode(ctx, zipPath, "deadcode-"+hash[:16], opts.MinConfidence, opts.Limit)
	spin.Stop()
	if err != nil {
		return err
	}

	// Store result in cache for subsequent calls.
	if fp, err := cache.RepoFingerprint(dir); err == nil {
		key := cache.AnalysisKey(fp, "dead-code", build.Version)
		_ = cache.PutJSON(key, result)
	}

	if len(opts.Ignore) > 0 {
		result.DeadCodeCandidates = filterIgnored(result.DeadCodeCandidates, opts.Ignore)
	}

	return printResults(os.Stdout, result, ui.ParseFormat(opts.Output))
}

func printResults(w io.Writer, result *api.DeadCodeResult, fmt_ ui.Format) error {
	if fmt_ == ui.FormatJSON {
		return ui.JSON(w, result)
	}

	candidates := result.DeadCodeCandidates
	if len(candidates) == 0 {
		fmt.Fprintln(w, "No dead code detected.")
		return nil
	}

	rows := make([][]string, len(candidates))
	for i, c := range candidates {
		line := ""
		if c.Line > 0 {
			line = fmt.Sprintf("%d", c.Line)
		}
		rows[i] = []string{c.File, line, c.Name, c.Confidence, c.Reason}
	}
	ui.Table(w, []string{"FILE", "LINE", "FUNCTION", "CONFIDENCE", "REASON"}, rows)

	fmt.Fprintf(w, "\n%d dead code candidate(s) out of %d total declarations.\n",
		len(candidates), result.Metadata.TotalDeclarations)
	return nil
}

// filterIgnored removes candidates whose file path matches any of the given glob patterns.
func filterIgnored(candidates []api.DeadCodeCandidate, patterns []string) []api.DeadCodeCandidate {
	out := candidates[:0:0]
	for _, c := range candidates {
		ignored := false
		for _, p := range patterns {
			if matchGlob(p, c.File) {
				ignored = true
				break
			}
		}
		if !ignored {
			out = append(out, c)
		}
	}
	return out
}
