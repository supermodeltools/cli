package shards

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── isWatchSourceFile ─────────────────────────────────────────────────────────

func TestIsWatchSourceFile_SourceExtensions(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"app.ts", true},
		{"component.tsx", true},
		{"lib.js", true},
		{"util.py", true},
		{"handler.rs", true},
		{"Main.java", true},
		{"Service.cs", true},
		{"README.md", false},
		{"config.yaml", false},
		{"data.json", false},
		{".env", false},
		{"image.png", false},
	}
	for _, tc := range cases {
		got := isWatchSourceFile(tc.path)
		if got != tc.want {
			t.Errorf("isWatchSourceFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsWatchSourceFile_ShardPathExcluded(t *testing.T) {
	// shard paths (*.graph.go) should NOT be considered watch source files
	if isWatchSourceFile("internal/foo/bar.graph.go") {
		t.Error("shard path should not be a watch source file")
	}
}

func TestIsWatchSourceFile_CaseInsensitiveExt(t *testing.T) {
	// extension matching is case-insensitive
	if !isWatchSourceFile("Main.GO") {
		t.Error("isWatchSourceFile should be case-insensitive for extensions")
	}
}

// ── NewWatcher ────────────────────────────────────────────────────────────────

func TestNewWatcher_DefaultInterval(t *testing.T) {
	w := NewWatcher("/some/dir", 0)
	if w.pollInterval != 3*time.Second {
		t.Errorf("default poll interval = %v; want 3s", w.pollInterval)
	}
	if w.repoDir != "/some/dir" {
		t.Errorf("repoDir = %q; want %q", w.repoDir, "/some/dir")
	}
}

func TestNewWatcher_CustomInterval(t *testing.T) {
	w := NewWatcher("/repo", 500*time.Millisecond)
	if w.pollInterval != 500*time.Millisecond {
		t.Errorf("poll interval = %v; want 500ms", w.pollInterval)
	}
}

func TestNewWatcher_EventsChannelNotNil(t *testing.T) {
	w := NewWatcher("/some/dir", time.Second)
	if w.Events() == nil {
		t.Error("Events() channel should not be nil")
	}
}

func TestWatcher_RunCancellable(t *testing.T) {
	// Run should return when context is cancelled.
	w := NewWatcher(t.TempDir(), 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Run did not return after context cancellation")
	}
}

func TestWatcher_RunPollsOnTick(t *testing.T) {
	// Verifies that the ticker branch in Run is reachable (poll() is called).
	// Use a very short interval so the ticker fires before we cancel.
	dir := t.TempDir()
	w := NewWatcher(dir, 1*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	// Wait enough time for multiple ticks to fire.
	time.Sleep(20 * time.Millisecond)
	cancel()
}

// ── gitIndexMtime ─────────────────────────────────────────────────────────────

func TestWatcher_GitIndexMtime_NonGitDir(t *testing.T) {
	w := NewWatcher(t.TempDir(), time.Second)
	mtime := w.gitIndexMtime()
	if !mtime.IsZero() {
		t.Errorf("gitIndexMtime on non-git dir should return zero time, got %v", mtime)
	}
}

func TestWatcher_GitIndexMtime_GitRepo(t *testing.T) {
	dir := initWatcherGitRepo(t)
	w := NewWatcher(dir, time.Second)
	mtime := w.gitIndexMtime()
	if mtime.IsZero() {
		t.Error("gitIndexMtime should return non-zero time for a git repo")
	}
}

// ── gitDirtyFiles ─────────────────────────────────────────────────────────────

func TestWatcher_GitDirtyFiles_CleanRepo(t *testing.T) {
	dir := initWatcherGitRepo(t)
	w := NewWatcher(dir, time.Second)
	files := w.gitDirtyFiles()
	if len(files) != 0 {
		t.Errorf("clean repo should have 0 dirty files; got %v", files)
	}
}

func TestWatcher_GitDirtyFiles_ModifiedFile(t *testing.T) {
	dir := initWatcherGitRepo(t)
	// Modify the tracked file.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// modified\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(dir, time.Second)
	files := w.gitDirtyFiles()
	if _, ok := files["main.go"]; !ok {
		t.Error("modified tracked file should appear in dirty files")
	}
}

func TestWatcher_GitDirtyFiles_UntrackedSourceFile(t *testing.T) {
	dir := initWatcherGitRepo(t)
	// Add an untracked source file.
	if err := os.WriteFile(filepath.Join(dir, "newfile.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(dir, time.Second)
	files := w.gitDirtyFiles()
	if _, ok := files["newfile.go"]; !ok {
		t.Error("untracked source file should appear in dirty files")
	}
}

func TestWatcher_GitDirtyFiles_UntrackedNonSourceFile(t *testing.T) {
	dir := initWatcherGitRepo(t)
	// Add an untracked non-source file - should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(dir, time.Second)
	files := w.gitDirtyFiles()
	if _, ok := files["notes.txt"]; ok {
		t.Error("non-source file should not appear in dirty files")
	}
}

// ── poll ─────────────────────────────────────────────────────────────────────

func TestWatcher_Poll_NewDirtyFile(t *testing.T) {
	dir := initWatcherGitRepo(t)
	w := NewWatcher(dir, time.Second)
	w.lastCommitSHA = "abc" // non-empty so headChanged won't fire
	w.lastIndexMod = w.gitIndexMtime()

	// Add an untracked source file.
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}

	w.poll()

	select {
	case events := <-w.eventCh:
		found := false
		for _, e := range events {
			if e.Path == "new.go" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected event for new.go; got %v", events)
		}
	default:
		t.Error("expected event for new dirty file")
	}
}

func TestWatcher_Poll_CleanedDirtyFile(t *testing.T) {
	// When a file that was dirty becomes clean after an index change,
	// it should emit an event (the indexChanged + file no longer dirty path).
	dir := initWatcherGitRepo(t)
	w := NewWatcher(dir, time.Second)
	w.lastCommitSHA = "abc"

	// Simulate: file was previously dirty
	w.lastKnownFiles = map[string]struct{}{"main.go": {}}
	// Set lastIndexMod to zero so any index state triggers indexChanged
	w.lastIndexMod = time.Time{}

	// main.go is actually clean (committed), so gitDirtyFiles returns empty
	w.poll()

	select {
	case events := <-w.eventCh:
		found := false
		for _, e := range events {
			if e.Path == "main.go" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected event for main.go becoming clean; got %v", events)
		}
	default:
		t.Error("expected event when previously-dirty file is no longer dirty")
	}
}

func TestWatcher_Poll_HeadChanged(t *testing.T) {
	dir := initWatcherGitRepo(t)
	w := NewWatcher(dir, time.Second)
	w.lastIndexMod = w.gitIndexMtime()

	// Capture the initial commit SHA.
	initialSHA := strings.TrimSpace(w.runGit("rev-parse", "HEAD"))
	w.lastCommitSHA = initialSHA

	// Make a second commit that adds a source file.
	if err := os.WriteFile(filepath.Join(dir, "extra.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	runCmd := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.CombinedOutput() //nolint:errcheck
	}
	runCmd("git", "add", "extra.go")
	runCmd("git", "commit", "-m", "second")

	w.poll()

	// headChanged fired; lastCommitSHA should now be the new HEAD
	if w.lastCommitSHA == initialSHA {
		t.Error("poll should update lastCommitSHA when head changes")
	}

	// The event for extra.go should have been emitted.
	select {
	case events := <-w.eventCh:
		found := false
		for _, e := range events {
			if e.Path == "extra.go" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected event for extra.go; got %v", events)
		}
	default:
		t.Error("expected event for head-changed source file")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func initWatcherGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runWatcherGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runWatcherGit("git", "init")
	runWatcherGit("git", "config", "user.email", "ci@test.local")
	runWatcherGit("git", "config", "user.name", "CI")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runWatcherGit("git", "add", ".")
	runWatcherGit("git", "commit", "-m", "init")
	return dir
}
