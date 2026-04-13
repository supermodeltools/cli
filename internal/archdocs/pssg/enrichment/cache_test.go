package enrichment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeCache(t *testing.T, dir, slug string, entry CacheEntry) {
	t.Helper()
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug+".json"), data, 0600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
}

// ── ReadCache ─────────────────────────────────────────────────────────────────

func TestReadCache_ValidFile(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, "test-slug", CacheEntry{
		ContentHash: "abc",
		Enrichment:  map[string]interface{}{"title": "Test"},
	})

	got := ReadCache(dir, "test-slug")
	if got == nil {
		t.Fatal("expected non-nil enrichment")
	}
	if got["title"] != "Test" {
		t.Errorf("title: got %v", got["title"])
	}
}

func TestReadCache_MissingFile(t *testing.T) {
	dir := t.TempDir()
	if got := ReadCache(dir, "nonexistent"); got != nil {
		t.Errorf("missing file: expected nil, got %v", got)
	}
}

func TestReadCache_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ReadCache(dir, "bad"); got != nil {
		t.Errorf("invalid JSON: expected nil, got %v", got)
	}
}

// ── ReadAllCaches ─────────────────────────────────────────────────────────────

func TestReadAllCaches_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := ReadAllCaches(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestReadAllCaches_EmptyCacheDir(t *testing.T) {
	result, err := ReadAllCaches("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestReadAllCaches_NonExistentDir(t *testing.T) {
	result, err := ReadAllCaches("/nonexistent-enrichment-dir-xyz")
	if err != nil {
		t.Fatalf("non-existent dir should return empty result, not error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestReadAllCaches_WithFiles(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, "recipe-a", CacheEntry{Enrichment: map[string]interface{}{"field": "val"}})
	writeCache(t, dir, "recipe-b", CacheEntry{Enrichment: map[string]interface{}{"other": "data"}})
	// Also add a non-JSON file and a subdir (should be skipped)
	if err := os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	result, err := ReadAllCaches(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d: %v", len(result), result)
	}
	if result["recipe-a"] == nil || result["recipe-b"] == nil {
		t.Error("expected both recipes in result")
	}
}

func TestReadAllCaches_UnreadableDir(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) }) //nolint:errcheck

	_, err := ReadAllCaches(dir)
	if err == nil {
		t.Error("expected error for unreadable cache dir")
	}
}

func TestReadAllCaches_SkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	writeCache(t, dir, "good", CacheEntry{Enrichment: map[string]interface{}{"k": "v"}})

	result, err := ReadAllCaches(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 valid entry, got %d", len(result))
	}
}

// ── GetIngredients ────────────────────────────────────────────────────────────

func TestGetIngredients_Present(t *testing.T) {
	data := map[string]interface{}{
		"ingredients": []interface{}{
			map[string]interface{}{"name": "flour", "amount": "2 cups"},
			map[string]interface{}{"name": "sugar", "amount": "1 cup"},
		},
	}
	got := GetIngredients(data)
	if len(got) != 2 {
		t.Errorf("expected 2 ingredients, got %d", len(got))
	}
}

func TestGetIngredients_Missing(t *testing.T) {
	if got := GetIngredients(map[string]interface{}{}); got != nil {
		t.Errorf("missing key: expected nil, got %v", got)
	}
}

func TestGetIngredients_WrongType(t *testing.T) {
	data := map[string]interface{}{"ingredients": "string value"}
	if got := GetIngredients(data); got != nil {
		t.Errorf("wrong type: expected nil, got %v", got)
	}
}

func TestGetIngredients_SkipsNonMapItems(t *testing.T) {
	data := map[string]interface{}{
		"ingredients": []interface{}{
			map[string]interface{}{"name": "flour"},
			"not a map",
			42,
		},
	}
	got := GetIngredients(data)
	if len(got) != 1 {
		t.Errorf("expected 1 (skipping non-map items), got %d", len(got))
	}
}

// ── GetGear ───────────────────────────────────────────────────────────────────

func TestGetGear_Present(t *testing.T) {
	data := map[string]interface{}{
		"gear": []interface{}{
			map[string]interface{}{"name": "pan"},
		},
	}
	got := GetGear(data)
	if len(got) != 1 {
		t.Errorf("expected 1 gear item, got %d", len(got))
	}
}

func TestGetGear_Missing(t *testing.T) {
	if got := GetGear(map[string]interface{}{}); got != nil {
		t.Errorf("missing: expected nil, got %v", got)
	}
}

func TestGetGear_WrongType(t *testing.T) {
	data := map[string]interface{}{"gear": "string"}
	if got := GetGear(data); got != nil {
		t.Errorf("wrong type: expected nil, got %v", got)
	}
}

// ── GetCookingTips ────────────────────────────────────────────────────────────

func TestGetCookingTips_Present(t *testing.T) {
	data := map[string]interface{}{
		"cookingTips": []interface{}{"tip1", "tip2"},
	}
	got := GetCookingTips(data)
	if len(got) != 2 || got[0] != "tip1" {
		t.Errorf("got %v", got)
	}
}

func TestGetCookingTips_Missing(t *testing.T) {
	if got := GetCookingTips(map[string]interface{}{}); got != nil {
		t.Errorf("missing: expected nil, got %v", got)
	}
}

func TestGetCookingTips_WrongType(t *testing.T) {
	data := map[string]interface{}{"cookingTips": "single tip"}
	if got := GetCookingTips(data); got != nil {
		t.Errorf("wrong type: expected nil, got %v", got)
	}
}

func TestGetCookingTips_SkipsNonString(t *testing.T) {
	data := map[string]interface{}{
		"cookingTips": []interface{}{"tip1", 42, "tip2"},
	}
	got := GetCookingTips(data)
	if len(got) != 2 {
		t.Errorf("expected 2 (skip non-string), got %d: %v", len(got), got)
	}
}

// ── GetCoachingPrompt ─────────────────────────────────────────────────────────

func TestGetCoachingPrompt_Present(t *testing.T) {
	data := map[string]interface{}{"coachingPrompt": "Be patient with this recipe."}
	got := GetCoachingPrompt(data)
	if got != "Be patient with this recipe." {
		t.Errorf("got %q", got)
	}
}

func TestGetCoachingPrompt_Missing(t *testing.T) {
	if got := GetCoachingPrompt(map[string]interface{}{}); got != "" {
		t.Errorf("missing: expected empty, got %q", got)
	}
}

func TestGetCoachingPrompt_WrongType(t *testing.T) {
	data := map[string]interface{}{"coachingPrompt": 42}
	if got := GetCoachingPrompt(data); got != "" {
		t.Errorf("wrong type: expected empty, got %q", got)
	}
}
