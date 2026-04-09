package taxonomy

import (
	"testing"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

// TestGroupByLetterASCII verifies that ASCII entry names are grouped correctly.
func TestGroupByLetterASCII(t *testing.T) {
	entries := []Entry{
		{Name: "Apple", Slug: "apple"},
		{Name: "Banana", Slug: "banana"},
		{Name: "avocado", Slug: "avocado"},
		{Name: "123numeric", Slug: "123numeric"},
	}
	groups := GroupByLetter(entries)

	letterMap := make(map[string][]string)
	for _, g := range groups {
		for _, e := range g.Entries {
			letterMap[g.Letter] = append(letterMap[g.Letter], e.Name)
		}
	}

	if len(letterMap["A"]) != 2 {
		t.Errorf("expected 2 entries under 'A', got %d: %v", len(letterMap["A"]), letterMap["A"])
	}
	if len(letterMap["B"]) != 1 {
		t.Errorf("expected 1 entry under 'B', got %d: %v", len(letterMap["B"]), letterMap["B"])
	}
	if len(letterMap["#"]) != 1 {
		t.Errorf("expected 1 entry under '#', got %d: %v", len(letterMap["#"]), letterMap["#"])
	}
}

// TestGroupByLetterNonASCII verifies that entries starting with multi-byte UTF-8
// characters are grouped under their correct letter, not the raw first byte value.
// Before the fix, "Étoile" was grouped under 'Ã' (the Latin-1 misread of 0xC3,
// the first byte of É's UTF-8 encoding), not 'E'/'É'.
func TestGroupByLetterNonASCII(t *testing.T) {
	entries := []Entry{
		{Name: "Étoile", Slug: "etoile"}, // É is U+00C9, encoded as 0xC3 0x89
		{Name: "Ñoño", Slug: "nono"},     // Ñ is U+00D1, encoded as 0xC3 0x91
		{Name: "Über", Slug: "uber"},     // Ü is U+00DC, encoded as 0xC3 0x9C
		{Name: "English", Slug: "english"},
	}
	groups := GroupByLetter(entries)

	letterMap := make(map[string][]string)
	for _, g := range groups {
		for _, e := range g.Entries {
			letterMap[g.Letter] = append(letterMap[g.Letter], e.Name)
		}
	}

	// None of the non-ASCII entries should be grouped under 'Ã' (the buggy value).
	if names, ok := letterMap["Ã"]; ok {
		t.Errorf("non-ASCII entries were incorrectly grouped under 'Ã' (raw byte 0xC3 misread): %v", names)
	}

	// Étoile should be under 'É', Ñoño under 'Ñ', Über under 'Ü', English under 'E'.
	if _, ok := letterMap["É"]; !ok {
		t.Errorf("expected 'Étoile' under letter 'É', got groups: %v", letterMap)
	}
	if _, ok := letterMap["Ñ"]; !ok {
		t.Errorf("expected 'Ñoño' under letter 'Ñ', got groups: %v", letterMap)
	}
	if _, ok := letterMap["Ü"]; !ok {
		t.Errorf("expected 'Über' under letter 'Ü', got groups: %v", letterMap)
	}
	if _, ok := letterMap["E"]; !ok {
		t.Errorf("expected 'English' under letter 'E', got groups: %v", letterMap)
	}
}

// TestGroupByLetterEmpty verifies that empty entry names are skipped.
func TestGroupByLetterEmpty(t *testing.T) {
	entries := []Entry{
		{Name: "", Slug: "empty"},
		{Name: "Apple", Slug: "apple"},
	}
	groups := GroupByLetter(entries)
	total := 0
	for _, g := range groups {
		total += len(g.Entries)
	}
	if total != 1 {
		t.Errorf("expected 1 entry (empty name skipped), got %d", total)
	}
}

// TestTopEntries verifies that TopEntries returns entries sorted by entity count.
func TestTopEntries(t *testing.T) {
	e := func(n int) []*entity.Entity {
		s := make([]*entity.Entity, n)
		for i := range s {
			s[i] = &entity.Entity{}
		}
		return s
	}
	entries := []Entry{
		{Name: "A", Slug: "a", Entities: e(3)},
		{Name: "B", Slug: "b", Entities: e(10)},
		{Name: "C", Slug: "c", Entities: e(1)},
	}
	top := TopEntries(entries, 2)
	if len(top) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(top))
	}
	if top[0].Name != "B" || top[1].Name != "A" {
		t.Errorf("wrong order: got [%s, %s], want [B, A]", top[0].Name, top[1].Name)
	}
	// original slice must not be modified
	if entries[0].Name != "A" {
		t.Errorf("original slice was modified")
	}
}
