package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/audit"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
)

func init() {
	var dir string

	c := &cobra.Command{
		Use:   "share",
		Short: "Upload your codebase health report and get a public URL",
		Long: `Runs a health audit and uploads the report to supermodeltools.com,
returning a short public URL you can share or embed as a README badge.

Example:

  supermodel share
  supermodel share --dir ./path/to/project`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runShare(cmd, dir)
		},
		SilenceUsage: true,
	}

	c.Flags().StringVar(&dir, "dir", "", "project directory (default: current working directory)")
	rootCmd.AddCommand(c)
}

func runShare(cmd *cobra.Command, dir string) error {
	rootDir, projectName, err := resolveAuditDir(dir)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Run the full audit pipeline.
	ir, err := shareAnalyze(cmd, cfg, rootDir, projectName)
	if err != nil {
		return err
	}

	report := audit.Analyze(ir, projectName)

	impact, err := runImpactForShare(cmd, cfg, rootDir)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: impact analysis unavailable: %v\n", err)
	} else {
		audit.EnrichWithImpact(report, impact)
	}

	// Render to Markdown.
	var buf bytes.Buffer
	audit.RenderHealth(&buf, report)

	// Upload and get public URL.
	client := api.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Fprintln(cmd.ErrOrStderr(), "Uploading report…")
	url, err := client.Share(ctx, api.ShareRequest{
		ProjectName: projectName,
		Status:      string(report.Status),
		Content:     buf.String(),
	})
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n  Report: %s\n\n", url)
	fmt.Fprintf(cmd.OutOrStdout(), "  Add this badge to your README:\n\n")
	fmt.Fprintf(cmd.OutOrStdout(),
		"  [![Supermodel](https://img.shields.io/badge/supermodel-%s-blueviolet)](%s)\n\n",
		report.Status, url)

	return nil
}

func shareAnalyze(cmd *cobra.Command, cfg *config.Config, rootDir, projectName string) (*api.SupermodelIR, error) {
	if err := cfg.RequireAPIKey(); err != nil {
		return nil, err
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "Creating repository archive…")
	zipPath, err := audit.CreateZip(rootDir)
	if err != nil {
		return nil, fmt.Errorf("create archive: %w", err)
	}
	defer func() { _ = os.Remove(zipPath) }()

	hash, err := cache.HashFile(zipPath)
	if err != nil {
		return nil, fmt.Errorf("hash archive: %w", err)
	}

	client := api.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Fprintf(cmd.ErrOrStderr(), "Analyzing %s…\n", projectName)
	return client.AnalyzeDomains(ctx, zipPath, "share-"+hash[:16])
}

func runImpactForShare(cmd *cobra.Command, cfg *config.Config, rootDir string) (*api.ImpactResult, error) {
	zipPath, err := audit.CreateZip(rootDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(zipPath) }()

	hash, err := cache.HashFile(zipPath)
	if err != nil {
		return nil, err
	}

	client := api.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Fprintln(cmd.ErrOrStderr(), "Running impact analysis…")
	return client.Impact(ctx, zipPath, "share-impact-"+hash[:16], "", "")
}

// resolveAuditDir and findGitRoot are defined in cmd/audit.go (same package).
