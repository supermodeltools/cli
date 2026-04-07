package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
		Use:   "audit",
		Short: "Analyse codebase health using graph intelligence",
		Long: `Audit analyses the codebase via the Supermodel API and produces a structured
Markdown health report covering:

  - Overall status (HEALTHY / DEGRADED / CRITICAL)
  - Circular dependency detection
  - Domain coupling metrics and high-coupling domains
  - High blast-radius files
  - Prioritised recommendations

Example:

  supermodel audit
  supermodel audit --dir ./path/to/project`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAudit(cmd, dir)
		},
		SilenceUsage: true,
	}

	c.Flags().StringVar(&dir, "dir", "", "project directory (default: current working directory)")
	rootCmd.AddCommand(c)
}

func runAudit(cmd *cobra.Command, dir string) error {
	rootDir, projectName, err := resolveAuditDir(dir)
	if err != nil {
		return err
	}

	ir, err := auditAnalyze(cmd, rootDir, projectName)
	if err != nil {
		return err
	}

	report := audit.Analyze(ir, projectName)

	// Run impact analysis (global mode) to enrich the health report.
	impact, err := runImpactForAudit(cmd, rootDir)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: impact analysis unavailable: %v\n", err)
	} else {
		audit.EnrichWithImpact(report, impact)
	}

	audit.RenderHealth(cmd.OutOrStdout(), report)
	return nil
}

func resolveAuditDir(dir string) (rootDir, projectName string, err error) {
	if dir == "" {
		dir, err = os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("get working directory: %w", err)
		}
	}
	rootDir = findGitRoot(dir)
	projectName = filepath.Base(rootDir)
	return rootDir, projectName, nil
}

func auditAnalyze(cmd *cobra.Command, rootDir, projectName string) (*api.SupermodelIR, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
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
	return client.AnalyzeDomains(ctx, zipPath, "audit-"+hash[:16])
}

// runImpactForAudit runs global impact analysis to enrich the health report.
func runImpactForAudit(cmd *cobra.Command, rootDir string) (*api.ImpactResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Fprintln(cmd.ErrOrStderr(), "Running impact analysis…")
	return client.Impact(ctx, zipPath, "audit-impact-"+hash[:16], "", "")
}
