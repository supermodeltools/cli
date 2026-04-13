package entity

import (
	"testing"
)

// ── GetString ─────────────────────────────────────────────────────────────────

func TestGetString_Present(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"title": "My Recipe"}}
	if got := e.GetString("title"); got != "My Recipe" {
		t.Errorf("got %q, want %q", got, "My Recipe")
	}
}

func TestGetString_Missing(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{}}
	if got := e.GetString("missing"); got != "" {
		t.Errorf("missing key: got %q, want empty", got)
	}
}

func TestGetString_NonString(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"count": 42}}
	if got := e.GetString("count"); got != "" {
		t.Errorf("non-string: got %q, want empty", got)
	}
}

// ── GetStringSlice ────────────────────────────────────────────────────────────

func TestGetStringSlice_StringSlice(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"tags": []string{"a", "b"}}}
	got := e.GetStringSlice("tags")
	if len(got) != 2 || got[0] != "a" {
		t.Errorf("got %v", got)
	}
}

func TestGetStringSlice_InterfaceSlice(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"tags": []interface{}{"x", "y"}}}
	got := e.GetStringSlice("tags")
	if len(got) != 2 || got[1] != "y" {
		t.Errorf("got %v", got)
	}
}

func TestGetStringSlice_InterfaceSliceWithNonString(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"tags": []interface{}{"x", 42}}}
	got := e.GetStringSlice("tags")
	if len(got) != 1 || got[0] != "x" {
		t.Errorf("expected 1 item with 'x', got %v", got)
	}
}

func TestGetStringSlice_Missing(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{}}
	if got := e.GetStringSlice("tags"); got != nil {
		t.Errorf("missing key: got %v, want nil", got)
	}
}

func TestGetStringSlice_WrongType(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"tags": "string"}}
	if got := e.GetStringSlice("tags"); got != nil {
		t.Errorf("wrong type: got %v, want nil", got)
	}
}

// ── GetInt ────────────────────────────────────────────────────────────────────

func TestGetInt_Int(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"count": 5}}
	if got := e.GetInt("count"); got != 5 {
		t.Errorf("int: got %d, want 5", got)
	}
}

func TestGetInt_Int64(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"count": int64(10)}}
	if got := e.GetInt("count"); got != 10 {
		t.Errorf("int64: got %d, want 10", got)
	}
}

func TestGetInt_Float64(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"rating": float64(4)}}
	if got := e.GetInt("rating"); got != 4 {
		t.Errorf("float64: got %d, want 4", got)
	}
}

func TestGetInt_Missing(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{}}
	if got := e.GetInt("count"); got != 0 {
		t.Errorf("missing: got %d, want 0", got)
	}
}

func TestGetInt_WrongType(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"count": "five"}}
	if got := e.GetInt("count"); got != 0 {
		t.Errorf("wrong type: got %d, want 0", got)
	}
}

// ── GetFloat ──────────────────────────────────────────────────────────────────

func TestGetFloat_Float64(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"price": 3.14}}
	if got := e.GetFloat("price"); got != 3.14 {
		t.Errorf("float64: got %f", got)
	}
}

func TestGetFloat_Int(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"price": 3}}
	if got := e.GetFloat("price"); got != 3.0 {
		t.Errorf("int: got %f", got)
	}
}

func TestGetFloat_Int64(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"price": int64(7)}}
	if got := e.GetFloat("price"); got != 7.0 {
		t.Errorf("int64: got %f", got)
	}
}

func TestGetFloat_Missing(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{}}
	if got := e.GetFloat("price"); got != 0 {
		t.Errorf("missing: got %f", got)
	}
}

func TestGetFloat_WrongType(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"price": "cheap"}}
	if got := e.GetFloat("price"); got != 0 {
		t.Errorf("wrong type: got %f", got)
	}
}

// ── GetBool ───────────────────────────────────────────────────────────────────

func TestGetBool_True(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"active": true}}
	if got := e.GetBool("active"); !got {
		t.Error("expected true")
	}
}

func TestGetBool_False(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"active": false}}
	if got := e.GetBool("active"); got {
		t.Error("expected false")
	}
}

func TestGetBool_Missing(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{}}
	if got := e.GetBool("active"); got {
		t.Error("missing: expected false")
	}
}

func TestGetBool_WrongType(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"active": "yes"}}
	if got := e.GetBool("active"); got {
		t.Error("wrong type: expected false")
	}
}

// ── GetIngredients ────────────────────────────────────────────────────────────

func TestGetIngredients_Present(t *testing.T) {
	e := &Entity{Sections: map[string]interface{}{"ingredients": []string{"flour", "sugar"}}}
	got := e.GetIngredients()
	if len(got) != 2 || got[0] != "flour" {
		t.Errorf("got %v", got)
	}
}

