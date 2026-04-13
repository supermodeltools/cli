package render

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// ── svgEscape ─────────────────────────────────────────────────────────────────

func TestSvgEscape(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello", "hello"},
		{"a & b", "a &amp; b"},
		{"<tag>", "&lt;tag&gt;"},
		{`say "hi"`, "say &quot;hi&quot;"},
		{"a & <b>", "a &amp; &lt;b&gt;"},
	}
	for _, tc := range cases {
		if got := svgEscape(tc.in); got != tc.want {
			t.Errorf("svgEscape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ── renderBarsSVG ─────────────────────────────────────────────────────────────

func TestRenderBarsSVG_Empty(t *testing.T) {
	if got := renderBarsSVG(nil, 0, 0, 100, 20, 5); got != "" {
		t.Errorf("empty bars: got %q, want empty", got)
	}
}

func TestRenderBarsSVG_SingleBar(t *testing.T) {
	bars := []NameCount{{Name: "Italian", Count: 10}}
	got := renderBarsSVG(bars, 60, 200, 400, 20, 5)
	if !strings.Contains(got, "Italian") {
		t.Errorf("should contain bar name: %s", got)
	}
	if !strings.Contains(got, "<rect") {
		t.Errorf("should contain rect element: %s", got)
	}
}

func TestRenderBarsSVG_AllZeroCount(t *testing.T) {
	bars := []NameCount{{Name: "A", Count: 0}}
	got := renderBarsSVG(bars, 0, 0, 100, 20, 5)
	if !strings.Contains(got, "<rect") {
		t.Error("zero-count bars should still produce rect elements")
	}
}

// ── GenerateHomepageShareSVG ──────────────────────────────────────────────────

func TestGenerateHomepageShareSVG(t *testing.T) {
	stats := []NameCount{{Name: "Italian", Count: 5}, {Name: "French", Count: 3}}
	got := GenerateHomepageShareSVG("My Site", "A cooking site", stats, 42)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("should start with <svg, got: %.50s", got)
	}
	if !strings.Contains(got, "42") {
		t.Error("should contain total entity count")
	}
}

func TestGenerateHomepageShareSVG_Empty(t *testing.T) {
	got := GenerateHomepageShareSVG("Site", "Desc", nil, 0)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("empty stats should still produce SVG: %.50s", got)
	}
}

// ── GenerateEntityShareSVG ────────────────────────────────────────────────────

func TestGenerateEntityShareSVG(t *testing.T) {
	got := GenerateEntityShareSVG("My Site", "Spaghetti Carbonara", "Main Course", "Italian", "Easy")
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("should start with <svg, got: %.50s", got)
	}
	if !strings.Contains(got, "Spaghetti") {
		t.Error("should contain entity title")
	}
}

// ── GenerateHubShareSVG ───────────────────────────────────────────────────────

func TestGenerateHubShareSVG(t *testing.T) {
	topTypes := []NameCount{{Name: "Pasta", Count: 10}}
	got := GenerateHubShareSVG("My Site", "Italian", "Cuisine", 25, topTypes)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("should start with <svg, got: %.50s", got)
	}
	if !strings.Contains(got, "Italian") {
		t.Error("should contain hub name")
	}
}

// ── GenerateTaxIndexShareSVG ──────────────────────────────────────────────────

func TestGenerateTaxIndexShareSVG(t *testing.T) {
	topEntries := []NameCount{{Name: "Italian", Count: 5}}
	got := GenerateTaxIndexShareSVG("My Site", "Cuisine", topEntries)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("should start with <svg, got: %.50s", got)
	}
}

// ── GenerateAllEntitiesShareSVG ───────────────────────────────────────────────

func TestGenerateAllEntitiesShareSVG(t *testing.T) {
	typeDist := []NameCount{{Name: "Dinner", Count: 50}, {Name: "Lunch", Count: 30}}
	got := GenerateAllEntitiesShareSVG("My Site", 100, typeDist)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("should start with <svg, got: %.50s", got)
	}
}

func TestGenerateAllEntitiesShareSVG_Empty(t *testing.T) {
	got := GenerateAllEntitiesShareSVG("My Site", 0, nil)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("empty dist: should produce SVG, got: %.50s", got)
	}
}

// TestGenerateAllEntitiesShareSVG_AllZeroCounts covers the totalForBar==0 branch
// (L190): typeDist is non-nil but every Count is 0, so totalForBar is set to 1
// to avoid division by zero.
func TestGenerateAllEntitiesShareSVG_AllZeroCounts(t *testing.T) {
	dist := []NameCount{
		{Name: "Dinner", Count: 0},
		{Name: "Lunch", Count: 0},
	}
	got := GenerateAllEntitiesShareSVG("My Site", 0, dist)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("all-zero counts: should produce SVG, got: %.50s", got)
	}
}

// ── limit capping branches ────────────────────────────────────────────────────

