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

// ── toStringSlice ─────────────────────────────────────────────────────────────

func TestToStringSlice(t *testing.T) {
	if got := toStringSlice([]string{"a", "b"}); len(got) != 2 || got[0] != "a" {
		t.Errorf("[]string: got %v", got)
	}
	if got := toStringSlice("single"); len(got) != 1 || got[0] != "single" {
		t.Errorf("string: got %v", got)
	}
	if got := toStringSlice([]interface{}{"x", "y"}); len(got) != 2 || got[0] != "x" {
		t.Errorf("[]interface{}: got %v", got)
	}
	if got := toStringSlice(42); got != nil {
		t.Errorf("int: want nil, got %v", got)
	}
}

// ── HubPageURL ────────────────────────────────────────────────────────────────

func TestHubPageURL(t *testing.T) {
	if got := HubPageURL("cuisine", "italian", 1); got != "/cuisine/italian.html" {
		t.Errorf("page 1: got %q", got)
	}
	if got := HubPageURL("cuisine", "italian", 2); got != "/cuisine/italian-page-2.html" {
		t.Errorf("page 2: got %q", got)
	}
	if got := HubPageURL("cuisine", "italian", 10); got != "/cuisine/italian-page-10.html" {
		t.Errorf("page 10: got %q", got)
	}
}

// ── LetterPageURL ─────────────────────────────────────────────────────────────

func TestLetterPageURL(t *testing.T) {
	if got := LetterPageURL("cuisine", "A"); got != "/cuisine/letter-a.html" {
		t.Errorf("A: got %q", got)
	}
	if got := LetterPageURL("cuisine", "#"); got != "/cuisine/letter-num.html" {
		t.Errorf("#: got %q", got)
	}
	if got := LetterPageURL("tags", "Z"); got != "/tags/letter-z.html" {
		t.Errorf("Z: got %q", got)
	}
}

// ── FindEntry ─────────────────────────────────────────────────────────────────

func TestFindEntry(t *testing.T) {
	tx := &Taxonomy{
		Entries: []Entry{
			{Slug: "italian", Name: "Italian"},
			{Slug: "french", Name: "French"},
		},
	}
	e := tx.FindEntry("french")
	if e == nil {
		t.Fatal("FindEntry('french') returned nil")
	}
	if e.Name != "French" {
		t.Errorf("FindEntry('french').Name = %q, want 'French'", e.Name)
	}
	if tx.FindEntry("japanese") != nil {
		t.Error("FindEntry for unknown slug should return nil")
	}
}

// ── ComputePagination ─────────────────────────────────────────────────────────

func TestComputePagination_SinglePage(t *testing.T) {
	e := func(n int) []*entity.Entity {
		s := make([]*entity.Entity, n)
		for i := range s {
			s[i] = &entity.Entity{}
		}
		return s
	}
	entry := Entry{Slug: "italian", Entities: e(5)}
	info := ComputePagination(entry, 1, 20, "cuisine")
	if info.TotalPages != 1 {
		t.Errorf("TotalPages: got %d, want 1", info.TotalPages)
	}
	if info.TotalItems != 5 {
		t.Errorf("TotalItems: got %d, want 5", info.TotalItems)
	}
	if info.PrevURL != "" {
		t.Error("page 1 should have no PrevURL")
	}
	if info.NextURL != "" {
		t.Error("single page should have no NextURL")
	}
}

func TestComputePagination_MultiPage(t *testing.T) {
	e := func(n int) []*entity.Entity {
		s := make([]*entity.Entity, n)
		for i := range s {
			s[i] = &entity.Entity{}
		}
		return s
	}
	entry := Entry{Slug: "italian", Entities: e(10)}
	info := ComputePagination(entry, 2, 4, "cuisine")
	if info.TotalPages != 3 {
		t.Errorf("TotalPages: got %d, want 3", info.TotalPages)
	}
	if info.CurrentPage != 2 {
		t.Errorf("CurrentPage: got %d, want 2", info.CurrentPage)
	}
	if info.PrevURL != "/cuisine/italian.html" {
		t.Errorf("PrevURL: got %q", info.PrevURL)
	}
	if info.NextURL != "/cuisine/italian-page-3.html" {
		t.Errorf("NextURL: got %q", info.NextURL)
	}
	if len(info.PageURLs) != 3 {
		t.Errorf("PageURLs: got %d entries", len(info.PageURLs))
	}
}

func TestComputePagination_Empty(t *testing.T) {
	entry := Entry{Slug: "empty"}
	info := ComputePagination(entry, 1, 20, "cuisine")
	if info.TotalPages != 1 {
		t.Errorf("empty entries: TotalPages should be 1, got %d", info.TotalPages)
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
