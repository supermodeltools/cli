package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/supermodeltools/cli/internal/analyze"
	"github.com/supermodeltools/cli/internal/config"
)

// ANSI color codes
const (
	reset  = "\033[0m"
	dim    = "\033[2m"
	cyan   = "\033[36m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	bCyan  = "\033[1;36m"
	bGreen = "\033[1;32m"
	bWhite = "\033[1;97m"
	dWhite = "\033[2;37m"
)

// Run executes the setup wizard.
func Run(ctx context.Context, cfg *config.Config) error {
	// ── Header ──────────────────────────────────────────────────────
	fmt.Println()
	fmt.Printf("  %sSupermodel%s  %ssetup%s\n", bCyan, reset, dim, reset)
	fmt.Println()
	fmt.Printf("  %sMake your coding agents %s3× faster%s, %s50%%+ cheaper%s, and %smore accurate%s%s.\n",
		reset, bWhite, reset, bWhite, reset, bWhite, reset, reset)
	fmt.Println()
	fmt.Printf("  %sInjects a live code graph next to your source files so agents pick it%s\n", dWhite, reset)
	fmt.Printf("  %sup automatically through their native grep, cat, and rg calls — no%s\n", dWhite, reset)
	fmt.Printf("  %sprompt engineering, no extra context windows, no new tools to learn.%s\n", dWhite, reset)
	fmt.Println()

	// ── Step 1: Authentication ──────────────────────────────────────
	fmt.Printf("  %s◆%s  Authentication\n", cyan, reset)
	fmt.Println()

	if cfg.APIKey == "" {
		fmt.Printf("  %sRun 'supermodel login' first, then re-run 'supermodel setup'.%s\n\n", yellow, reset)
		return nil
	}
	fmt.Printf("     %sUsing key%s %s%s%s\n", dim, reset, bWhite, maskKey(cfg.APIKey), reset)
	fmt.Printf("  %s✓%s  Authentication\n", green, reset)
	fmt.Println()

	// ── Step 2: Repository ─────────────────────────────────────────
	fmt.Printf("  %s◆%s  Repository\n", cyan, reset)
	fmt.Println()

	repoDir := findGitRoot()
	if repoDir != "" {
		fmt.Printf("     %sDetected:%s %s\n", dim, reset, repoDir)
		fmt.Println()
		if !confirmYN("Use this directory?", true) {
			repoDir = promptText("Path to repository", "")
		}
	} else {
		repoDir = promptText("Path to repository", ".")
	}
	repoDir, _ = filepath.Abs(repoDir)
	fmt.Printf("  %s✓%s  Repository\n", green, reset)
	fmt.Println()

	// ── Step 3: File mode ──────────────────────────────────────────
	fmt.Printf("  %s◆%s  File mode\n", cyan, reset)
	fmt.Println()
	fmt.Printf("  %sFile mode writes a .graph file next to each source file in your repo.%s\n", dWhite, reset)
	fmt.Printf("  %sAI agents pick these up automatically through grep, cat, and rg — no%s\n", dWhite, reset)
	fmt.Printf("  %sprompt engineering, no extra context windows, no new tools to learn.%s\n", dWhite, reset)
	fmt.Println()
	fmt.Printf("  %sKeep files updated with 'supermodel watch' in the background, or run%s\n", dWhite, reset)
	fmt.Printf("  %s'supermodel analyze' once to generate them on demand.%s\n", dWhite, reset)
	fmt.Println()
	fmt.Printf("  %sDisable at any time with: supermodel clean%s\n", dWhite, reset)
	fmt.Println()

	filesEnabled := confirmYN("Enable file mode?", true)
	fmt.Println()

	// Persist file mode setting
	cfg.Files = boolPtr(filesEnabled)
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "  %sWarning: could not save config: %v%s\n", yellow, err, reset)
	}

	if filesEnabled {
		fmt.Printf("  %s✓%s  File mode enabled\n", green, reset)
	} else {
		fmt.Printf("  %s✓%s  File mode disabled\n", green, reset)
	}
	fmt.Println()

	// ── Step 4: Claude Code hook (only if file mode enabled) ───────
	hookInstalled := false
	hookNote := "not installed"

	if filesEnabled {
		fmt.Printf("  %s◆%s  Claude Code hook\n", cyan, reset)
		fmt.Println()

		switch detectClaude() {
		case true:
			fmt.Printf("  %sInstalling a PostToolUse hook keeps your .graph files updated every%s\n", dWhite, reset)
			fmt.Printf("  %stime Claude Code writes or edits a file — no manual re-runs needed.%s\n", dWhite, reset)
			fmt.Println()

			if confirmYN("Install Claude Code hook?", true) {
				installed, err := installHook(repoDir)
				switch {
				case err != nil:
					fmt.Fprintf(os.Stderr, "  %sWarning: could not install hook: %v%s\n", yellow, err, reset)
				case installed:
					hookInstalled = true
					hookNote = "installed in .claude/settings.json"
					fmt.Printf("  %s✓%s  Hook installed\n", green, reset)
				default:
					fmt.Printf("  %s✓%s  Hook already installed\n", green, reset)
					hookInstalled = true
					hookNote = "already in .claude/settings.json"
				}
			}
		default:
			fmt.Printf("  %sClaude Code not detected. You can install the hook later by adding%s\n", dWhite, reset)
			fmt.Printf("  %sthis to .claude/settings.json in your repo:%s\n", dWhite, reset)
			fmt.Println()
			fmt.Printf("  %s{%s\n", dim, reset)
			fmt.Printf("  %s  \"hooks\": {%s\n", dim, reset)
			fmt.Printf("  %s    \"PostToolUse\": [{%s\n", dim, reset)
			fmt.Printf("  %s      \"matcher\": \"Write|Edit\",%s\n", dim, reset)
			fmt.Printf("  %s      \"hooks\": [{\"type\": \"command\", \"command\": \"supermodel hook\"}]%s\n", dim, reset)
			fmt.Printf("  %s    }]%s\n", dim, reset)
			fmt.Printf("  %s  }%s\n", dim, reset)
			fmt.Printf("  %s}%s\n", dim, reset)
		}
		fmt.Println()
	}

	// ── Summary ────────────────────────────────────────────────────
	fmt.Printf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", dim, reset)
	fmt.Println()
	fmt.Printf("  %s✓%s  Setup complete\n", bGreen, reset)
	fmt.Println()

	fileModeStr := "disabled"
	if filesEnabled {
		fileModeStr = "enabled"
	}
	fmt.Printf("     %sFile mode%s    %s%s%s\n", dim, reset, bWhite, fileModeStr, reset)
	if filesEnabled {
		fmt.Printf("     %sHook%s         %s%s%s\n", dim, reset, bWhite, hookNote, reset)
	}
	fmt.Println()
	fmt.Printf("  %sNext steps:%s\n", dWhite, reset)
	fmt.Println()
	fmt.Printf("     %ssupermodel analyze%s        %sgenerate graph files now%s\n", bWhite, reset, dim, reset)
	fmt.Printf("     %ssupermodel watch%s          %skeep files updated as you code%s\n", bWhite, reset, dim, reset)
	fmt.Println()
	fmt.Printf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", dim, reset)
	fmt.Println()

	_ = hookInstalled

	if confirmYN("Run 'supermodel analyze' now?", true) {
		fmt.Println()
		return analyze.Run(ctx, cfg, repoDir, analyze.Options{})
	}

	return nil
}

