package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/supermodeltools/cli/internal/config"
	"github.com/supermodeltools/cli/internal/shards"
)

// noConfigCommands are subcommands that work without a config file.
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
}

var rootCmd = &cobra.Command{
	Use:   "supermodel [path]",
	Short: "Give your AI coding agent a map of your codebase",
	Long: `Runs a full generate on startup (using cached graph if available), then
enters daemon mode. Listens for file-change notifications from the
'supermodel hook' command and incrementally re-renders affected files.

Press Ctrl+C to stop and remove graph files.

See https://supermodeltools.com for documentation.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Walk up to the root command name to get the subcommand.
		name := cmd.Name()
		if noConfigCommands[name] {
			return nil
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		if cfg.APIKey == "" {
			fmt.Fprintln(os.Stderr, "Run 'supermodel setup' to get started.")
			os.Exit(1)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := cfg.RequireAPIKey(); err != nil {
			return err
		}
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
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
	watchCacheFile    string
	watchDebounce     time.Duration
	watchNotifyPort   int
	watchFSWatch      bool
	watchPollInterval time.Duration
)

func init() {
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
