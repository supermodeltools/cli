package files

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/ui"
)

// ANSI helpers used only for watch summary output.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiGreen  = "\033[32m"
	ansiBGreen = "\033[1;32m"
	ansiDim    = "\033[2m"
)

// GenerateOptions configures the generate command.
type GenerateOptions struct {
	Force     bool
	DryRun    bool
	CacheFile string
}

// WatchOptions configures the watch command.
type WatchOptions struct {
	CacheFile    string
	Debounce     time.Duration
	NotifyPort   int
	FSWatch      bool
	PollInterval time.Duration
}

// RenderOptions configures the render command.
type RenderOptions struct {
	CacheFile string
	DryRun    bool
}

// Generate uploads a zip, builds the graph cache, and renders all sidecars.
func Generate(ctx context.Context, cfg *config.Config, dir string, opts GenerateOptions) error {
	repoDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	cacheFile := opts.CacheFile
	if cacheFile == "" {
		cacheFile = filepath.Join(repoDir, ".supermodel", "graph.json")
	}

	// Check for existing cache unless --force
	if !opts.Force {
		if data, err := os.ReadFile(cacheFile); err == nil {
			var ir api.SidecarIR
			if err := json.Unmarshal(data, &ir); err == nil && len(ir.Graph.Nodes) > 0 {
				ui.Success("Using cached graph (%d nodes) — use --force to re-fetch", len(ir.Graph.Nodes))
				cache := NewCache()
				cache.Build(&ir)
				files := cache.SourceFiles()
				spin := ui.Start("Rendering sidecars…")
				written, err := RenderAll(repoDir, cache, files, opts.DryRun)
				spin.Stop()
				if err != nil {
					return err
				}
				ui.Success("Wrote %d sidecars for %d source files", written, len(files))
				return updateGitignore(repoDir)
			}
		}
	}

	if fileList, listErr := DryRunList(repoDir); listErr == nil {
		stats := LanguageStats(fileList)
		PrintLanguageBarChart(stats, len(fileList))
	}

	spin := ui.Start("Creating repository archive…")
	zipPath, err := CreateZipFile(repoDir, nil)
	spin.Stop()
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer os.Remove(zipPath)

	client := api.New(cfg)
	idemKey := newUUID()

	spin = ui.Start("Uploading and analyzing repository…")
	ir, err := client.AnalyzeSidecars(ctx, zipPath, "sidecars-"+idemKey[:8])
	spin.Stop()
	if err != nil {
		return err
	}

	// Persist cache
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	cacheJSON, err := json.MarshalIndent(ir, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	tmp := cacheFile + ".tmp"
	if err := os.WriteFile(tmp, cacheJSON, 0o644); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}
	if err := os.Rename(tmp, cacheFile); err != nil {
		return fmt.Errorf("finalize cache: %w", err)
	}

	cache := NewCache()
	cache.Build(ir)
	files := cache.SourceFiles()

	spin = ui.Start("Rendering sidecars…")
	written, err := RenderAll(repoDir, cache, files, opts.DryRun)
	spin.Stop()
	if err != nil {
		return err
	}

	ui.Success("Wrote %d sidecars for %d source files (%d nodes, %d relationships)",
		written, len(files), len(ir.Graph.Nodes), len(ir.Graph.Relationships))

	return updateGitignore(repoDir)
}

// Watch runs generate on startup, then enters daemon mode.
func Watch(ctx context.Context, cfg *config.Config, dir string, opts WatchOptions) error {
	repoDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	cacheFile := opts.CacheFile
	if cacheFile == "" {
		cacheFile = filepath.Join(repoDir, ".supermodel", "graph.json")
	}

	client := api.New(cfg)

	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = 2 * time.Second
	}
	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = 3 * time.Second
	}
	notifyPort := opts.NotifyPort
	if notifyPort <= 0 {
		notifyPort = 7734
	}

	logf := func(format string, args ...interface{}) {
		fmt.Fprintf(os.Stderr, "[supermodel] "+format+"\n", args...)
	}

	daemonCfg := DaemonConfig{
		RepoDir:      repoDir,
		CacheFile:    cacheFile,
		Debounce:     debounce,
		NotifyPort:   notifyPort,
		FSWatch:      opts.FSWatch,
		PollInterval: pollInterval,
		LogFunc:      logf,
		OnReady: func(s GraphStats) {
			src := "fetched"
			if s.FromCache {
				src = "cached"
			}
			line := fmt.Sprintf("\n  %s✓%s  %s%d files%s · %s%d functions%s · %s%d relationships%s",
				ansiBGreen, ansiReset,
				ansiBold, s.SourceFiles, ansiReset,
				ansiBold, s.Functions, ansiReset,
				ansiBold, s.Relationships, ansiReset,
			)
			if s.DeadFunctionCount > 0 {
				line += fmt.Sprintf(" · %s%d uncalled%s", ansiBold, s.DeadFunctionCount, ansiReset)
			}
			line += fmt.Sprintf("  %s(%s)%s\n\n", ansiDim, src, ansiReset)
			fmt.Print(line)
		},
		OnUpdate: func(s GraphStats) {
			fmt.Printf("  %s✓%s  Updated — %s%d files%s · %s%d functions%s · %s%d relationships%s\n",
				ansiGreen, ansiReset,
				ansiBold, s.SourceFiles, ansiReset,
				ansiBold, s.Functions, ansiReset,
				ansiBold, s.Relationships, ansiReset,
			)
		},
	}

	d := NewDaemon(daemonCfg, client)
	return d.Run(ctx)
}

