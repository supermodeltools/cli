package render

import (
	"testing"
	"unicode/utf8"
)

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
