package schema

import (
	"testing"
)

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