// Clean removes all .graph.* sidecar files from the directory tree.
func Clean(_ context.Context, _ *config.Config, dir string, dryRun bool) error {
	repoDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	var removed int
	err = filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			// Skip hidden dirs and common build dirs
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSidecarFile(info.Name()) {
			return nil
		}
		if dryRun {
			fmt.Printf("  [dry-run] would remove %s\n", path)
			removed++
			return nil
		}
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: remove %s: %v\n", path, err)
			return nil
		}
		removed++
		return nil
	})
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("Would remove %d sidecar files\n", removed)
	} else {
		fmt.Printf("Removed %d sidecar files\n", removed)
	}
	return nil
}

// postToolUseEvent is the Claude Code PostToolUse hook payload.
type postToolUseEvent struct {
	ToolName   string          `json:"tool_name"`
	ToolInput  json.RawMessage `json:"tool_input"`
	ToolResult json.RawMessage `json:"tool_result"`
}

// toolInput captures the file_path from Write/Edit tool inputs.
type toolInput struct {
	FilePath string `json:"file_path"`
	Path     string `json:"path"`
}

// Hook reads a Claude Code PostToolUse JSON event from stdin and sends a UDP
// notification to the watch daemon for any source file written or edited.
func Hook(port int) error {
	if port <= 0 {
		port = 7734
	}

	scanner := bufio.NewScanner(os.Stdin)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	raw := strings.Join(lines, "\n")

	var ev postToolUseEvent
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		// Not a valid event — silently exit (hooks must not break the agent)
		return nil
	}

	// Only handle write/edit-type tools
	name := strings.ToLower(ev.ToolName)
	if name != "write" && name != "edit" && name != "multiedit" &&
		name != "writefile" && name != "editfile" {
		return nil
	}

	var input toolInput
	if err := json.Unmarshal(ev.ToolInput, &input); err != nil {
		return nil
	}

	filePath := input.FilePath
	if filePath == "" {
		filePath = input.Path
	}
	if filePath == "" {
		return nil
	}

	// Only notify for source files, not sidecars
	ext := strings.ToLower(filepath.Ext(filePath))
	if !SourceExtensions[ext] || isSidecarPath(filePath) {
		return nil
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		// UDP dial rarely fails (connectionless), but treat errors as daemon absent.
		return nil
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = conn.Write([]byte(filePath))
	return nil
}

// Render renders sidecars from the existing cache without fetching from the API.
func Render(dir string, opts RenderOptions) error {
	repoDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	cacheFile := opts.CacheFile
	if cacheFile == "" {
		cacheFile = filepath.Join(repoDir, ".supermodel", "graph.json")
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return fmt.Errorf("reading cache %s: %w (run `supermodel analyze` first)", cacheFile, err)
	}

	var ir api.SidecarIR
	if err := json.Unmarshal(data, &ir); err != nil {
		return fmt.Errorf("parsing cache: %w", err)
	}

	cache := NewCache()
	cache.Build(&ir)
	files := cache.SourceFiles()

	written, err := RenderAll(repoDir, cache, files, opts.DryRun)
	if err != nil {
		return err
	}

	if opts.DryRun {
		fmt.Printf("Would write %d sidecars for %d source files\n", written, len(files))
	} else {
		ui.Success("Wrote %d sidecars for %d source files", written, len(files))
	}
	return nil
}

// updateGitignore ensures .supermodel/ is in the repo's .gitignore.
func updateGitignore(repoDir string) error {
	gitignorePath := filepath.Join(repoDir, ".gitignore")
	const entry = ".supermodel/"

	data, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return nil // can't read, skip silently
	}

	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == entry || strings.TrimSpace(line) == ".supermodel" {
			return nil // already present
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // .gitignore is a standard repo file; 0o600 satisfies gosec while remaining functional
	if err != nil {
		return nil // can't write, skip silently
	}
	defer f.Close()

	if content != "" && !strings.HasSuffix(content, "\n") {
		fmt.Fprintln(f)
	}
	fmt.Fprintln(f, entry)
	return nil
}
