package render

import (
	"testing"
)

func TestDurationMinutes(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"PT30M", 30},
		{"PT1H", 60},
		{"PT1H30M", 90},
		{"PT90S", 1},        // 90 seconds → 1 minute (truncated)
		{"PT30S", 0},        // 30 seconds → 0 minutes (truncated)
		{"PT2H30M45S", 150}, // seconds truncated
		{"PT10M30S", 10},    // 10 min 30 sec → 10 min
		{"PT0S", 0},
		{"", 0},
		{"invalid", 0},
	}
	for _, c := range cases {
		got := durationMinutes(c.input)
		if got != c.want {
			t.Errorf("durationMinutes(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestSliceHelper(t *testing.T) {
	s := []string{"a", "b", "c"}

	// Normal case
	got := sliceHelper(s, 0, 2).([]string)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("sliceHelper normal: got %v", got)
	}

	// start > len(v) — must not panic, return empty
	got = sliceHelper(s, 5, 10).([]string)
	if len(got) != 0 {
		t.Errorf("sliceHelper start>len: want empty, got %v", got)
	}

	// start > end after clamping — must not panic
	got = sliceHelper(s, 2, 1).([]string)
	if len(got) != 0 {
		t.Errorf("sliceHelper start>end: want empty, got %v", got)
	}

	// negative start
	got = sliceHelper(s, -1, 2).([]string)
	if len(got) != 2 {
		t.Errorf("sliceHelper negative start: got %v", got)
	}
}

func TestSortStrings(t *testing.T) {
	input := []string{"c", "a", "b"}
	got := sortStrings(input)
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("sortStrings: got %v, want [a b c]", got)
	}
	// Original must not be modified
	if input[0] != "c" {
		t.Errorf("sortStrings modified original slice")
	}
}