// maskKey returns a display-safe version of the API key.
func maskKey(key string) string {
	if len(key) <= 12 {
		return strings.Repeat("*", len(key))
	}
	return key[:8] + "..." + key[len(key)-4:]
}

// findGitRoot detects the git root from the current working directory.
func findGitRoot() string {
	cwd, _ := os.Getwd()
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return cwd
}

// detectClaude checks if Claude Code is installed.
func detectClaude() bool {
	if _, err := exec.LookPath("claude"); err == nil {
		return true
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
			return true
		}
	}
	return false
}

// installHook writes the PostToolUse hook to .claude/settings.json in repoDir.
// Returns true if newly installed, false if already present. Error on failure.
func installHook(repoDir string) (bool, error) {
	claudeDir := filepath.Join(repoDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return false, fmt.Errorf("create .claude dir: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	var settings map[string]interface{}

	if data, err := os.ReadFile(settingsPath); err == nil {
		if unmarshalErr := json.Unmarshal(data, &settings); unmarshalErr != nil {
			return false, fmt.Errorf("%s contains invalid JSON (%w); skipping to avoid data loss", settingsPath, unmarshalErr)
		}
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	const hookCmd = "supermodel hook"

	// Check if already installed
	if hooks, ok := settings["hooks"].(map[string]interface{}); ok {
		if existing, ok := hooks["PostToolUse"].([]interface{}); ok {
			for _, entry := range existing {
				if m, ok := entry.(map[string]interface{}); ok {
					if innerHooks, ok := m["hooks"].([]interface{}); ok {
						for _, h := range innerHooks {
							if hm, ok := h.(map[string]interface{}); ok {
								if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, "supermodel hook") {
									return false, nil // already installed
								}
							}
						}
					}
				}
			}
		}
	}

	hookEntry := map[string]interface{}{
		"matcher": "Write|Edit",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": hookCmd,
			},
		},
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = make(map[string]interface{})
	}
	existing, _ := hooks["PostToolUse"].([]interface{})
	existing = append(existing, hookEntry)
	hooks["PostToolUse"] = existing
	settings["hooks"] = hooks

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return false, fmt.Errorf("write settings: %w", err)
	}
	return true, nil
}

