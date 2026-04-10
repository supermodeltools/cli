package cmd

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/build"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/restore"
)

func init() {
	var localMode bool
	var maxTokens int
	var dir string

	c := &cobra.Command{
		Use:   "restore",
		Short: "Generate a project context summary to restore Claude's understanding",
		Long: `Restore builds a high-level project summary (a "context bomb") and writes it
to stdout. Use it after Claude Code compacts its context window to re-establish
understanding of your codebase structure, domains, and key files.

With an API key configured (run 'supermodel login'), restore calls the
Supermodel API for an AI-powered analysis including semantic domains, external
dependencies, and critical file ranking.

Without an API key (or with --local), restore performs a local scan of the
repository file tree and produces a simpler structural summary.

Examples:

  # pipe into Claude Code (typical use)
  supermodel restore

  # use local analysis only, no API call
  supermodel restore --local

  # increase the token budget for larger projects
  supermodel restore --max-tokens 4000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestore(cmd, dir, localMode, maxTokens)
		},
		SilenceUsage: true,
	}

	c.Flags().BoolVar(&localMode, "local", false, "use local file scan instead of Supermodel API")
	c.Flags().IntVar(&maxTokens, "max-tokens", restore.DefaultMaxTokens, "maximum token budget for the output")
	c.Flags().StringVar(&dir, "dir", "", "project directory (default: current working directory)")

	rootCmd.AddCommand(c)
}

func runRestore(cmd *cobra.Command, dir string, localMode bool, maxTokens int) error {
	// Resolve the project directory.
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
	}
	rootDir := findGitRoot(dir)

	projectName := filepath.Base(rootDir)

	opts := restore.RenderOptions{
		MaxTokens: maxTokens,
		ClaudeMD:  restore.ReadClaudeMD(rootDir),
	}

	var graph *restore.ProjectGraph

	cfg, _ := config.Load()
	hasAPIKey := cfg != nil && cfg.APIKey != ""

	if !localMode && hasAPIKey {
		var err error
		graph, err = restoreViaAPI(cmd, cfg, rootDir, projectName)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: API analysis failed (%v), falling back to local mode\n", err)
		}
	}

	if graph == nil {
		opts.LocalMode = true
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		var err error
		graph, err = restore.BuildProjectGraph(ctx, rootDir, projectName)
		if err != nil {
			return fmt.Errorf("local analysis failed: %w", err)
		}
	}

	output, _, err := restore.Render(graph, projectName, opts)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), output)
	return err
}

func restoreViaAPI(cmd *cobra.Command, cfg *config.Config, rootDir, projectName string) (*restore.ProjectGraph, error) {
	// Fast-path: check fingerprint cache before uploading.
	if fp, err := cache.RepoFingerprint(rootDir); err == nil {
		key := cache.AnalysisKey(fp, "restore", build.Version)
		var cached api.SupermodelIR
		if hit, _ := cache.GetJSON(key, &cached); hit {
			return restore.FromSupermodelIR(&cached, projectName), nil
		}
	}

	zipPath, err := restoreCreateZip(rootDir)
	if err != nil {
		return nil, fmt.Errorf("create archive: %w", err)
	}
	defer func() { _ = os.Remove(zipPath) }()

	hash, err := cache.HashFile(zipPath)
	if err != nil {
		return nil, fmt.Errorf("hash archive: %w", err)
	}

	client := api.New(cfg)

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
	defer cancel()

	fmt.Fprintln(cmd.ErrOrStderr(), "Analyzing repository…")
	ir, err := client.AnalyzeDomains(ctx, zipPath, "restore-"+hash[:16])
	if err != nil {
		return nil, err
	}

	if fp, err := cache.RepoFingerprint(rootDir); err == nil {
		key := cache.AnalysisKey(fp, "restore", build.Version)
		_ = cache.PutJSON(key, ir)
	}

	graph := restore.FromSupermodelIR(ir, projectName)
	return graph, nil
}

// restoreCreateZip creates a temporary ZIP of the repository at dir.
// It tries git archive first (respects .gitignore), then falls back to a
// simple directory walk. Each vertical slice owns its own zip helper so that
// slice-specific behavior (file-size limits, skip lists) can diverge without
// coordination; see internal/analyze/zip.go for the canonical reference.
func restoreCreateZip(dir string) (string, error) {
	f, err := os.CreateTemp("", "supermodel-restore-*.zip")
	if err != nil {
		return "", err
	}
	dest := f.Name()
	f.Close()

	if restoreIsGitRepo(dir) && restoreIsWorktreeClean(dir) {
		cmd := exec.Command("git", "-C", dir, "archive", "--format=zip", "-o", dest, "HEAD")
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return dest, nil
		}
	}

	// Fallback: walk the directory.
	if err := restoreWalkZip(dir, dest); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	return dest, nil
}

func restoreIsGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func restoreIsWorktreeClean(dir string) bool {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	return err == nil && strings.TrimSpace(string(out)) == ""
}

// restoreWalkZip archives dir into a ZIP at dest, skipping common build/cache dirs.
func restoreWalkZip(dir, dest string) error {
	out, err := os.Create(dest) //nolint:gosec // dest is a temp file path from os.CreateTemp
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "__pycache__": true,
		".venv": true, "venv": true, "dist": true, "build": true, "target": true,
		".next": true, ".nuxt": true, "coverage": true, ".terraform": true,
	}

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") || info.Size() > 10<<20 {
			return nil
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		return copyFileIntoZip(path, w)
	})
}

// copyFileIntoZip opens path, copies its contents into w, then closes the file.
// Using an explicit Close (rather than defer) avoids accumulating open handles
// across all Walk iterations.
func copyFileIntoZip(path string, w io.Writer) error {
	src, err := os.Open(path) //nolint:gosec // path is from filepath.Walk within dir
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	if closeErr := src.Close(); err == nil {
		err = closeErr
	}
	return err
}

// findGitRoot walks up from start to find the directory containing .git.
// Returns start itself if no .git directory is found.
func findGitRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}
