package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── maskKey ───────────────────────────────────────────────────────────────────

func TestMaskKey_Short(t *testing.T) {
	// Keys ≤12 chars are fully masked with '*'.
	for _, key := range []string{"", "abc", "123456789012"} {
		got := maskKey(key)
		if got != strings.Repeat("*", len([]rune(key))) {
			t.Errorf("maskKey(%q) = %q, want all stars", key, got)
		}
	}
}

func TestMaskKey_Long(t *testing.T) {
	// Keys >12 chars: first 8 chars, "...", last 4 chars visible.
	key := "sk-ant-abcdefghijklmnop"
	got := maskKey(key)
	runes := []rune(key)
	want := string(runes[:8]) + "..." + string(runes[len(runes)-4:])
	if got != want {
		t.Errorf("maskKey(%q) = %q, want %q", key, got, want)
	}
}

func TestMaskKey_ExactlyThirteen(t *testing.T) {
	// 13 chars: just over the threshold.
	key := "abcdefghijklm" // 13 chars
	got := maskKey(key)
	runes := []rune(key)
	want := string(runes[:8]) + "..." + string(runes[len(runes)-4:])
	if got != want {
		t.Errorf("maskKey(%q) = %q, want %q", key, got, want)
	}
}

func TestMaskKey_MultiByteRunes(t *testing.T) {
	// Prior bug: sliced at byte positions, not rune boundaries.
	// Each emoji is 4 bytes; 20 of them = 80 bytes but 20 runes.
	key := strings.Repeat("😀", 20) // 20 runes, 80 bytes
	got := maskKey(key)
	runes := []rune(key)
	want := string(runes[:8]) + "..." + string(runes[len(runes)-4:])
	if got != want {
		t.Errorf("maskKey(20×emoji): got %q, want %q", got, want)
	}
}

// ── boolPtr ───────────────────────────────────────────────────────────────────

func TestBoolPtr(t *testing.T) {
	p := boolPtr(true)
	if p == nil || !*p {
		t.Error("boolPtr(true) should return non-nil pointer to true")
	}
	p = boolPtr(false)
	if p == nil || *p {
		t.Error("boolPtr(false) should return non-nil pointer to false")
	}
}

// ── detectCursor ──────────────────────────────────────────────────────────────

func TestDetectCursor_WithDotCursorDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	if !detectCursor(dir) {
		t.Error("detectCursor: should detect .cursor directory in repoDir")
	}
}

func TestDetectCursor_WithoutDir(t *testing.T) {
	// Empty temp dir has no .cursor and the home dir is redirected.
	dir := t.TempDir()
	// Override HOME so global ~/.cursor doesn't interfere.
	t.Setenv("HOME", t.TempDir())
	if detectCursor(dir) {
		t.Error("detectCursor: should return false when no .cursor dir exists")
	}
}

// ── installHook ───────────────────────────────────────────────────────────────

func TestInstallHook_FreshDir(t *testing.T) {
	dir := t.TempDir()
	installed, err := installHook(dir)
	if err != nil {
		t.Fatalf("installHook: %v", err)
	}
	if !installed {
		t.Error("installHook: want installed=true on first install")
	}

	// Verify the settings file was created with the hook.
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}
	if !strings.Contains(string(data), "supermodel hook") {
		t.Errorf("settings.json should contain 'supermodel hook': %s", data)
	}
}

func TestInstallHook_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := installHook(dir); err != nil {
		t.Fatalf("first installHook: %v", err)
	}
	installed, err := installHook(dir)
	if err != nil {
		t.Fatalf("second installHook: %v", err)
	}
	if installed {
		t.Error("installHook: second install should return installed=false (already present)")
	}
}

func TestInstallHook_ExistingSettings(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write an existing settings file with unrelated content.
	existing := map[string]interface{}{"theme": "dark"}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	installed, err := installHook(dir)
	if err != nil {
		t.Fatalf("installHook with existing settings: %v", err)
	}
	if !installed {
		t.Error("should install into existing settings file")
	}

	// Verify theme is preserved.
	updated, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	var m map[string]interface{}
	if json.Unmarshal(updated, &m) != nil {
		t.Fatal("updated settings is not valid JSON")
	}
	if m["theme"] != "dark" {
		t.Errorf("existing 'theme' field should be preserved, got %v", m["theme"])
	}
}

func TestInstallHook_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write invalid JSON to simulate corrupted settings.
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{invalid}"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := installHook(dir)
	if err == nil {
		t.Error("installHook with invalid JSON: want error to avoid data loss")
	}
}
