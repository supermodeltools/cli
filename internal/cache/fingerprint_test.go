package cache

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "init")
	return dir
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
}

func TestRepoFingerprint_CleanRepo(t *testing.T) {
	dir := initGitRepo(t)
	fp, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp == "" {
		t.Fatal("expected non-empty fingerprint")
	}
	// Should be a plain commit SHA (40 hex chars).
	if len(fp) != 40 {
		t.Errorf("expected 40-char commit SHA, got %q (%d chars)", fp, len(fp))
	}
}

func TestRepoFingerprint_DirtyRepo(t *testing.T) {
	dir := initGitRepo(t)
	// Modify a tracked file.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Dirty fingerprint should contain a colon separator.
	if len(fp) <= 40 {
		t.Errorf("expected dirty fingerprint (>40 chars), got %q", fp)
	}
}

func TestRepoFingerprint_StableForClean(t *testing.T) {
	dir := initGitRepo(t)
	fp1, _ := RepoFingerprint(dir)
	fp2, _ := RepoFingerprint(dir)
	if fp1 != fp2 {
		t.Errorf("fingerprint should be stable: %q != %q", fp1, fp2)
	}
}

func TestRepoFingerprint_ChangesAfterCommit(t *testing.T) {
	dir := initGitRepo(t)
	fp1, _ := RepoFingerprint(dir)

	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "second")

	fp2, _ := RepoFingerprint(dir)
	if fp1 == fp2 {
		t.Error("fingerprint should change after commit")
	}
}

func TestRepoFingerprint_ChangesForUntrackedFile(t *testing.T) {
	dir := initGitRepo(t)
	fp1, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "generated.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp2, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 == fp2 {
		t.Error("fingerprint should change when an untracked uploadable file appears")
	}

	if err := os.WriteFile(filepath.Join(dir, "generated.go"), []byte("package main\nfunc generated() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp3, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp2 == fp3 {
		t.Error("fingerprint should change when untracked file contents change")
	}
}

func TestRepoFingerprint_IgnoresGeneratedArtifacts(t *testing.T) {
	dir := initGitRepo(t)
	fp1, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, ".supermodel"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".supermodel", "shards.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.graph.go"), []byte("// generated graph\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs-output"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs-output", "index.html"), []byte("<html></html>\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fp2, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("fingerprint should ignore generated artifacts: %q != %q", fp1, fp2)
	}
}

func TestRepoFingerprint_IgnoresHiddenDirsAndSecrets(t *testing.T) {
	dir := initGitRepo(t)
	fp1, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		filepath.Join(".claude", "settings.json"),
		filepath.Join(".github", "workflows", "ci.yml"),
		".env",
		"prod.key",
		"credentials.txt",
	} {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("ignored\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	fp2, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("fingerprint should ignore non-uploaded hidden dirs and secrets: %q != %q", fp1, fp2)
	}
}

func TestRepoFingerprint_IgnoresGeneratedArtifactsWhenSourceIsDirty(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "main.graph.go"), []byte("// generated graph\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "-f", "main.graph.go")
	run(t, dir, "git", "commit", "-m", "track generated artifact")

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.graph.go"), []byte("// regenerated graph\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fpWithGeneratedDirty, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "main.graph.go"), []byte("// generated graph\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fpSourceOnlyDirty, err := RepoFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fpWithGeneratedDirty != fpSourceOnlyDirty {
		t.Fatalf("generated shard changes should not affect source fingerprint: %q != %q", fpWithGeneratedDirty, fpSourceOnlyDirty)
	}
}

func TestRepoFingerprint_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := RepoFingerprint(dir)
	if err == nil {
		t.Error("expected error for non-git dir")
	}
}

func TestAnalysisKey_DifferentTypes(t *testing.T) {
	fp := "abc123"
	k1 := AnalysisKey(fp, "graph", "dev")
	k2 := AnalysisKey(fp, "dead-code", "dev")
	if k1 == k2 {
		t.Error("different analysis types should produce different keys")
	}
}

func TestAnalysisKey_Stable(t *testing.T) {
	k1 := AnalysisKey("abc", "graph", "dev")
	k2 := AnalysisKey("abc", "graph", "dev")
	if k1 != k2 {
		t.Error("same inputs should produce same key")
	}
}
