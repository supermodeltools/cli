package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/cache"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/factory"
)

func init() {
	factoryCmd := &cobra.Command{
		Use:   "factory",
		Short: "AI-native SDLC orchestration via graph intelligence",
		Long: `Factory is an AI-native SDLC system that uses the Supermodel code graph API
to power graph-first development workflows.

Inspired by Big Iron (github.com/supermodeltools/bigiron), factory provides
three commands:

  health  — Analyse codebase health: circular deps, domain coupling, blast radius
  run     — Generate a graph-enriched 8-phase SDLC execution plan for a goal
  improve — Generate a prioritised, graph-driven improvement plan

All commands require an API key (run 'supermodel login' to configure).

Examples:

  supermodel factory health
  supermodel factory run "Add rate limiting to the order API"
  supermodel factory improve`,
		SilenceUsage: true,
	}

	// ── health ────────────────────────────────────────────────────────────────
	var healthDir string
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Alias for 'supermodel audit'",
		Long:  "Health is an alias for 'supermodel audit'. See 'supermodel audit --help' for full documentation.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAudit(cmd, healthDir)
		},
		SilenceUsage: true,
	}
	healthCmd.Flags().StringVar(&healthDir, "dir", "", "project directory (default: current working directory)")

	// ── run ───────────────────────────────────────────────────────────────────
	var runDir string
	runCmd := &cobra.Command{
		Use:   "run <goal>",
		Short: "Generate a graph-enriched SDLC execution plan for a goal",
		Long: `Run analyses the codebase and generates a graph-enriched 8-phase SDLC
execution plan tailored to the supplied goal.

The output is a Markdown prompt designed to be consumed by Claude Code or any
AI agent. Pipe it directly into an agent session:

  supermodel factory run "Add rate limiting to the order API" | claude --print

The plan includes codebase context (domains, key files, tech stack), the goal,
and phase-by-phase instructions with graph-aware quality gates.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFactoryRun(cmd, runDir, args[0])
		},
		SilenceUsage: true,
	}
	runCmd.Flags().StringVar(&runDir, "dir", "", "project directory (default: current working directory)")

	// ── improve ───────────────────────────────────────────────────────────────
	var improveDir string
	improveCmd := &cobra.Command{
		Use:   "improve",
		Short: "Generate a graph-driven improvement plan",
		Long: `Improve runs a health analysis and generates a prioritised improvement plan
using the Supermodel code graph.

The output is a Markdown prompt that guides an AI agent through:

  1. Scoring improvement targets (circular deps, coupling, dead code, depth)
  2. Executing refactors in bottom-up topological order
  3. Running quality gates after each change
  4. A final dead code sweep and health check

Pipe it into an agent session:

  supermodel factory improve | claude --print`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFactoryImprove(cmd, improveDir)
		},
		SilenceUsage: true,
	}
	improveCmd.Flags().StringVar(&improveDir, "dir", "", "project directory (default: current working directory)")

	factoryCmd.AddCommand(healthCmd, runCmd, improveCmd)
	rootCmd.AddCommand(factoryCmd)
}

// ── run ───────────────────────────────────────────────────────────────────────

func runFactoryRun(cmd *cobra.Command, dir, goal string) error {
	rootDir, projectName, err := resolveFactoryDir(dir)
	if err != nil {
		return err
	}

	ir, err := factoryAnalyze(cmd, rootDir, projectName)
	if err != nil {
		return err
	}

	report := factory.Analyze(ir, projectName)
	data := factoryPromptData(report, goal)
	factory.RenderRunPrompt(cmd.OutOrStdout(), data)
	return nil
}

// ── improve ───────────────────────────────────────────────────────────────────

func runFactoryImprove(cmd *cobra.Command, dir string) error {
	rootDir, projectName, err := resolveFactoryDir(dir)
	if err != nil {
		return err
	}

	ir, err := factoryAnalyze(cmd, rootDir, projectName)
	if err != nil {
		return err
	}

	report := factory.Analyze(ir, projectName)
	data := factoryPromptData(report, "")
	factory.RenderImprovePrompt(cmd.OutOrStdout(), data)
	return nil
}

// ── shared helpers ────────────────────────────────────────────────────────────

func resolveFactoryDir(dir string) (rootDir, projectName string, err error) {
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

func factoryAnalyze(cmd *cobra.Command, rootDir, projectName string) (*api.SupermodelIR, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("no API key configured — run 'supermodel login' first")
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "Creating repository archive…")
	zipPath, err := factory.CreateZip(rootDir)
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
	ir, err := client.AnalyzeDomains(ctx, zipPath, "factory-"+hash[:16])
	if err != nil {
		return nil, err
	}
	return ir, nil
}

func factoryPromptData(report *factory.HealthReport, goal string) *factory.SDLCPromptData {
	domains := make([]factory.DomainHealth, len(report.Domains))
	copy(domains, report.Domains)

	criticalFiles := make([]factory.CriticalFile, len(report.CriticalFiles))
	copy(criticalFiles, report.CriticalFiles)

	data := &factory.SDLCPromptData{
		ProjectName:    report.ProjectName,
		Language:       report.Language,
		TotalFiles:     report.TotalFiles,
		TotalFunctions: report.TotalFunctions,
		ExternalDeps:   report.ExternalDeps,
		Domains:        domains,
		CriticalFiles:  criticalFiles,
		CircularDeps:   report.CircularDeps,
		Goal:           goal,
		GeneratedAt:    report.AnalyzedAt.Format("2006-01-02 15:04:05 UTC"),
	}
	if goal == "" {
		data.HealthReport = report
	}
	return data
}
