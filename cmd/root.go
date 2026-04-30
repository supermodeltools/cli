package cmd

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/setup"
	"github.com/supermodeltools/cli/internal/shards"
)

// openDevTty opens /dev/tty (the process's controlling terminal). It is a
// package-level var so tests can replace it with a mock. On Windows,
// os.Open("/dev/tty") will fail, which is expected — Windows Console API
// (used by term.IsTerminal above) handles Windows Terminal/PowerShell.
var openDevTty = func() (*os.File, error) {
	return nil, fmt.Errorf("not implemented")
}

// stdinIsTerminal reports whether stdin is connected to an interactive
// terminal. Pulled into a var so tests can stub it.
var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(syscall.Stdin)) //nolint:unconvert // syscall.Stdin is uintptr on Windows
}

// rootAction enumerates the three branches the bare `supermodel` command
// can take based on auth state and whether stdin is interactive.
type rootAction int

const (
	// runWatch: an API key is configured; start the watch daemon.
	runWatch rootAction = iota
	// runSetup: no key, but stdin is a TTY — drop into the setup wizard.
	runSetup
	// errNotAuthenticated: no key and we can't prompt; surface a clean error.
	errNotAuthenticated
)

// pickRootAction is the decision logic for bare `supermodel`. Pulled into
// a pure function so the dispatch matrix can be unit-tested without
// invoking the wizard or the watch daemon (issue #151).
func pickRootAction(hasAPIKey, interactive bool) rootAction {
	switch {
	case hasAPIKey:
		return runWatch
	case interactive:
		return runSetup
	default:
		return errNotAuthenticated
	}
}

// persistentPreRunE gates every subcommand on having an API key, except
// for the bare root command (which handles its own dispatch — see issue
// #151) and noConfigCommands.
func persistentPreRunE(cmd *cobra.Command, args []string) error {
	for c := cmd; c != nil; c = c.Parent() {
		if noConfigCommands[c.Name()] {
			return nil
		}
	}
	// The root command (bare `supermodel`) has no parent. Skip the
	// pre-run check and let RunE handle interactive vs CI dispatch.
	if cmd.Parent() == nil {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("run 'supermodel setup' to get started")
	}
	return nil
}

// noConfigCommands are subcommands that work without a config file or API key.
// Includes Cobra's internal shell-completion helpers to avoid crashing them.
var noConfigCommands = map[string]bool{
	"setup":            true,
	"login":            true,
	"logout":           true,
	"version":          true,
	"help":             true,
	"completion":       true,
	"__complete":       true,
	"__completeNoDesc": true,
	// Commands that work fully offline or have local-only modes:
	"compact": true, // entirely local (AST transforms, no API)
	"clean":   true, // removes .graph.* files, no API
	"status":  true, // reads config; works even when not authenticated
	"hook":    true, // forwards events to daemon; no API needed
	"restore": true, // has --local fallback; API key is optional
}

var rootCmd = &cobra.Command{
	Use:   "supermodel",
	Short: "Give your AI coding agent a map of your codebase",
	Long: `Runs a full generate on startup (using cached graph if available), then
enters daemon mode. Listens for file-change notifications from the
'supermodel hook' command and incrementally re-renders affected files.

Press Ctrl+C to stop and remove graph files.

See https://supermodeltools.com for documentation.`,
	Args:              cobra.NoArgs,
	SilenceUsage:      true,
	PersistentPreRunE: persistentPreRunE,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		// First-run dispatch (issue #151): pick a branch based on auth
		// state and whether stdin is interactive. Non-interactive callers
		// (CI, scripts, piped stdin) get a clean error so they can't hang
		// on a browser-auth prompt.
		switch pickRootAction(cfg.APIKey != "", stdinIsTerminal()) {
		case runSetup:
			return setup.Run(cmd.Context(), cfg)
		case errNotAuthenticated:
			return fmt.Errorf("not authenticated — run `supermodel setup` or set SUPERMODEL_API_KEY")
		}
		dir := watchDir
		opts := shards.WatchOptions{
			CacheFile:    watchCacheFile,
			Debounce:     watchDebounce,
			NotifyPort:   watchNotifyPort,
			FSWatch:      watchFSWatch,
			PollInterval: watchPollInterval,
		}
		return shards.Watch(cmd.Context(), cfg, dir, opts)
	},
}

var (
	watchDir          string
	watchCacheFile    string
	watchDebounce     time.Duration
	watchNotifyPort   int
	watchFSWatch      bool
	watchPollInterval time.Duration
)

func init() {
	rootCmd.Flags().StringVar(&watchDir, "dir", ".", "project directory")
	rootCmd.Flags().StringVar(&watchCacheFile, "cache-file", "", "override cache file path")
	rootCmd.Flags().DurationVar(&watchDebounce, "debounce", 2*time.Second, "debounce duration before processing changes")
	rootCmd.Flags().IntVar(&watchNotifyPort, "notify-port", 7734, "UDP port for hook notifications")
	rootCmd.Flags().BoolVar(&watchFSWatch, "fs-watch", false, "enable git-poll fallback")
	rootCmd.Flags().DurationVar(&watchPollInterval, "poll-interval", 3*time.Second, "git poll interval when --fs-watch is enabled")
}

// Execute is the entry point called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
