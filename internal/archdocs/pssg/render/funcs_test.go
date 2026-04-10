package render

import (
	"math"
	"strings"
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

// ── totalTime ─────────────────────────────────────────────────────────────────

func TestTotalTime(t *testing.T) {
	cases := []struct {
		d1, d2, want string
	}{
		{"PT30M", "PT30M", "PT1H"},
		{"PT1H", "PT30M", "PT1H30M"},
		{"PT45M", "PT20M", "PT1H5M"},
		{"PT10M", "PT5M", "PT15M"},
		{"PT2H", "PT0M", "PT2H"},
	}
	for _, c := range cases {
		got := totalTime(c.d1, c.d2)
		if got != c.want {
			t.Errorf("totalTime(%q, %q) = %q, want %q", c.d1, c.d2, got, c.want)
		}
	}
}

// ── formatDuration ────────────────────────────────────────────────────────────

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"PT30M", "30 min"},
		{"PT1H", "1 hr"},
		{"PT2H", "2 hrs"},
		{"PT1H30M", "1 hr 30 min"},
		{"PT90M", "1 hr 30 min"},
		{"PT0S", "PT0S"}, // 0 minutes → passthrough
		{"invalid", "invalid"},
	}
	for _, c := range cases {
		got := formatDuration(c.input)
		if got != c.want {
			t.Errorf("formatDuration(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── seq ───────────────────────────────────────────────────────────────────────

func TestSeq(t *testing.T) {
	got := seq(5)
	if len(got) != 5 {
		t.Fatalf("seq(5): len=%d", len(got))
	}
	for i, v := range got {
		if v != i+1 {
			t.Errorf("seq(5)[%d] = %d, want %d", i, v, i+1)
		}
	}
	if len(seq(0)) != 0 {
		t.Error("seq(0) should return empty slice")
	}
}

// ── dict ──────────────────────────────────────────────────────────────────────

func TestDict(t *testing.T) {
	m := dict("key1", "val1", "key2", 42)
	if m["key1"] != "val1" {
		t.Errorf("dict: key1=%v, want 'val1'", m["key1"])
	}
	if m["key2"] != 42 {
		t.Errorf("dict: key2=%v, want 42", m["key2"])
	}
	if len(dict()) != 0 {
		t.Error("dict() should return empty map")
	}
	// Odd number of args: last key gets no value, should not panic
	m2 := dict("orphan")
	if len(m2) != 0 {
		t.Errorf("dict with odd args: expected no entries, got %v", m2)
	}
}

// ── reverseStrings ────────────────────────────────────────────────────────────

func TestReverseStrings(t *testing.T) {
	got := reverseStrings([]string{"a", "b", "c"})
	if got[0] != "c" || got[1] != "b" || got[2] != "a" {
		t.Errorf("reverseStrings: got %v", got)
	}
	if len(reverseStrings(nil)) != 0 {
		t.Error("reverseStrings(nil) should return empty")
	}
}

// ── minInt / maxInt ───────────────────────────────────────────────────────────

func TestMinInt(t *testing.T) {
	if minInt(3, 5) != 3 {
		t.Error("minInt(3,5) should be 3")
	}
	if minInt(7, 2) != 2 {
		t.Error("minInt(7,2) should be 2")
	}
	if minInt(4, 4) != 4 {
		t.Error("minInt(4,4) should be 4")
	}
}

func TestMaxInt(t *testing.T) {
	if maxInt(3, 5) != 5 {
		t.Error("maxInt(3,5) should be 5")
	}
	if maxInt(7, 2) != 7 {
		t.Error("maxInt(7,2) should be 7")
	}
}

// ── length ────────────────────────────────────────────────────────────────────

func TestLength(t *testing.T) {
	if length([]string{"a", "b"}) != 2 {
		t.Error("[]string length")
	}
	if length(map[string]int{"x": 1}) != 1 {
		t.Error("map length")
	}
	if length("hello") != 5 {
		t.Error("string length")
	}
	if length(nil) != 0 {
		t.Error("nil length should be 0")
	}
	if length(42) != 0 {
		t.Error("int length should be 0")
	}
}

// ── Entity accessor functions ─────────────────────────────────────────────────

func newTestEntity(fields map[string]interface{}) *entity.Entity {
	return &entity.Entity{
		Fields:   fields,
		Sections: map[string]interface{}{},
	}
}

func TestFieldAccess_NilEntity(t *testing.T) {
	if fieldAccess(nil, "key") != nil {
		t.Error("fieldAccess(nil) should return nil")
	}
}

func TestFieldAccess_Present(t *testing.T) {
	e := newTestEntity(map[string]interface{}{"title": "My Recipe"})
	if got := fieldAccess(e, "title"); got != "My Recipe" {
		t.Errorf("fieldAccess: got %v", got)
	}
}

func TestSectionAccess_NilEntity(t *testing.T) {
	if sectionAccess(nil, "intro") != nil {
		t.Error("sectionAccess(nil) should return nil")
	}
}

func TestSectionAccess_Present(t *testing.T) {
	e := &entity.Entity{
		Fields:   map[string]interface{}{},
		Sections: map[string]interface{}{"ingredients": []string{"flour", "eggs"}},
	}
	v := sectionAccess(e, "ingredients")
	ss, ok := v.([]string)
	if !ok || len(ss) != 2 {
		t.Errorf("sectionAccess: got %v", v)
	}
}

func TestGetStringSlice_NilEntity(t *testing.T) {
	if getStringSlice(nil, "k") != nil {
		t.Error("getStringSlice(nil) should return nil")
	}
}

func TestGetStringSlice_Present(t *testing.T) {
	e := newTestEntity(map[string]interface{}{"tags": []string{"vegan", "quick"}})
	got := getStringSlice(e, "tags")
	if len(got) != 2 || got[0] != "vegan" {
		t.Errorf("getStringSlice: got %v", got)
	}
}

func TestHasField(t *testing.T) {
	e := newTestEntity(map[string]interface{}{"title": "X"})
	if !hasField(e, "title") {
		t.Error("hasField: should find 'title'")
	}
	if hasField(e, "missing") {
		t.Error("hasField: should not find 'missing'")
	}
	if hasField(nil, "title") {
		t.Error("hasField(nil): should return false")
	}
}

func TestGetInt(t *testing.T) {
	e := newTestEntity(map[string]interface{}{"servings": 4})
	if got := getInt(e, "servings"); got != 4 {
		t.Errorf("getInt: got %d", got)
	}
	if getInt(nil, "k") != 0 {
		t.Error("getInt(nil) should return 0")
	}
}

func TestGetFloat(t *testing.T) {
	e := newTestEntity(map[string]interface{}{"rating": float64(4.5)})
	if got := getFloat(e, "rating"); got != 4.5 {
		t.Errorf("getFloat: got %f", got)
	}
	if getFloat(nil, "k") != 0 {
		t.Error("getFloat(nil) should return 0")
	}
}

// ── jsonMarshal / toJSON ──────────────────────────────────────────────────────

func TestJsonMarshal(t *testing.T) {
	got := string(jsonMarshal(map[string]int{"x": 1}))
	if !strings.Contains(got, `"x":1`) {
		t.Errorf("jsonMarshal: got %q", got)
	}
}

func TestToJSON(t *testing.T) {
	got := toJSON([]string{"a", "b"})
	if got != `["a","b"]` {
		t.Errorf("toJSON: got %q", got)
	}
}

// ── defaultVal / ternary / hasKey ─────────────────────────────────────────────

func TestDefaultVal(t *testing.T) {
	if defaultVal("fallback", nil) != "fallback" {
		t.Error("nil should use default")
	}
	if defaultVal("fallback", "") != "fallback" {
		t.Error("empty string should use default")
	}
	if defaultVal("fallback", "value") != "value" {
		t.Error("non-empty should not use default")
	}
}

func TestTernary(t *testing.T) {
	if ternary(true, "yes", "no") != "yes" {
		t.Error("ternary(true): want 'yes'")
	}
	if ternary(false, "yes", "no") != "no" {
		t.Error("ternary(false): want 'no'")
	}
}

func TestHasKey(t *testing.T) {
	m := map[string]interface{}{"a": 1}
	if !hasKey(m, "a") {
		t.Error("hasKey: should find 'a'")
	}
	if hasKey(m, "b") {
		t.Error("hasKey: should not find 'b'")
	}
}

// ── parseQuantity ─────────────────────────────────────────────────────────────

func TestParseQuantity(t *testing.T) {
	cases := []struct {
		input string
		qty   float64
		rest  string
	}{
		{"2 cups flour", 2, "cups flour"},
		{"1 1/2 cups sugar", 1.5, "cups sugar"},
		{"1/2 tsp salt", 0.5, "tsp salt"},
		{"3 eggs", 3, "eggs"},
		{"", 0, ""},
		{"½ cup milk", 0.5, "cup milk"},
	}
	for _, c := range cases {
		qty, rest := parseQuantity(c.input)
		if math.Abs(qty-c.qty) > 0.01 {
			t.Errorf("parseQuantity(%q).qty = %f, want %f", c.input, qty, c.qty)
		}
		if rest != c.rest {
			t.Errorf("parseQuantity(%q).rest = %q, want %q", c.input, rest, c.rest)
		}
	}
}

// ── parseUnit ─────────────────────────────────────────────────────────────────

func TestParseUnit(t *testing.T) {
	cases := []struct {
		input, unit, rest string
	}{
		{"cups flour", "cup", "flour"},
		{"tsp salt", "teaspoon", "salt"},
		{"tablespoon oil", "tablespoon", "oil"},
		{"g butter", "gram", "butter"},
		{"eggs", "", "eggs"}, // no unit
		{"", "", ""},
	}
	for _, c := range cases {
		unit, rest := parseUnit(c.input)
		if unit != c.unit {
			t.Errorf("parseUnit(%q).unit = %q, want %q", c.input, unit, c.unit)
		}
		if rest != c.rest {
			t.Errorf("parseUnit(%q).rest = %q, want %q", c.input, rest, c.rest)
		}
	}
}

// ── parseIngredient* wrappers ─────────────────────────────────────────────────

func TestParseIngredientFunctions(t *testing.T) {
	line := "2 cups flour"
	if got := parseIngredientQty(line); math.Abs(got-2) > 0.01 {
		t.Errorf("parseIngredientQty(%q) = %f, want 2", line, got)
	}
	if got := parseIngredientUnit(line); got != "cup" {
		t.Errorf("parseIngredientUnit(%q) = %q, want 'cup'", line, got)
	}
	if got := parseIngredientDesc(line); got != "flour" {
		t.Errorf("parseIngredientDesc(%q) = %q, want 'flour'", line, got)
	}
}

// ── fractionDisplay ───────────────────────────────────────────────────────────

func TestFractionDisplay(t *testing.T) {
	cases := []struct {
		input float64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{2, "2"},
		{0.5, "½"},   // 0.5 is exactly ½
		{0.75, "¾"},  // 0.75 is exactly ¾
		{1.5, "1 ½"}, // whole + fraction
		{0.125, "⅛"},  // exactly ⅛
		{0.875, "⅞"},  // exactly ⅞
	}
	for _, c := range cases {
		got := fractionDisplay(c.input)
		if got != c.want {
			t.Errorf("fractionDisplay(%v) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── scaleQty ──────────────────────────────────────────────────────────────────

func TestScaleQty(t *testing.T) {
	// 1 cup base for 2 servings → scaled to 4 servings = 2 cups
	got := scaleQty(1.0, 2, 4)
	if got != "2" {
		t.Errorf("scaleQty(1.0, 2, 4) = %q, want '2'", got)
	}
	// zero base servings → returns fractionDisplay of base qty
	got = scaleQty(0.5, 0, 4)
	if got != "½" {
		t.Errorf("scaleQty(0.5, 0, 4) = %q, want '½'", got)
	}
}
