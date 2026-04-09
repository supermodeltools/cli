package schema

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStepName_ShortStep(t *testing.T) {
	short := "Mix well."
	if got := stepName(short); got != short {
		t.Errorf("stepName(%q) = %q, want unchanged", short, got)
	}
}

func TestStepName_FirstSentence(t *testing.T) {
	step := "Sauté the onions. Then add garlic and cook for 2 more minutes."
	got := stepName(step)
	if got != "Sauté the onions." {
		t.Errorf("stepName first-sentence: got %q", got)
	}
}

func TestStepName_TruncatesLongASCII(t *testing.T) {
	long := strings.Repeat("a", 81)
	got := stepName(long)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("long step should end with '...', got %q", got)
	}
	if len([]rune(got)) > 80 {
		t.Errorf("truncated step too long: %d runes", len([]rune(got)))
	}
}

// TestStepName_MultiByteUTF8 is the regression test for the byte-based slicing
// bug: a step whose byte length exceeds 80 but rune count is near the boundary
// must produce valid UTF-8 when truncated.
func TestStepName_MultiByteUTF8(t *testing.T) {
	// "é" is 2 bytes. 75 ASCII chars + 4 "é" = 75 + 8 = 83 bytes > 80,
	// but only 79 runes — just under the limit.
	// A step with 81 runes of "é" should be truncated to 77 "é" + "...".
	long := strings.Repeat("é", 81) // 162 bytes, 81 runes
	got := stepName(long)
	if !utf8.ValidString(got) {
		t.Errorf("stepName output contains invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("long multi-byte step should be truncated with '...', got %q", got)
	}
	if strings.Contains(got, strings.Repeat("é", 81)) {
		t.Errorf("long multi-byte step should be truncated, not returned in full")
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
