package status

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/supermodeltools/cli/internal/build"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// Options configures the status command.
type Options struct {
	Output string // "human" | "json"
}

// Report holds all status information.
type Report struct {
	Version     string `json:"version"`
	Authed      bool   `json:"authenticated"`
	APIBase     string `json:"api_base"`
	ConfigPath  string `json:"config_path"`
	CacheDir    string `json:"cache_dir"`
	CacheCount  int    `json:"cached_analyses"`
}

// Run prints the current Supermodel status.
func Run(_ context.Context, opts Options) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	r := Report{
		Version:    build.Version,
		Authed:     cfg.APIKey != "",
		APIBase:    cfg.APIBase,
		ConfigPath: config.Path(),
		CacheDir:   filepath.Join(config.Dir(), "cache"),
	}
	r.CacheCount = countCacheEntries(r.CacheDir)
	return print(os.Stdout, r, ui.ParseFormat(opts.Output))
}

func countCacheEntries(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			n++
		}
	}
	return n
}

func print(w io.Writer, r Report, fmt_ ui.Format) error {
	if fmt_ == ui.FormatJSON {
		return ui.JSON(w, r)
	}

	authed := "Not authenticated — run `supermodel login`"
	if r.Authed {
		authed = "Authenticated"
	}

	ui.Table(w, []string{"KEY", "VALUE"}, [][]string{
		{"Version", r.Version},
		{"Auth", authed},
		{"Config", r.ConfigPath},
		{"API base", r.APIBase},
		{"Cache", fmt.Sprintf("%s  (%d entries)", r.CacheDir, r.CacheCount)},
	})
	return nil
}
