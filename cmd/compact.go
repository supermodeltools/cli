package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/compact"
)

func init() {
	var outDir string
	var dryRun bool

	c := &cobra.Command{
		Use:     "compact [path]",
		Aliases: []string{"pack", "minify"},
		Short:   "Reduce token usage of source code while preserving semantics",
		Long: `Strips comments, removes blank lines, and shortens local identifiers to
produce token-efficient source code that remains syntactically valid and
semantically identical (all tests still pass).

Supports Go, Python, TypeScript, JavaScript, and Rust.

For a single file, compacted output is written to stdout.
For a directory, files are written to --output (default: ./compacted/).

Examples:

  # compact a single file to stdout
  supermodel compact internal/api/client.go

  # compact a whole repo, write to ./compacted/
  supermodel compact .

  # dry-run: show savings without writing files
  supermodel compact --dry-run .`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			info, err := os.Stat(path)
			if err != nil {
				return err
			}

			if !info.IsDir() {
				return compactFile(cmd.OutOrStdout(), cmd.ErrOrStderr(), path, dryRun)
			}
			return compactDir(cmd.ErrOrStderr(), path, outDir, dryRun)
		},
	}

	c.Flags().StringVarP(&outDir, "output", "o", "compacted", "output directory for directory mode")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print stats without writing files")

	rootCmd.AddCommand(c)
}

func compactFile(out, errOut io.Writer, path string, dryRun bool) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lang := compact.DetectLanguage(path)
	if lang == compact.Unknown {
		return fmt.Errorf("unsupported file type: %s", path)
	}
	result, err := compact.CompactSource(src, lang)
	if err != nil {
		return fmt.Errorf("compact %s: %w", path, err)
	}

	origTok := (len(src) + 3) / 4
	compTok := (len(result) + 3) / 4
	pct := float64(len(src)-len(result)) / float64(len(src)) * 100
	fmt.Fprintf(errOut, "%s: %d → %d bytes  (%.1f%% reduction, ~%d → ~%d tokens)\n",
		path, len(src), len(result), pct, origTok, compTok)

	if !dryRun {
		_, err = out.Write(result)
	}
	return err
}

func compactDir(errOut io.Writer, dir, outDir string, dryRun bool) error {
	dest := outDir
	if dryRun {
		dest = "" // CompactDir with empty dest writes in-place; we'll just not call it
	}

	if dryRun {
		// Walk and measure without writing.
		var stats compact.Stats
		var walkErr error
		// Re-use CompactDir by pointing at a temp dir, or just inline the walk.
		// Simplest: call with a temp dir and discard.
		tmp, err := os.MkdirTemp("", "supermodel-compact-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		stats, walkErr = compact.CompactDir(dir, tmp)
		if walkErr != nil {
			return walkErr
		}
		fmt.Fprintln(errOut, stats.String())
		return nil
	}

	stats, err := compact.CompactDir(dir, dest)
	if err != nil {
		return err
	}
	fmt.Fprintf(errOut, "wrote %d files to %s\n", stats.Files, dest)
	fmt.Fprintln(errOut, stats.String())
	return nil
}
