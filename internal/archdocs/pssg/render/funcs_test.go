package render

import (
	"testing"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

func TestFormatNumber(t *testing.T) {
	cases := []struct {
		input interface{}
		want  string
	}{
		{100, "100"},
		{-100, "-100"}, // was producing "-,100" before fix
		{-999, "-999"}, // was producing "-,999" before fix
		{-99, "-99"},
		{1000, "1,000"},
		{-1000, "-1,000"},
		{1234567, "1,234,567"},
		{-1234567, "-1,234,567"},
		{0, "0"},
		{float64(1500), "1,500"},
	}
	for _, c := range cases {
		got := formatNumber(c.input)
		if got != c.want {
			t.Errorf("formatNumber(%v) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestDurationMinutes(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"PT30M", 30},
		{"PT1H", 60},
		{"PT1H30M", 90},
		{"PT90S", 1},
		{"PT30S", 0},
		{"PT2H30M45S", 150},
		{"PT10M30S", 10},
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

	got := sliceHelper(s, 0, 2).([]string)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("sliceHelper normal: got %v", got)
	}

	got = sliceHelper(s, 5, 10).([]string)
	if len(got) != 0 {
		t.Errorf("sliceHelper start>len: want empty, got %v", got)
	}

	got = sliceHelper(s, 2, 1).([]string)
	if len(got) != 0 {
		t.Errorf("sliceHelper start>end: want empty, got %v", got)
	}

	got = sliceHelper(s, -1, 2).([]string)
	if len(got) != 2 {
		t.Errorf("sliceHelper negative start: got %v", got)
	}
}

func TestFirstLast_EntitySlice(t *testing.T) {
	a := &entity.Entity{Slug: "a"}
	b := &entity.Entity{Slug: "b"}
	c := &entity.Entity{Slug: "c"}
	entities := []*entity.Entity{a, b, c}

	if got := first(entities); got != a {
		t.Errorf("first([]*entity.Entity) = %v, want %v", got, a)
	}
	if got := last(entities); got != c {
		t.Errorf("last([]*entity.Entity) = %v, want %v", got, c)
	}

	var empty []*entity.Entity
	if got := first(empty); got != nil {
		t.Errorf("first(empty []*entity.Entity) = %v, want nil", got)
	}
	if got := last(empty); got != nil {
		t.Errorf("last(empty []*entity.Entity) = %v, want nil", got)
	}
}

func TestSortStrings(t *testing.T) {
	input := []string{"c", "a", "b"}
	got := sortStrings(input)
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("sortStrings: got %v, want [a b c]", got)
	}
	if input[0] != "c" {
		t.Errorf("sortStrings modified original slice")
	}
}
