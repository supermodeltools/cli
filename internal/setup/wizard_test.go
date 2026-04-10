package setup

import (
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