// ── UI Helpers ──────────────────────────────────────────────────────

// selectMenu shows an arrow-key navigable list and returns the selected index.
func selectMenu(label string, items []string, cursorPos int) int {
	sel := promptui.Select{
		Label:     label,
		Items:     items,
		CursorPos: cursorPos,
		Size:      len(items),
		HideHelp:  true,
		Templates: &promptui.SelectTemplates{
			Label:    fmt.Sprintf("  %s{{ . }}%s", dim, reset),
			Active:   fmt.Sprintf("  %s▸%s {{ . | cyan }}", green, reset),
			Inactive: "    {{ . }}",
			Selected: fmt.Sprintf("  %s✔%s {{ . | cyan }}", green, reset),
		},
	}

	idx, _, err := sel.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  %sCancelled.%s\n\n", dim, reset)
		os.Exit(0)
	}
	return idx
}

// confirmYN shows a Y/N prompt navigable with arrow keys.
func confirmYN(label string, defaultYes bool) bool {
	items := []string{"Yes", "No"}
	cursorPos := 0
	if !defaultYes {
		cursorPos = 1
	}

	sel := promptui.Select{
		Label:     label,
		Items:     items,
		CursorPos: cursorPos,
		Size:      2,
		HideHelp:  true,
		Templates: &promptui.SelectTemplates{
			Label:    fmt.Sprintf("  %s{{ . }}%s", dim, reset),
			Active:   fmt.Sprintf("  %s▸%s {{ . | green }}", green, reset),
			Inactive: "    {{ . }}",
			Selected: fmt.Sprintf("  %s✔%s {{ . | green }}", green, reset),
		},
	}

	idx, _, err := sel.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  %sCancelled.%s\n\n", dim, reset)
		os.Exit(0)
	}
	return idx == 0
}

// promptText shows a text input prompt and returns the entered value.
func promptText(label, defaultVal string) string {
	p := promptui.Prompt{
		Label:   label,
		Default: defaultVal,
		Templates: &promptui.PromptTemplates{
			Prompt:  fmt.Sprintf("  %s{{ . }}:%s ", dim, reset),
			Valid:   fmt.Sprintf("  %s{{ . }}:%s ", dim, reset),
			Invalid: fmt.Sprintf("  %s{{ . }}:%s ", red, reset),
			Success: fmt.Sprintf("  %s✔%s {{ . }}: ", green, reset),
		},
	}

	result, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  %sCancelled.%s\n\n", dim, reset)
		os.Exit(0)
	}
	return strings.TrimSpace(result)
}

func boolPtr(b bool) *bool { return &b }

// keep selectMenu referenced to avoid unused import if callers don't use it directly
var _ = selectMenu
