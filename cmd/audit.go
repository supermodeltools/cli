package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/factory"
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

The report is also used internally by 'supermodel factory run' and
'supermodel factory improve' as the Phase 8 health gate.

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

// runAudit is the shared implementation used by both 'supermodel audit' and
// 'supermodel factory health'.
func runAudit(cmd *cobra.Command, dir string) error {
	rootDir, projectName, err := resolveFactoryDir(dir)
	if err != nil {
		return err
	}

	ir, err := factoryAnalyze(cmd, rootDir, projectName)
	if err != nil {
		return err
	}

	report := factory.Analyze(ir, projectName)

	// Run impact analysis (global mode) to enrich the health report.
	impact, err := runImpactForAudit(cmd, rootDir)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: impact analysis unavailable: %v\n", err)
	} else {
		factory.EnrichWithImpact(report, impact)
	}

	factory.RenderHealth(cmd.OutOrStdout(), report)
	return nil
}

// runImpactForAudit runs global impact analysis for the audit report.
func runImpactForAudit(cmd *cobra.Command, rootDir string) (*api.ImpactResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	zipPath, err := factory.CreateZip(rootDir)
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
