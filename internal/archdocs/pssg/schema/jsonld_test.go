package schema

import (
	"testing"
	"unicode/utf8"
)

func TestStepName(t *testing.T) {
	// ASCII truncation: step longer than 80 bytes, no sentence break.
	long := ""
	for i := 0; i < 85; i++ {
		long += "a"
	}
	got := stepName(long)
	if len([]rune(got)) != 80 { // 77 + len("...") = 80
		t.Errorf("ASCII truncation: got %d runes, want 80", len([]rune(got)))
	}

	// Short sentence extraction.
	got = stepName("Mix ingredients. Then bake for 30 minutes.")
	if got != "Mix ingredients." {
		t.Errorf("short sentence: got %q, want %q", got, "Mix ingredients.")
	}

	// Multi-byte truncation: 85 'é' chars (2 bytes each), no period.
	// Byte length > 80 but we must truncate at rune boundary.
	multiLong := ""
	for i := 0; i < 85; i++ {
		multiLong += "é"
	}
	got = stepName(multiLong)
	if !utf8.ValidString(got) {
		t.Errorf("multi-byte truncation produced invalid UTF-8: %q", got)
	}
	if len([]rune(got)) != 80 { // 77 runes + "..."
		t.Errorf("multi-byte truncation: got %d runes, want 80", len([]rune(got)))
	}

	// Multi-byte sentence: 'é' × 79 chars followed by ". rest"
	// Sentence rune count = 80 (79 é + 1 period), which is NOT < 80, so falls through.
	// Resulting truncation: 85-char total → truncate to 77+...
	multiSentence := ""
	for i := 0; i < 79; i++ {
		multiSentence += "é"
	}
	multiSentence += ". rest of step"
	got = stepName(multiSentence)
	if !utf8.ValidString(got) {
		t.Errorf("multi-byte sentence truncation produced invalid UTF-8: %q", got)
	}
}

func TestParseDurationMinutes(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"PT30M", 30},
		{"PT1H", 60},
		{"PT1H30M", 90},
		{"PT90S", 1},        // 90 seconds = 1 minute
		{"PT30S", 0},        // 30 seconds rounds down to 0 minutes
		{"PT2H30M45S", 150}, // 2h + 30m + 45s → 150m (45s/60 = 0 extra)
		{"PT2H30M90S", 151}, // 2h + 30m + 90s → 151m (90s/60 = 1 extra)
		{"PT15M90S", 16},    // 15m + 90s = 16m — this was 15 before the fix
		{"PT0S", 0},
		{"", 0},
		{"invalid", 0},
	}
	for _, c := range cases {
		got := parseDurationMinutes(c.input)
		if got != c.want {
			t.Errorf("parseDurationMinutes(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestComputeTotalTime(t *testing.T) {
	cases := []struct {
		d1, d2 string
		want   string
	}{
		{"PT15M", "PT30M", "PT45M"},
		{"PT1H", "PT30M", "PT1H30M"},
		{"PT30M", "PT30M", "PT1H"},
		{"PT15M90S", "PT30M", "PT46M"}, // 16m + 30m = 46m
		{"PT0S", "PT1H", "PT1H"},
	}
	for _, c := range cases {
		got := computeTotalTime(c.d1, c.d2)
		if got != c.want {
			t.Errorf("computeTotalTime(%q, %q) = %q, want %q", c.d1, c.d2, got, c.want)
		}
	}
}
