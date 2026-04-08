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
	"github.com/supermodeltools/cli/internal/build"
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

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.RequireAPIKey(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Fingerprint for caching — best-effort; empty string means no caching.
	fp, _ := cache.RepoFingerprint(rootDir)

	ir, err := auditAnalyze(ctx, cmd, cfg, rootDir, projectName, fp)
	if err != nil {
		return err
	}

	report := audit.Analyze(ir, projectName)

	// Run impact analysis (global mode) to enrich the health report.
	impact, err := runImpactForAudit(ctx, cmd, cfg, rootDir, fp)
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

func auditAnalyze(ctx context.Context, cmd *cobra.Command, cfg *config.Config, rootDir, projectName, fp string) (*api.SupermodelIR, error) {
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
	ir, err := client.AnalyzeDomains(ctx, zipPath, "audit-"+hash[:16])
	if err != nil {
		return nil, err
	}

	if fp != "" {
		key := cache.AnalysisKey(fp, "audit-domains", build.Version)
		_ = cache.PutJSON(key, ir)
	}
	return ir, nil
}

// runImpactForAudit runs global impact analysis to enrich the health report.
func runImpactForAudit(ctx context.Context, cmd *cobra.Command, cfg *config.Config, rootDir, fp string) (*api.ImpactResult, error) {
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
	result, err := client.Impact(ctx, zipPath, "audit-impact-"+hash[:16], "", "")
	if err != nil {
		return nil, err
	}

	if fp != "" {
		key := cache.AnalysisKey(fp, "impact", build.Version)
		_ = cache.PutJSON(key, result)
	}
	return result, nil
}