func TestGenerateHomepageShareSVG_OverLimit(t *testing.T) {
	// >8 items should be capped at 8
	var stats []NameCount
	for i := 0; i < 12; i++ {
		stats = append(stats, NameCount{Name: fmt.Sprintf("Cat%d", i), Count: i + 1})
	}
	got := GenerateHomepageShareSVG("My Site", "A cooking site", stats, 100)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("over-limit stats should still produce SVG")
	}
}

func TestGenerateHubShareSVG_OverLimit(t *testing.T) {
	// >6 items should be capped at 6
	var topTypes []NameCount
	for i := 0; i < 9; i++ {
		topTypes = append(topTypes, NameCount{Name: fmt.Sprintf("Type%d", i), Count: i + 1})
	}
	got := GenerateHubShareSVG("My Site", "Italian", "Cuisine", 50, topTypes)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("over-limit hub should still produce SVG")
	}
}

func TestGenerateTaxIndexShareSVG_OverLimit(t *testing.T) {
	// >10 entries should be capped at 10
	var entries []NameCount
	for i := 0; i < 15; i++ {
		entries = append(entries, NameCount{Name: fmt.Sprintf("Entry%d", i), Count: i + 1})
	}
	got := GenerateTaxIndexShareSVG("My Site", "Cuisine", entries)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("over-limit tax index should still produce SVG")
	}
}

func TestGenerateAllEntitiesShareSVG_OverLimit(t *testing.T) {
	// >8 type dist items should be capped at 8
	// Use very uneven counts to trigger the w < 2 minimum-width branch.
	dist := []NameCount{
		{Name: "Big", Count: 1000},
		{Name: "Tiny", Count: 1}, // 1*1080/1001 = 1 → w < 2 → w = 2
		{Name: "Type2", Count: 10},
		{Name: "Type3", Count: 20},
		{Name: "Type4", Count: 15},
		{Name: "Type5", Count: 8},
		{Name: "Type6", Count: 5},
		{Name: "Type7", Count: 3},
		{Name: "Type8", Count: 2},
		{Name: "Type9", Count: 2},
		{Name: "TypeA", Count: 1},
		{Name: "TypeB", Count: 1},
	}
	got := GenerateAllEntitiesShareSVG("My Site", 200, dist)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("over-limit all-entities should still produce SVG")
	}
}

func TestGenerateEntityShareSVG_EmptyPills(t *testing.T) {
	// All pill labels empty → all pills skipped via continue
	got := GenerateEntityShareSVG("My Site", "Dish Name", "", "", "")
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("empty pills should still produce SVG")
	}
}

// ── GenerateLetterShareSVG ────────────────────────────────────────────────────

func TestGenerateLetterShareSVG(t *testing.T) {
	got := GenerateLetterShareSVG("My Site", "Cuisine", "A", 7)
	if !strings.HasPrefix(got, "<svg") {
		t.Errorf("should start with <svg, got: %.50s", got)
	}
	if !strings.Contains(got, "7") {
		t.Error("should contain entry count")
	}
}

func TestTruncateASCII(t *testing.T) {
	cases := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},       // short — no truncation
		{"hello", 5, "hello"},        // exactly max — no truncation
		{"hello world", 6, "hello…"}, // truncated to 5 runes + ellipsis
		{"", 5, ""},                  // empty string
	}
	for _, c := range cases {
		got := truncate(c.input, c.max)
		if got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.input, c.max, got, c.want)
		}
	}
}

// TestTruncateMultiByte verifies that truncate does not slice inside a multi-byte
// UTF-8 character sequence, which would produce invalid UTF-8 in the SVG output.
// Before the fix, truncate used byte-based slicing: s[:max-1].
// For a string like "Ñandú" (6 runes but 8 bytes), truncating at max=3 would
// compute s[:2] = [0xC3, 0x9C] — the first 2 bytes of "Ñ" — yielding "Ñ"
// rather than the expected "Ña". The important invariant is that the output is
// always valid UTF-8 and has exactly min(len(runes), max) rune-units.
func TestTruncateMultiByte(t *testing.T) {
	cases := []struct {
		input string
		max   int
		want  string
	}{
		// "Über" is 4 runes, 6 bytes (Ü = 2 bytes)
		{"Über", 10, "Über"}, // no truncation
		{"Über", 4, "Über"},  // exactly max runes — no truncation
		{"Über", 3, "Üb…"},   // 2 runes + ellipsis
		{"Über", 2, "Ü…"},    // 1 rune + ellipsis

		// "Ñandú" is 5 runes, 7 bytes
		{"Ñandú", 4, "Ñan…"}, // 3 runes + ellipsis

		// "日本語" is 3 runes, 9 bytes (each char = 3 bytes)
		{"日本語テスト", 4, "日本語…"}, // 3 runes + ellipsis
	}
	for _, c := range cases {
		got := truncate(c.input, c.max)
		if got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.input, c.max, got, c.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("truncate(%q, %d) = %q — result is not valid UTF-8", c.input, c.max, got)
		}
	}
}