func TestGetIngredients_Missing(t *testing.T) {
	e := &Entity{Sections: map[string]interface{}{}}
	if got := e.GetIngredients(); got != nil {
		t.Errorf("missing: got %v", got)
	}
}

func TestGetIngredients_WrongType(t *testing.T) {
	e := &Entity{Sections: map[string]interface{}{"ingredients": "string"}}
	if got := e.GetIngredients(); got != nil {
		t.Errorf("wrong type: got %v", got)
	}
}

// ── GetInstructions ───────────────────────────────────────────────────────────

func TestGetInstructions_Present(t *testing.T) {
	e := &Entity{Sections: map[string]interface{}{"instructions": []string{"mix", "bake"}}}
	got := e.GetInstructions()
	if len(got) != 2 {
		t.Errorf("got %v", got)
	}
}

func TestGetInstructions_Missing(t *testing.T) {
	e := &Entity{Sections: map[string]interface{}{}}
	if got := e.GetInstructions(); got != nil {
		t.Errorf("got %v", got)
	}
}

func TestGetInstructions_WrongType(t *testing.T) {
	e := &Entity{Sections: map[string]interface{}{"instructions": 42}}
	if got := e.GetInstructions(); got != nil {
		t.Errorf("wrong type: got %v", got)
	}
}

// ── GetFAQs ───────────────────────────────────────────────────────────────────

func TestGetFAQs_Present(t *testing.T) {
	e := &Entity{Sections: map[string]interface{}{"faqs": []FAQ{{Question: "Q?", Answer: "A."}}}}
	got := e.GetFAQs()
	if len(got) != 1 || got[0].Question != "Q?" {
		t.Errorf("got %v", got)
	}
}

func TestGetFAQs_Missing(t *testing.T) {
	e := &Entity{Sections: map[string]interface{}{}}
	if got := e.GetFAQs(); got != nil {
		t.Errorf("got %v", got)
	}
}

func TestGetFAQs_WrongType(t *testing.T) {
	e := &Entity{Sections: map[string]interface{}{"faqs": "not faqs"}}
	if got := e.GetFAQs(); got != nil {
		t.Errorf("wrong type: got %v", got)
	}
}

// ── HasField ──────────────────────────────────────────────────────────────────

func TestHasField_PresentNonEmpty(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"title": "Cake"}}
	if !e.HasField("title") {
		t.Error("expected true for non-empty string")
	}
}

func TestHasField_EmptyString(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"title": ""}}
	if e.HasField("title") {
		t.Error("expected false for empty string")
	}
}

func TestHasField_NonEmptySlice(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"tags": []interface{}{"a"}}}
	if !e.HasField("tags") {
		t.Error("expected true for non-empty []interface{}")
	}
}

func TestHasField_EmptyInterfaceSlice(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"tags": []interface{}{}}}
	if e.HasField("tags") {
		t.Error("expected false for empty []interface{}")
	}
}

func TestHasField_NonEmptyStringSlice(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"tags": []string{"a"}}}
	if !e.HasField("tags") {
		t.Error("expected true for non-empty []string")
	}
}

func TestHasField_EmptyStringSlice(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"tags": []string{}}}
	if e.HasField("tags") {
		t.Error("expected false for empty []string")
	}
}

func TestHasField_NilValue(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"x": nil}}
	if e.HasField("x") {
		t.Error("expected false for nil value")
	}
}

func TestHasField_OtherType(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{"count": 42}}
	if !e.HasField("count") {
		t.Error("expected true for int value (default case)")
	}
}

func TestHasField_Missing(t *testing.T) {
	e := &Entity{Fields: map[string]interface{}{}}
	if e.HasField("missing") {
		t.Error("expected false for missing key")
	}
}

// ── ToSlug ────────────────────────────────────────────────────────────────────

func TestToSlug_Basic(t *testing.T) {
	if got := ToSlug("Chocolate Cake!"); got != "chocolate-cake" {
		t.Errorf("got %q", got)
	}
}

func TestToSlug_AlreadySlug(t *testing.T) {
	if got := ToSlug("chocolate-cake"); got != "chocolate-cake" {
		t.Errorf("got %q", got)
	}
}

func TestToSlug_Numbers(t *testing.T) {
	if got := ToSlug("Recipe 42"); got != "recipe-42" {
		t.Errorf("got %q", got)
	}
}

func TestToSlug_TrimHyphens(t *testing.T) {
	if got := ToSlug("!!! title !!!"); got != "title" {
		t.Errorf("got %q", got)
	}
}

func TestToSlug_Empty(t *testing.T) {
	if got := ToSlug(""); got != "" {
		t.Errorf("got %q", got)
	}
}
