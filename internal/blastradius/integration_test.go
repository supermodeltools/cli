//go:build integration

package blastradius_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/blastradius"
	"github.com/supermodeltools/cli/internal/testutil"
)

// TestIntegration_Run_TargetFile analyzes the minimal repo via the impact endpoint.
func TestIntegration_Run_TargetFile(t *testing.T) {
	cfg := testutil.IntegrationConfig(t)
	dir := testutil.MinimalGoDir(t)
	t.Setenv("HOME", t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := blastradius.Run(ctx, cfg, dir, []string{"main.go"}, blastradius.Options{
		Force:  true,
		Output: "human",
	})
	if err != nil {
		t.Fatalf("blastradius.Run: %v", err)
	}
}

// TestIntegration_Run_JSON verifies JSON output mode.
func TestIntegration_Run_JSON(t *testing.T) {
	cfg := testutil.IntegrationConfig(t)
	dir := testutil.MinimalGoDir(t)
	t.Setenv("HOME", t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := blastradius.Run(ctx, cfg, dir, []string{"main.go"}, blastradius.Options{
		Force:  true,
		Output: "json",
	})
	if err != nil {
		t.Fatalf("blastradius.Run JSON: %v", err)
	}
}

// TestIntegration_Run_GlobalCoupling runs with no targets for global analysis.
func TestIntegration_Run_GlobalCoupling(t *testing.T) {
	cfg := testutil.IntegrationConfig(t)
	dir := testutil.MinimalGoDir(t)
	t.Setenv("HOME", t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := blastradius.Run(ctx, cfg, dir, nil, blastradius.Options{
		Force:  true,
		Output: "human",
	})
	if err != nil {
		t.Fatalf("blastradius.Run global: %v", err)
	}
}

// TestIntegration_API_Impact calls the impact API directly and validates the response.
func TestIntegration_API_Impact(t *testing.T) {
	cfg := testutil.IntegrationConfig(t)
	zipPath := testutil.MinimalGoZip(t)
	defer os.Remove(zipPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := api.New(cfg)
	result, err := client.Impact(ctx, zipPath, "integration-test-impact", "main.go", "")
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}
	if result.Metadata.TotalFiles == 0 {
		t.Error("expected totalFiles > 0")
	}
	t.Logf("targets=%d files=%d functions=%d impacts=%d",
		result.Metadata.TargetsAnalyzed,
		result.Metadata.TotalFiles,
		result.Metadata.TotalFunctions,
		len(result.Impacts))
}
