package render

import "testing"

func TestDurationMinutes(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"PT30M", 30},
		{"PT1H", 60},
		{"PT1H30M", 90},
		{"PT90S", 1},   // 90 seconds → 1 minute (truncated)
		{"PT30S", 0},   // 30 seconds → 0 minutes (truncated)
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
