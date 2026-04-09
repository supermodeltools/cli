package shards

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WatchEvent represents a detected file change.
type WatchEvent struct {
	Path string
	Time time.Time
}

// Watcher detects source file changes using git as the source of truth.
type Watcher struct {
	repoDir      string
	pollInterval time.Duration

	mu             sync.Mutex
	lastKnownFiles map[string]struct{}
	lastIndexMod   time.Time
	lastCommitSHA  string // HEAD at last poll; empty until first poll

	eventCh chan []WatchEvent
}

// NewWatcher creates a watcher for the given repo directory.
func NewWatcher(repoDir string, pollInterval time.Duration) *Watcher {
	if pollInterval <= 0 {
		pollInterval = 3 * time.Second
	}
	return &Watcher{
		repoDir:        repoDir,
		pollInterval:   pollInterval,
		lastKnownFiles: make(map[string]struct{}),
		eventCh:        make(chan []WatchEvent, 16),
	}
}

// Events returns the channel that receives batches of change events.
func (w *Watcher) Events() <-chan []WatchEvent {
	return w.eventCh
}

// Run starts the watcher loop. It blocks until the context is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	defer close(w.eventCh)

	w.lastIndexMod = w.gitIndexMtime()
	w.lastCommitSHA = strings.TrimSpace(w.runGit("rev-parse", "HEAD"))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *Watcher) poll() {
	indexMod := w.gitIndexMtime()
	indexChanged := indexMod != w.lastIndexMod
	if indexChanged {
		w.lastIndexMod = indexMod
	}

	// Detect HEAD change (git commit, pull, checkout, merge, stash pop, etc.)
	currentSHA := strings.TrimSpace(w.runGit("rev-parse", "HEAD"))
	headChanged := currentSHA != "" && currentSHA != w.lastCommitSHA && w.lastCommitSHA != ""

	currentDirty := w.gitDirtyFiles()

	w.mu.Lock()
	defer w.mu.Unlock()

	var newEvents []WatchEvent
	now := time.Now()

	if headChanged {
		// Emit all files that changed between the old and new HEAD.
		diffOutput := w.runGit("diff", "--name-only", w.lastCommitSHA, currentSHA)
		for _, line := range strings.Split(diffOutput, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && isWatchSourceFile(line) {
				newEvents = append(newEvents, WatchEvent{Path: line, Time: now})
			}
		}
		w.lastCommitSHA = currentSHA
	}

	for f := range currentDirty {
		if _, known := w.lastKnownFiles[f]; !known {
			newEvents = append(newEvents, WatchEvent{Path: f, Time: now})
		}
	}

	if indexChanged {
		for f := range w.lastKnownFiles {
			if _, stillDirty := currentDirty[f]; !stillDirty {
				newEvents = append(newEvents, WatchEvent{Path: f, Time: now})
			}
		}
	}

	w.lastKnownFiles = currentDirty

	if len(newEvents) > 0 {
		w.eventCh <- newEvents
	}
}

func (w *Watcher) gitDirtyFiles() map[string]struct{} {
	files := make(map[string]struct{})

	out := w.runGit("diff", "--name-only", "HEAD")
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isWatchSourceFile(line) {
			files[line] = struct{}{}
		}
	}

	out = w.runGit("ls-files", "--others", "--exclude-standard")
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isWatchSourceFile(line) {
			files[line] = struct{}{}
		}
	}

	return files
}

func (w *Watcher) gitIndexMtime() time.Time {
	info, err := os.Stat(filepath.Join(w.repoDir, ".git", "index"))
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func (w *Watcher) runGit(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = w.repoDir
	out, _ := cmd.Output()
	return string(out)
}

func isWatchSourceFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if !SourceExtensions[ext] {
		return false
	}
	return !isShardPath(path)
}
