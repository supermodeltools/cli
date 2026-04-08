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
	"github.com/supermodeltools/cli/internal/build"
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fp, _ := cache.RepoFingerprint(rootDir)

	// Run the full audit pipeline (shares cache with `supermodel audit`).
	ir, err := shareAnalyze(ctx, cmd, cfg, rootDir, projectName, fp)
	if err != nil {
		return err
	}

	report := audit.Analyze(ir, projectName)

	impact, err := runImpactForShare(ctx, cmd, cfg, rootDir, fp)
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
	uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer uploadCancel()

	fmt.Fprintln(cmd.ErrOrStderr(), "Uploading report…")
	url, err := client.Share(uploadCtx, api.ShareRequest{
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

func shareAnalyze(ctx context.Context, cmd *cobra.Command, cfg *config.Config, rootDir, projectName, fp string) (*api.SupermodelIR, error) {
	if err := cfg.RequireAPIKey(); err != nil {
		return nil, err
	}

	// Check fingerprint cache (shares results with `supermodel audit`).
	if fp != "" {
		key := cache.AnalysisKey(fp, "audit-domains", build.Version)
		var cached api.SupermodelIR
		if hit, _ := cache.GetJSON(key, &cached); hit {
			return &cached, nil
		}
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
	fmt.Fprintf(cmd.ErrOrStderr(), "Analyzing %s…\n", projectName)
	ir, err := client.AnalyzeDomains(ctx, zipPath, "share-"+hash[:16])
	if err != nil {
		return nil, err
	}

	if fp != "" {
		key := cache.AnalysisKey(fp, "audit-domains", build.Version)
		_ = cache.PutJSON(key, ir)
	}
	return ir, nil
}

func runImpactForShare(ctx context.Context, cmd *cobra.Command, cfg *config.Config, rootDir, fp string) (*api.ImpactResult, error) {
	// Check fingerprint cache (shares results with `supermodel audit` and `supermodel blast-radius`).
	if fp != "" {
		key := cache.AnalysisKey(fp, "impact", build.Version)
		var cached api.ImpactResult
		if hit, _ := cache.GetJSON(key, &cached); hit {
			return &cached, nil
		}
	}

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
	fmt.Fprintln(cmd.ErrOrStderr(), "Running impact analysis…")
	result, err := client.Impact(ctx, zipPath, "share-impact-"+hash[:16], "", "")
	if err != nil {
		return nil, err
	}

	if fp != "" {
		key := cache.AnalysisKey(fp, "impact", build.Version)
		_ = cache.PutJSON(key, result)
	}
	return result, nil
}

// resolveAuditDir and findGitRoot are defined in cmd/audit.go (same package).
