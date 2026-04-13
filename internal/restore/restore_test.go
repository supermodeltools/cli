package restore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/supermodeltools/cli/internal/api"
)

// ── CountTokens ───────────────────────────────────────────────────────────────

func TestCountTokens_Empty(t *testing.T) {
	if got := CountTokens(""); got != 0 {
		t.Errorf("empty string: want 0, got %d", got)
	}
}

func TestCountTokens_ShortWord(t *testing.T) {
	// "hi" → 2 chars / 4 = 0 charEstimate; 1 word * 100/75 = 1 wordEstimate → max = 1
	if got := CountTokens("hi"); got < 1 {
		t.Errorf("'hi': want >=1, got %d", got)
	}
}

func TestCountTokens_CharHeavy(t *testing.T) {
	// 100 chars with no spaces → charEstimate = 25, wordEstimate = 100/75 = 1 → 25
	noSpaces := strings.Repeat("a", 100)
	if got := CountTokens(noSpaces); got != 25 {
		t.Errorf("100 no-space chars: want 25, got %d", got)
	}
}

func TestCountTokens_WordHeavy(t *testing.T) {
	// 75 single-char words → wordEstimate = 75*100/75 = 100; charEstimate = (75*2-1)/4 ≈ 37 → 100
	words := strings.Repeat("a ", 75) // 75 words, 150 chars
	got := CountTokens(words)
	if got != 100 {
		t.Errorf("75 single-char words: want 100, got %d", got)
	}
}

func TestCountTokens_RealText(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog"
	got := CountTokens(text)
	// 9 words → 9*100/75 = 12; 43 chars → 43/4 = 10 → 12
	if got < 10 {
		t.Errorf("real text: want >=10 tokens, got %d", got)
	}
}

func TestCountTokens_MultiByteChars(t *testing.T) {
	// Prior bug: used len(text)/4 (bytes) not RuneCountInString/4.
	// Each CJK character is 3 bytes; 100 of them = 300 bytes but only 100 runes.
	// charEstimate must be 100/4 = 25, not 300/4 = 75.
	cjk := strings.Repeat("中", 100) // 100 runes, 300 bytes
	got := CountTokens(cjk)
	// charEstimate = 25, wordEstimate = 1*100/75 = 1 → 25
	if got != 25 {
		t.Errorf("100 CJK chars: want 25 tokens, got %d (byte-based would give 75)", got)
	}
}

// ── isHorizontalRule ─────────────────────────────────────────────────────────

func TestIsHorizontalRule(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"---", true},
		{"***", true},
		{"___", true},
		{"- - -", true},
		{"* * *", true},
		{"----", true},
		{"", false},
		{"--", false},
		{"abc", false},
		{"# heading", false},
		{"-*-", false},  // mixed characters
		{"- -a", false}, // non-separator character
	}
	for _, tt := range tests {
		if got := isHorizontalRule(tt.line); got != tt.want {
			t.Errorf("isHorizontalRule(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

// ── entryPointPriority ────────────────────────────────────────────────────────

func TestEntryPointPriority(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{"main.go", 4},
		{"cmd/main.go", 4},
		{"app.go", 3},
		{"application.go", 3},
		{"server.go", 2},
		{"index.go", 2},
		{"init.go", 1},
		{"__init__.py", 1},
		{"handler.go", 0},
		{"utils.go", 0},
		{"README.md", 0},
		{"MAIN.GO", 4}, // case-insensitive
		{"App.ts", 3},
		{"Server.js", 2},
	}
	for _, tt := range tests {
		if got := entryPointPriority(tt.path); got != tt.want {
			t.Errorf("entryPointPriority(%q) = %d, want %d", tt.path, got, tt.want)
		}
	}
}

// ── detectLanguages ───────────────────────────────────────────────────────────

func TestDetectLanguages_Empty(t *testing.T) {
	primary, langs := detectLanguages(map[string]int{})
	if primary != "" || len(langs) != 0 {
		t.Errorf("empty: want ('', []), got (%q, %v)", primary, langs)
	}
}

func TestDetectLanguages_SingleExtension(t *testing.T) {
	primary, langs := detectLanguages(map[string]int{".go": 10})
	if primary != "Go" {
		t.Errorf("primary: want Go, got %q", primary)
	}
	if len(langs) != 1 || langs[0] != "Go" {
		t.Errorf("langs: want [Go], got %v", langs)
	}
}

func TestDetectLanguages_MultipleExtensionsSameLanguage(t *testing.T) {
	// .ts and .tsx both map to TypeScript — should aggregate
	primary, langs := detectLanguages(map[string]int{".ts": 5, ".tsx": 3})
	if primary != "TypeScript" {
		t.Errorf("primary: want TypeScript, got %q", primary)
	}
	if len(langs) != 1 || langs[0] != "TypeScript" {
		t.Errorf("langs: want [TypeScript], got %v", langs)
	}
}

func TestDetectLanguages_SortedByCountDesc(t *testing.T) {
	_, langs := detectLanguages(map[string]int{
		".go": 1,
		".py": 10,
		".rs": 5,
	})
	if len(langs) != 3 {
		t.Fatalf("want 3, got %d", len(langs))
	}
	if langs[0] != "Python" {
		t.Errorf("first should be Python (count=10), got %q", langs[0])
	}
	if langs[1] != "Rust" {
		t.Errorf("second should be Rust (count=5), got %q", langs[1])
	}
}

func TestDetectLanguages_AlphaTieBreak(t *testing.T) {
	// Equal counts → alphabetical
	_, langs := detectLanguages(map[string]int{".go": 5, ".rs": 5})
	if len(langs) != 2 {
		t.Fatalf("want 2, got %d", len(langs))
	}
	if langs[0] != "Go" {
		t.Errorf("alpha tie: first should be Go, got %q", langs[0])
	}
}

func TestDetectLanguages_CapAt5(t *testing.T) {
	counts := map[string]int{
		".go": 10, ".py": 9, ".rs": 8, ".js": 7, ".ts": 6, ".rb": 5,
	}
	_, langs := detectLanguages(counts)
	if len(langs) > 5 {
		t.Errorf("cap at 5: want <=5, got %d: %v", len(langs), langs)
	}
}

func TestDetectLanguages_UnknownExtensionsIgnored(t *testing.T) {
	_, langs := detectLanguages(map[string]int{".xyz": 100, ".go": 1})
	if len(langs) != 1 || langs[0] != "Go" {
		t.Errorf("unknown extensions ignored: want [Go], got %v", langs)
	}
}

// ── buildDomains ──────────────────────────────────────────────────────────────

func TestBuildDomains_Empty(t *testing.T) {
	if domains := buildDomains(map[string][]string{}); len(domains) != 0 {
		t.Errorf("empty map: want [], got %v", domains)
	}
}

func TestBuildDomains_SortedByFileCountDesc(t *testing.T) {
	dirFiles := map[string][]string{
		"small": {"a.go"},
		"large": {"a.go", "b.go", "c.go"},
		"mid":   {"a.go", "b.go"},
	}
	domains := buildDomains(dirFiles)
	if len(domains) != 3 {
		t.Fatalf("want 3, got %d", len(domains))
	}
	if domains[0].Name != "large" {
		t.Errorf("first should be large (3 files), got %q", domains[0].Name)
	}
	if domains[1].Name != "mid" {
		t.Errorf("second should be mid (2 files), got %q", domains[1].Name)
	}
}

func TestBuildDomains_AlphaTieBreak(t *testing.T) {
	dirFiles := map[string][]string{
		"z_dir": {"a.go"},
		"a_dir": {"b.go"},
	}
	domains := buildDomains(dirFiles)
	if domains[0].Name != "a_dir" {
		t.Errorf("alpha tie: first should be a_dir, got %q", domains[0].Name)
	}
}

func TestBuildDomains_CapAt20(t *testing.T) {
	dirFiles := make(map[string][]string)
	for i := 0; i < 25; i++ {
		dirFiles[strings.Repeat("x", i+1)] = []string{"file.go"}
	}
	domains := buildDomains(dirFiles)
	if len(domains) > 20 {
		t.Errorf("cap at 20: want <=20, got %d", len(domains))
	}
}

func TestBuildDomains_KeyFilesSortedByPriority(t *testing.T) {
	dirFiles := map[string][]string{
		"cmd": {"handler.go", "main.go", "utils.go"},
	}
	domains := buildDomains(dirFiles)
	if len(domains) != 1 {
		t.Fatalf("want 1 domain, got %d", len(domains))
	}
	if domains[0].KeyFiles[0] != "main.go" {
		t.Errorf("main.go should be first key file, got %q", domains[0].KeyFiles[0])
	}
}

func TestBuildDomains_CapAt8KeyFiles(t *testing.T) {
	files := make([]string, 12)
	for i := range files {
		files[i] = strings.Repeat("a", i+1) + ".go"
	}
	dirFiles := map[string][]string{"big": files}
	domains := buildDomains(dirFiles)
	if len(domains[0].KeyFiles) > 8 {
		t.Errorf("cap at 8 key files: want <=8, got %d", len(domains[0].KeyFiles))
	}
}

func TestBuildDomains_RootFilesGroupedEmpty(t *testing.T) {
	// Files at root level (parts[0] == "") map to the "" dir key → domain named "Root"
	dirFiles := map[string][]string{
		"": {"README.md", "main.go"},
	}
	domains := buildDomains(dirFiles)
	if len(domains) != 1 {
		t.Fatalf("want 1 domain, got %d", len(domains))
	}
	if domains[0].Name != "Root" {
		t.Errorf("empty dir key should map to 'Root', got %q", domains[0].Name)
	}
}

func TestBuildDomains_DescriptionHasFileCount(t *testing.T) {
	dirFiles := map[string][]string{
		"api": {"a.go", "b.go", "c.go"},
	}
	domains := buildDomains(dirFiles)
	if !strings.Contains(domains[0].Description, "3") {
		t.Errorf("description should mention file count, got %q", domains[0].Description)
	}
}

// ── localTopFiles ─────────────────────────────────────────────────────────────

func TestLocalTopFiles_Empty(t *testing.T) {
	if files := localTopFiles(nil, 10); len(files) != 0 {
		t.Errorf("nil domains: want [], got %v", files)
	}
}

func TestLocalTopFiles_CapAtN(t *testing.T) {
	domains := []Domain{{
		KeyFiles: []string{"a.go", "b.go", "c.go", "d.go", "e.go"},
	}}
	files := localTopFiles(domains, 3)
	if len(files) != 3 {
		t.Errorf("cap at 3: want 3, got %d", len(files))
	}
}

func TestLocalTopFiles_DeduplicatesAcrossDomains(t *testing.T) {
	domains := []Domain{
		{KeyFiles: []string{"main.go", "handler.go"}},
		{KeyFiles: []string{"main.go", "service.go"}},
	}
	files := localTopFiles(domains, 10)
	seen := make(map[string]int)
	for _, f := range files {
		seen[f.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("duplicate file %q (appeared %d times)", path, count)
		}
	}
	if len(files) != 3 {
		t.Errorf("3 unique files across 2 domains: want 3, got %d", len(files))
	}
}

func TestLocalTopFiles_EntryPointsFirst(t *testing.T) {
	domains := []Domain{{
		KeyFiles: []string{"utils.go", "handler.go", "main.go"},
	}}
	files := localTopFiles(domains, 10)
	if files[0].Path != "main.go" {
		t.Errorf("main.go should be first, got %q", files[0].Path)
	}
}

func TestLocalTopFiles_RelationshipCountIsZero(t *testing.T) {
	domains := []Domain{{KeyFiles: []string{"main.go"}}}
	files := localTopFiles(domains, 10)
	if files[0].RelationshipCount != 0 {
		t.Errorf("local mode: RelationshipCount should be 0, got %d", files[0].RelationshipCount)
	}
}

// ── computeCriticalFiles ──────────────────────────────────────────────────────

func TestComputeCriticalFiles_Empty(t *testing.T) {
	if files := computeCriticalFiles(nil, 10); len(files) != 0 {
		t.Errorf("nil domains: want [], got %v", files)
	}
}

func TestComputeCriticalFiles_NZero(t *testing.T) {
	domains := []Domain{{KeyFiles: []string{"shared.go"}}, {KeyFiles: []string{"shared.go"}}}
	if files := computeCriticalFiles(domains, 0); files != nil {
		t.Errorf("n=0: want nil, got %v", files)
	}
}

func TestComputeCriticalFiles_SingleDomainNotCritical(t *testing.T) {
	domains := []Domain{{KeyFiles: []string{"a.go", "b.go"}}}
	// All files only in 1 domain — none are "critical" in the cross-domain sense
	// computeCriticalFiles returns ALL files, just sorted by domain reference count
	files := computeCriticalFiles(domains, 10)
	for _, f := range files {
		if f.RelationshipCount != 1 {
			t.Errorf("single domain file should have count=1, got %d for %s", f.RelationshipCount, f.Path)
		}
	}
}

func TestComputeCriticalFiles_CrossDomainCounts(t *testing.T) {
	domains := []Domain{
		{Name: "auth", KeyFiles: []string{"shared.go", "auth.go"}},
		{Name: "billing", KeyFiles: []string{"shared.go", "billing.go"}},
		{Name: "api", KeyFiles: []string{"shared.go"}},
	}
	files := computeCriticalFiles(domains, 10)
	// shared.go appears 3 times
	var sharedCount int
	for _, f := range files {
		if f.Path == "shared.go" {
			sharedCount = f.RelationshipCount
		}
	}
	if sharedCount != 3 {
		t.Errorf("shared.go: want RelationshipCount=3, got %d", sharedCount)
	}
}

func TestComputeCriticalFiles_SortedByCountDescThenPath(t *testing.T) {
	domains := []Domain{
		{KeyFiles: []string{"a.go", "b.go", "c.go"}},
		{KeyFiles: []string{"a.go", "b.go"}},
		{KeyFiles: []string{"a.go"}},
	}
	files := computeCriticalFiles(domains, 10)
	if files[0].Path != "a.go" || files[0].RelationshipCount != 3 {
		t.Errorf("first should be a.go×3, got %+v", files[0])
	}
	if files[1].Path != "b.go" || files[1].RelationshipCount != 2 {
		t.Errorf("second should be b.go×2, got %+v", files[1])
	}
}

func TestComputeCriticalFiles_DedupWithinDomain(t *testing.T) {
	domains := []Domain{
		{KeyFiles: []string{"shared.go", "shared.go"}},
		{KeyFiles: []string{"shared.go"}},
	}
	files := computeCriticalFiles(domains, 10)
	var count int
	for _, f := range files {
		if f.Path == "shared.go" {
			count = f.RelationshipCount
		}
	}
	if count != 2 {
		t.Errorf("dedup within domain: want count=2, got %d", count)
	}
}

func TestComputeCriticalFiles_CapAtN(t *testing.T) {
	// 15 domains each with same shared file + unique file → shared.go count=15
	// Plus 14 unique files each with count=1
	domains := make([]Domain, 15)
	for i := range domains {
		domains[i] = Domain{KeyFiles: []string{"shared.go", strings.Repeat("z", i+1) + ".go"}}
	}
	files := computeCriticalFiles(domains, 5)
	if len(files) > 5 {
		t.Errorf("cap at 5: want <=5, got %d", len(files))
	}
}

// ── buildDomainSection ────────────────────────────────────────────────────────

func TestBuildDomainSection_Basic(t *testing.T) {
	d := &Domain{Name: "API", Description: "2 file(s)"}
	got := buildDomainSection(d)
	if !strings.Contains(got, "### API") {
		t.Errorf("should contain domain name heading, got:\n%s", got)
	}
	if !strings.Contains(got, "2 file(s)") {
		t.Errorf("should contain description, got:\n%s", got)
	}
}

func TestBuildDomainSection_WithKeyFiles(t *testing.T) {
	d := &Domain{
		Name:     "auth",
		KeyFiles: []string{"auth/handler.go", "auth/service.go"},
	}
	got := buildDomainSection(d)
	if !strings.Contains(got, "auth/handler.go") {
		t.Errorf("should contain key files, got:\n%s", got)
	}
}

func TestBuildDomainSection_WithResponsibilities(t *testing.T) {
	d := &Domain{
		Name:             "auth",
		Responsibilities: []string{"Login", "Logout"},
	}
	got := buildDomainSection(d)
	if !strings.Contains(got, "Login") {
		t.Errorf("should contain responsibilities, got:\n%s", got)
	}
}

func TestBuildDomainSection_WithSubdomains(t *testing.T) {
	d := &Domain{
		Name: "auth",
		Subdomains: []Subdomain{
			{Name: "OAuth", Description: "OAuth2 flow"},
			{Name: "Sessions"},
		},
	}
	got := buildDomainSection(d)
	if !strings.Contains(got, "OAuth") || !strings.Contains(got, "OAuth2 flow") {
		t.Errorf("should contain subdomain with description, got:\n%s", got)
	}
	if !strings.Contains(got, "Sessions") {
		t.Errorf("should contain subdomain without description, got:\n%s", got)
	}
}

func TestBuildDomainSection_WithDependsOn(t *testing.T) {
	d := &Domain{
		Name:      "billing",
		DependsOn: []string{"api", "storage"},
	}
	got := buildDomainSection(d)
	if !strings.Contains(got, "Depends on") {
		t.Errorf("should contain DependsOn, got:\n%s", got)
	}
	if !strings.Contains(got, "api") || !strings.Contains(got, "storage") {
		t.Errorf("should list dependencies, got:\n%s", got)
	}
}

// ── FromSupermodelIR ──────────────────────────────────────────────────────────

func TestFromSupermodelIR_Basic(t *testing.T) {
	ir := &api.SupermodelIR{
		Summary: map[string]any{
			"filesProcessed": float64(100),
			"functions":      float64(500),
		},
		Metadata: api.IRMetadata{Languages: []string{"Go", "TypeScript"}},
		Domains: []api.IRDomain{
			{
				Name:               "Authentication",
				DescriptionSummary: "Auth flows",
				KeyFiles:           []string{"auth/handler.go"},
				Responsibilities:   []string{"Login"},
				Subdomains:         []api.IRSubdomain{{Name: "OAuth", DescriptionSummary: "OAuth2"}},
			},
		},
		Graph: api.IRGraph{
			Nodes: []api.IRNode{{Type: "ExternalDependency", Name: "cobra"}},
		},
	}

	g := FromSupermodelIR(ir, "myproject")

	if g.Name != "myproject" {
		t.Errorf("name: got %q", g.Name)
	}
	if g.Language != "Go" {
		t.Errorf("language: got %q", g.Language)
	}
	if g.Stats.TotalFiles != 100 {
		t.Errorf("total files: got %d", g.Stats.TotalFiles)
	}
	if g.Stats.TotalFunctions != 500 {
		t.Errorf("total functions: got %d", g.Stats.TotalFunctions)
	}
	if len(g.Domains) != 1 || g.Domains[0].Name != "Authentication" {
		t.Errorf("domains: got %v", g.Domains)
	}
	if len(g.Domains[0].Subdomains) != 1 || g.Domains[0].Subdomains[0].Name != "OAuth" {
		t.Errorf("subdomains: got %v", g.Domains[0].Subdomains)
	}
	if len(g.ExternalDeps) != 1 || g.ExternalDeps[0] != "cobra" {
		t.Errorf("external deps: got %v", g.ExternalDeps)
	}
}

func TestFromSupermodelIR_PrimaryLanguageFromSummaryOverridesMetadata(t *testing.T) {
	ir := &api.SupermodelIR{
		Summary:  map[string]any{"primaryLanguage": "TypeScript"},
		Metadata: api.IRMetadata{Languages: []string{"Go"}},
	}
	g := FromSupermodelIR(ir, "proj")
	if g.Language != "TypeScript" {
		t.Errorf("Summary primaryLanguage should override Metadata: got %q", g.Language)
	}
}

func TestFromSupermodelIR_PrimaryLanguageFromMetadata(t *testing.T) {
	ir := &api.SupermodelIR{
		Metadata: api.IRMetadata{Languages: []string{"Rust", "C"}},
	}
	g := FromSupermodelIR(ir, "proj")
	if g.Language != "Rust" {
		t.Errorf("first metadata language: got %q", g.Language)
	}
}

func TestFromSupermodelIR_DomainRelatesMapsToDepensOn(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{{Name: "billing"}},
		Graph: api.IRGraph{
			Relationships: []api.IRRelationship{
				{Type: "DOMAIN_RELATES", Source: "billing", Target: "api"},
				{Type: "DOMAIN_RELATES", Source: "billing", Target: "storage"},
			},
		},
	}
	g := FromSupermodelIR(ir, "proj")
	billing := g.Domains[0]
	if len(billing.DependsOn) != 2 {
		t.Fatalf("billing should depend on 2 domains, got %v", billing.DependsOn)
	}
}

func TestFromSupermodelIR_SubdomainConversion(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{{
			Name: "auth",
			Subdomains: []api.IRSubdomain{
				{Name: "OAuth", DescriptionSummary: "OAuth2 flow"},
				{Name: "Sessions"},
			},
		}},
	}
	g := FromSupermodelIR(ir, "proj")
	subs := g.Domains[0].Subdomains
	if len(subs) != 2 {
		t.Fatalf("want 2 subdomains, got %d", len(subs))
	}
	if subs[0].Description != "OAuth2 flow" {
		t.Errorf("subdomain description: got %q", subs[0].Description)
	}
}

func TestFromSupermodelIR_CriticalFilesComputed(t *testing.T) {
	ir := &api.SupermodelIR{
		Domains: []api.IRDomain{
			{Name: "a", KeyFiles: []string{"shared.go"}},
			{Name: "b", KeyFiles: []string{"shared.go"}},
		},
	}
	g := FromSupermodelIR(ir, "proj")
	if len(g.CriticalFiles) != 1 || g.CriticalFiles[0].Path != "shared.go" {
		t.Errorf("critical files: got %v", g.CriticalFiles)
	}
	if g.CriticalFiles[0].RelationshipCount != 2 {
		t.Errorf("critical file count: want 2, got %d", g.CriticalFiles[0].RelationshipCount)
	}
}

func TestFromSupermodelIR_Empty(t *testing.T) {
	g := FromSupermodelIR(&api.SupermodelIR{}, "empty")
	if g == nil {
		t.Fatal("returned nil")
		return
	}
	if g.Name != "empty" {
		t.Errorf("name: got %q", g.Name)
	}
}

// ── Render ────────────────────────────────────────────────────────────────────

func TestRender_NilGraphReturnsError(t *testing.T) {
	_, _, err := Render(nil, "proj", RenderOptions{MaxTokens: 2000})
	if err == nil {
		t.Error("nil graph should return error")
	}
}

func TestRender_SmallGraphFitsBudget(t *testing.T) {
	g := &ProjectGraph{
		Name:     "myproject",
		Language: "Go",
		Stats:    Stats{TotalFiles: 10, TotalFunctions: 50},
	}
	output, tokens, err := Render(g, "myproject", RenderOptions{MaxTokens: 5000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "myproject") {
		t.Error("output should contain project name")
	}
	if tokens > 5000 {
		t.Errorf("tokens %d exceeds budget 5000", tokens)
	}
	if tokens <= 0 {
		t.Error("token count should be positive")
	}
}

func TestRender_TokenCountMatchesOutput(t *testing.T) {
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Stats:    Stats{TotalFiles: 5},
	}
	output, tokens, err := Render(g, "proj", RenderOptions{MaxTokens: 5000})
	if err != nil {
		t.Fatal(err)
	}
	measured := CountTokens(output)
	if tokens != measured {
		t.Errorf("returned token count %d doesn't match measured %d", tokens, measured)
	}
}

func TestRender_DefaultMaxTokensApplied(t *testing.T) {
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Stats:    Stats{TotalFiles: 5},
	}
	_, tokens, err := Render(g, "proj", RenderOptions{MaxTokens: 0}) // 0 → use default
	if err != nil {
		t.Fatal(err)
	}
	if tokens > DefaultMaxTokens {
		t.Errorf("with default budget, tokens %d exceed DefaultMaxTokens %d", tokens, DefaultMaxTokens)
	}
}

func TestRender_LargeDomainListTruncated(t *testing.T) {
	// Build a graph that would exceed a tiny budget
	domains := make([]Domain, 30)
	for i := range domains {
		domains[i] = Domain{
			Name:        strings.Repeat("x", 20),
			Description: strings.Repeat("y", 50),
			KeyFiles:    []string{"file1.go", "file2.go", "file3.go"},
			Responsibilities: []string{
				strings.Repeat("responsibility text ", 5),
				strings.Repeat("responsibility text ", 5),
			},
		}
	}
	g := &ProjectGraph{
		Name:     "bigproject",
		Language: "Go",
		Domains:  domains,
		Stats:    Stats{TotalFiles: 500, TotalFunctions: 1000},
	}
	output, tokens, err := Render(g, "bigproject", RenderOptions{MaxTokens: 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens > 250 { // allow small overshoot due to estimation
		t.Errorf("truncation: tokens %d should be close to budget 200", tokens)
	}
	if len(output) == 0 {
		t.Error("truncated output should not be empty")
	}
}

func TestRender_ClaudeMDIncluded(t *testing.T) {
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Stats:    Stats{TotalFiles: 1},
	}
	opts := RenderOptions{
		MaxTokens: 5000,
		ClaudeMD:  "## Instructions\nDo the thing.",
	}
	output, _, err := Render(g, "proj", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Do the thing.") {
		t.Error("output should include ClaudeMD content")
	}
}

func TestRender_LocalModeBanner(t *testing.T) {
	g := &ProjectGraph{Name: "proj", Language: "Go", Stats: Stats{TotalFiles: 1}}
	output, _, err := Render(g, "proj", RenderOptions{MaxTokens: 5000, LocalMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "local mode") {
		t.Error("output should contain local mode banner")
	}
}

func TestRender_CircularDepsWarning(t *testing.T) {
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Stats:    Stats{TotalFiles: 10, CircularDependencyCycles: 2},
		Cycles: []CircularDependencyCycle{
			{Cycle: []string{"auth", "billing"}},
			{Cycle: []string{"api", "storage"}},
		},
	}
	output, _, err := Render(g, "proj", RenderOptions{MaxTokens: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "circular dependency") {
		t.Error("output should warn about circular dependencies")
	}
}

func TestRender_CyclesCappedAt10(t *testing.T) {
	cycles := make([]CircularDependencyCycle, 15)
	for i := range cycles {
		cycles[i] = CircularDependencyCycle{Cycle: []string{"a", "b"}}
	}
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Stats:    Stats{CircularDependencyCycles: 15},
		Cycles:   cycles,
	}
	output, _, err := Render(g, "proj", RenderOptions{MaxTokens: 5000})
	if err != nil {
		t.Fatal(err)
	}
	// Should mention "5 more" (15 - 10 = 5)
	if !strings.Contains(output, "5 more") {
		t.Errorf("should mention extra cycles count, got:\n%s", output)
	}
}

func TestRender_ContainsProjectOverview(t *testing.T) {
	g := &ProjectGraph{
		Name:        "awesomeproject",
		Language:    "TypeScript",
		Framework:   "Next.js",
		Description: "A web framework",
		Stats:       Stats{TotalFiles: 42, TotalFunctions: 150},
	}
	output, _, err := Render(g, "awesomeproject", RenderOptions{MaxTokens: 5000})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"awesomeproject", "TypeScript", "Next.js", "42", "150"} {
		if !strings.Contains(output, want) {
			t.Errorf("should contain %q, got:\n%s", want, output)
		}
	}
}

// TestRender_LanguageList covers the languageList FuncMap lambda (L21) by supplying
// Stats.Languages which is rendered via {{languageList .Graph.Stats.Languages}}.
func TestRender_LanguageList(t *testing.T) {
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Stats:    Stats{TotalFiles: 5, Languages: []string{"Go", "TypeScript"}},
	}
	output, _, err := Render(g, "proj", RenderOptions{MaxTokens: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "TypeScript") {
		t.Errorf("languageList should render languages: %s", output)
	}
}

// TestRender_CriticalFilesAdd1 covers the add1 FuncMap lambda (L22) by including
// CriticalFiles which are rendered with {{add1 $i}} for 1-based numbering.
func TestRender_CriticalFilesAdd1(t *testing.T) {
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Stats:    Stats{TotalFiles: 5},
		CriticalFiles: []CriticalFile{
			{Path: "core/db.go", RelationshipCount: 8},
		},
	}
	output, _, err := Render(g, "proj", RenderOptions{MaxTokens: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "core/db.go") {
		t.Errorf("critical file should appear: %s", output)
	}
}

// TestRender_StaleWithStaleAt covers L99-101: staleDuration computed when
// opts.Stale=true and opts.StaleAt is non-nil inside Render().
func TestRender_StaleWithStaleAt(t *testing.T) {
	staleAt := time.Now().Add(-3 * time.Hour)
	g := &ProjectGraph{Name: "proj", Language: "Go", Stats: Stats{TotalFiles: 1}}
	output, _, err := Render(g, "proj", RenderOptions{
		MaxTokens: 5000,
		Stale:     true,
		StaleAt:   &staleAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "STALE") {
		t.Errorf("output should contain STALE banner: %s", output)
	}
}

// ── truncateToTokenBudget ─────────────────────────────────────────────────────

func TestTruncateToTokenBudget_TinyBudgetFallback(t *testing.T) {
	g := &ProjectGraph{Name: "proj", Language: "Go", Stats: Stats{TotalFiles: 1}}
	output, _, _ := truncateToTokenBudget(g, "proj", RenderOptions{MaxTokens: 2})
	if !strings.Contains(output, "Budget too small") {
		t.Errorf("tiny budget should produce fallback, got:\n%s", output)
	}
}

func TestTruncateToTokenBudget_HeaderAlwaysPresent(t *testing.T) {
	g := &ProjectGraph{Name: "myproj", Language: "Rust", Stats: Stats{TotalFiles: 5}}
	output, _, err := truncateToTokenBudget(g, "myproj", RenderOptions{MaxTokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "myproj") {
		t.Error("header should always include project name")
	}
	if !strings.Contains(output, "Rust") {
		t.Error("header should include language")
	}
}

func TestTruncateToTokenBudget_RespectsTokenBudget(t *testing.T) {
	domains := make([]Domain, 10)
	for i := range domains {
		domains[i] = Domain{
			Name:        strings.Repeat("d", 20),
			Description: strings.Repeat("x", 100),
		}
	}
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Domains:  domains,
		Stats:    Stats{TotalFiles: 100},
	}
	budget := 300
	output, tokens, err := truncateToTokenBudget(g, "proj", RenderOptions{MaxTokens: budget})
	if err != nil {
		t.Fatal(err)
	}
	if tokens > budget+20 { // small tolerance for estimation
		t.Errorf("tokens %d should not exceed budget %d (±20)", tokens, budget)
	}
	if len(output) == 0 {
		t.Error("output should not be empty")
	}
}

// ── ReadClaudeMD ──────────────────────────────────────────────────────────────

func TestReadClaudeMD_NoFile(t *testing.T) {
	dir := t.TempDir()
	if got := ReadClaudeMD(dir); got != "" {
		t.Errorf("missing file: want '', got %q", got)
	}
}

func TestReadClaudeMD_ShortFile(t *testing.T) {
	dir := t.TempDir()
	content := "## Build\nRun `go build ./...`"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got := ReadClaudeMD(dir)
	if got != content {
		t.Errorf("short file: want %q, got %q", content, got)
	}
}

func TestReadClaudeMD_LongFileTruncated(t *testing.T) {
	dir := t.TempDir()
	// Write >3000 runes
	content := strings.Repeat("a", 4000)
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got := ReadClaudeMD(dir)
	if !strings.Contains(got, "truncated") {
		t.Errorf("long file: should contain truncation marker, got (len=%d)", len(got))
	}
	if len([]rune(got)) > 3100 { // 3000 + truncation notice
		t.Errorf("truncated content too long: %d runes", len([]rune(got)))
	}
}

func TestReadClaudeMD_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("   "), 0600); err != nil {
		t.Fatal(err)
	}
	// TrimSpace makes "" → ReadClaudeMD returns ""
	if got := ReadClaudeMD(dir); got != "" {
		t.Errorf("whitespace-only file: want '', got %q", got)
	}
}

// ── DetectExternalDeps ────────────────────────────────────────────────────────

func TestDetectExternalDeps_NoManifests(t *testing.T) {
	dir := t.TempDir()
	if deps := DetectExternalDeps(dir); len(deps) != 0 {
		t.Errorf("no manifests: want [], got %v", deps)
	}
}

func TestDetectExternalDeps_GoMod(t *testing.T) {
	dir := t.TempDir()
	gomod := `module github.com/myorg/myapp

go 1.21

require (
	github.com/spf13/cobra v1.8.0
	github.com/spf13/viper v1.18.0
)

require github.com/stretchr/testify v1.8.0
`
	writeFile(t, dir, "go.mod", gomod)
	deps := DetectExternalDeps(dir)
	if !contains(deps, "cobra") {
		t.Errorf("should include cobra, got %v", deps)
	}
	if !contains(deps, "viper") {
		t.Errorf("should include viper, got %v", deps)
	}
	if !contains(deps, "testify") {
		t.Errorf("should include testify, got %v", deps)
	}
	// Own module path should not appear
	if contains(deps, "myapp") {
		t.Errorf("own module should be excluded, got %v", deps)
	}
}

func TestDetectExternalDeps_GoModOwnModuleExcluded(t *testing.T) {
	dir := t.TempDir()
	gomod := `module github.com/myorg/myapp

go 1.21

require github.com/myorg/myapp v0.0.0
`
	writeFile(t, dir, "go.mod", gomod)
	deps := DetectExternalDeps(dir)
	if contains(deps, "myapp") {
		t.Errorf("own module should be excluded, got %v", deps)
	}
}

func TestDetectExternalDeps_PackageJSON(t *testing.T) {
	dir := t.TempDir()
	pkg := `{
		"dependencies": { "react": "^18.0.0", "axios": "^1.0.0" },
		"devDependencies": { "jest": "^29.0.0", "typescript": "^5.0.0" }
	}`
	writeFile(t, dir, "package.json", pkg)
	deps := DetectExternalDeps(dir)
	if !contains(deps, "react") {
		t.Errorf("should include react (runtime), got %v", deps)
	}
	if !contains(deps, "axios") {
		t.Errorf("should include axios (runtime), got %v", deps)
	}
	// Dev deps should also be included (if budget allows)
	if !contains(deps, "jest") {
		t.Errorf("should include jest (dev), got %v", deps)
	}
}

func TestDetectExternalDeps_RequirementsTxt(t *testing.T) {
	dir := t.TempDir()
	req := `# requirements
requests>=2.28.0
flask==2.3.0
sqlalchemy[asyncio]>=2.0
django
# comment
`
	writeFile(t, dir, "requirements.txt", req)
	deps := DetectExternalDeps(dir)
	if !contains(deps, "requests") {
		t.Errorf("should include requests, got %v", deps)
	}
	if !contains(deps, "flask") {
		t.Errorf("should include flask, got %v", deps)
	}
	if !contains(deps, "sqlalchemy") {
		t.Errorf("should include sqlalchemy (extras stripped), got %v", deps)
	}
	if !contains(deps, "django") {
		t.Errorf("should include django, got %v", deps)
	}
}

func TestDetectExternalDeps_CargoToml(t *testing.T) {
	dir := t.TempDir()
	cargo := `[package]
name = "myapp"
version = "0.1.0"

[dependencies]
serde = { version = "1.0", features = ["derive"] }
tokio = "1.0"

[dev-dependencies]
criterion = "0.5"
`
	writeFile(t, dir, "Cargo.toml", cargo)
	deps := DetectExternalDeps(dir)
	if !contains(deps, "serde") {
		t.Errorf("should include serde, got %v", deps)
	}
	if !contains(deps, "tokio") {
		t.Errorf("should include tokio, got %v", deps)
	}
	if !contains(deps, "criterion") {
		t.Errorf("should include criterion (dev dep), got %v", deps)
	}
}

func TestDetectExternalDeps_Gemfile(t *testing.T) {
	dir := t.TempDir()
	gemfile := `source 'https://rubygems.org'

gem 'rails', '~> 7.0'
gem 'devise'
gem "pundit", "~> 2.3"
`
	writeFile(t, dir, "Gemfile", gemfile)
	deps := DetectExternalDeps(dir)
	if !contains(deps, "rails") {
		t.Errorf("should include rails, got %v", deps)
	}
	if !contains(deps, "devise") {
		t.Errorf("should include devise, got %v", deps)
	}
	if !contains(deps, "pundit") {
		t.Errorf("should include pundit (double quotes), got %v", deps)
	}
}

func TestDetectExternalDeps_PyprojectTomlPoetry(t *testing.T) {
	dir := t.TempDir()
	pyproject := `[tool.poetry]
name = "myapp"

[tool.poetry.dependencies]
python = "^3.11"
fastapi = "^0.100.0"
pydantic = "^2.0"
`
	writeFile(t, dir, "pyproject.toml", pyproject)
	deps := DetectExternalDeps(dir)
	if contains(deps, "python") {
		t.Errorf("python itself should be excluded, got %v", deps)
	}
	if !contains(deps, "fastapi") {
		t.Errorf("should include fastapi, got %v", deps)
	}
	if !contains(deps, "pydantic") {
		t.Errorf("should include pydantic, got %v", deps)
	}
}

func TestDetectExternalDeps_PyprojectTomlProjectSection(t *testing.T) {
	// Tests the [project] section with a multi-line dependencies array.
	dir := t.TempDir()
	pyproject := `[project]
name = "myapp"
dependencies = [
    "requests>=2.0",
    "pydantic",
    "fastapi ; python_version>='3.8'",
]
`
	writeFile(t, dir, "pyproject.toml", pyproject)
	deps := DetectExternalDeps(dir)
	for _, want := range []string{"requests", "pydantic", "fastapi"} {
		if !contains(deps, want) {
			t.Errorf("should include %q, got %v", want, deps)
		}
	}
}

func TestDetectExternalDeps_PyprojectTomlProjectInlineArray(t *testing.T) {
	// Tests the [project] section with an inline array on one line.
	dir := t.TempDir()
	pyproject := `[project]
name = "myapp"
dependencies = ["requests", "pydantic"]
`
	writeFile(t, dir, "pyproject.toml", pyproject)
	deps := DetectExternalDeps(dir)
	for _, want := range []string{"requests", "pydantic"} {
		if !contains(deps, want) {
			t.Errorf("should include %q in inline array, got %v", want, deps)
		}
	}
}

func TestDetectExternalDeps_NpmDevDepsFillRemainingCapacity(t *testing.T) {
	// 14 non-npm deps from requirements.txt + 1 npm runtime + 2 npm dev;
	// only 1 slot remains after non-npm, so only 1 npm runtime should be added.
	dir := t.TempDir()
	lines := make([]string, 14)
	for i := range lines {
		lines[i] = "dep" + strings.Repeat("x", i+1)
	}
	writeFile(t, dir, "requirements.txt", strings.Join(lines, "\n"))
	writeFile(t, dir, "package.json", `{"dependencies":{"npm-a":"^1.0"},"devDependencies":{"npm-dev":"^1.0"}}`)
	deps := DetectExternalDeps(dir)
	if len(deps) > 15 {
		t.Errorf("should cap at 15, got %d: %v", len(deps), deps)
	}
}

func TestDetectExternalDeps_CapAt15(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = strings.Repeat("dep", i+1)
	}
	writeFile(t, dir, "requirements.txt", strings.Join(lines, "\n"))
	deps := DetectExternalDeps(dir)
	if len(deps) > 15 {
		t.Errorf("cap at 15: got %d", len(deps))
	}
}

// ── BuildProjectGraph ─────────────────────────────────────────────────────────

func TestBuildProjectGraph_BasicGoProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "handler.go", "package main\n\nfunc handler() {}\n")
	writeFile(t, dir, "go.mod", "module example.com/hello\n\ngo 1.21\n\nrequire github.com/spf13/cobra v1.8.0\n")

	ctx := context.Background()
	g, err := BuildProjectGraph(ctx, dir, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Name != "hello" {
		t.Errorf("name: got %q", g.Name)
	}
	if g.Language != "Go" {
		t.Errorf("language: want Go, got %q", g.Language)
	}
	if g.Stats.TotalFiles < 2 {
		t.Errorf("total files: want >=2, got %d", g.Stats.TotalFiles)
	}
}

func TestBuildProjectGraph_DetectsLanguage(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.py", "b.py", "c.py", "d.ts"} {
		writeFile(t, dir, f, "# code\n")
	}
	g, err := BuildProjectGraph(context.Background(), dir, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if g.Language != "Python" {
		t.Errorf("3 py files vs 1 ts: want Python, got %q", g.Language)
	}
}

func TestBuildProjectGraph_BuildsDomains(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "api"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "internal", "auth"), 0750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "internal/api/client.go", "package api\n")
	writeFile(t, dir, "internal/auth/handler.go", "package auth\n")
	writeFile(t, dir, "main.go", "package main\n")

	g, err := BuildProjectGraph(context.Background(), dir, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Domains) == 0 {
		t.Error("should have at least one domain")
	}
}

func TestBuildProjectGraph_DetectsDepsFromGoMod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	writeFile(t, dir, "go.mod", "module example.com/hi\n\ngo 1.21\n\nrequire github.com/spf13/cobra v1.8.0\n")

	g, err := BuildProjectGraph(context.Background(), dir, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(g.ExternalDeps, "cobra") {
		t.Errorf("should detect cobra from go.mod, got %v", g.ExternalDeps)
	}
}

func TestBuildProjectGraph_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := BuildProjectGraph(ctx, dir, "proj")
	if err == nil {
		t.Error("cancelled context should return error")
	}
}

func TestBuildProjectGraph_ReadsREADMEDescription(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	writeFile(t, dir, "README.md", "# My Project\n\nA simple command-line tool for data processing.\n")

	g, err := BuildProjectGraph(context.Background(), dir, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g.Description, "command-line") {
		t.Errorf("should extract description from README, got %q", g.Description)
	}
}

// ── collectFiles edge cases ───────────────────────────────────────────────────

// TestBuildProjectGraph_NonExistentRoot covers L325-327: WalkDir calls the
// callback with a non-nil error for the root directory when it does not exist.
func TestBuildProjectGraph_NonExistentRoot(t *testing.T) {
	ctx := context.Background()
	_, err := BuildProjectGraph(ctx, "/nonexistent-dir-for-collectfiles-test-xyz", "proj")
	if err == nil {
		t.Error("BuildProjectGraph should fail for a non-existent root directory")
	}
}

// TestBuildProjectGraph_HiddenAndIgnoredDirs covers L335-337: a hidden directory
// and an ignoreDirs entry (node_modules) are both skipped during the walk.
func TestBuildProjectGraph_HiddenAndIgnoredDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	// Hidden dir (starts with "."): should be skipped.
	if err := os.MkdirAll(filepath.Join(dir, ".hidden_dir"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, ".hidden_dir/secret.go", "package hidden\n")
	// ignoreDirs entry (node_modules): should be skipped.
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "node_modules/pkg.js", "x\n")

	ctx := context.Background()
	g, err := BuildProjectGraph(ctx, dir, "proj")
	if err != nil {
		t.Fatal(err)
	}
	// Only main.go should be counted; hidden and ignored dirs must not add files.
	if g.Stats.TotalFiles != 1 {
		t.Errorf("want 1 file (hidden and node_modules skipped), got %d", g.Stats.TotalFiles)
	}
}

// TestBuildProjectGraph_SymlinkSkipped covers L340-342: a symlink entry in the
// walk is silently skipped.
func TestBuildProjectGraph_SymlinkSkipped(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "main.go")
	writeFile(t, dir, "main.go", "package main\n")
	link := filepath.Join(dir, "link.go")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink creation not supported: " + err.Error())
	}

	ctx := context.Background()
	g, err := BuildProjectGraph(ctx, dir, "proj")
	if err != nil {
		t.Fatal(err)
	}
	// The symlink must not be counted as a separate file.
	if g.Stats.TotalFiles != 1 {
		t.Errorf("want 1 file (symlink skipped), got %d", g.Stats.TotalFiles)
	}
}

// TestBuildProjectGraph_HiddenFileSkipped covers L348-350: a hidden file
// (starting with ".") in the walk is silently skipped.
func TestBuildProjectGraph_HiddenFileSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\n")
	writeFile(t, dir, ".hidden_file", "not a source file\n")

	ctx := context.Background()
	g, err := BuildProjectGraph(ctx, dir, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if g.Stats.TotalFiles != 1 {
		t.Errorf("want 1 file (.hidden_file skipped), got %d", g.Stats.TotalFiles)
	}
}

// ── DetectExternalDeps edge cases ─────────────────────────────────────────────

// TestDetectExternalDeps_DuplicateDep covers L99-101: the seen[name] check in the
// add closure skips an already-added dependency.
func TestDetectExternalDeps_DuplicateDep(t *testing.T) {
	dir := t.TempDir()
	// Same dep listed under two different top-level require statements → add called
	// twice with "cobra", second call hits seen[name] == true branch.
	writeFile(t, dir, "go.mod", "module example.com/x\n\nrequire github.com/spf13/cobra v1.0.0\nrequire github.com/spf13/cobra v1.8.0\n")
	deps := DetectExternalDeps(dir)
	count := 0
	for _, d := range deps {
		if d == "cobra" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("duplicate dep 'cobra' should appear once, got %d times in %v", count, deps)
	}
}

// TestDetectExternalDeps_GoModInlineComment covers L132-134: a require block entry
// with an inline // comment has the comment stripped before the module name is parsed.
func TestDetectExternalDeps_GoModInlineComment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", `module example.com/x

go 1.21

require (
	github.com/spf13/cobra v1.8.0 // indirect
	github.com/pkg/errors v0.9.0
)
`)
	deps := DetectExternalDeps(dir)
	if !contains(deps, "cobra") {
		t.Errorf("should detect cobra from require block with // comment, got %v", deps)
	}
}

// TestDetectExternalDeps_RequirementsURLSpec covers L175-177: a requirements.txt
// line using the "name @ URL" PEP 440 URL specifier strips the URL part.
func TestDetectExternalDeps_RequirementsURLSpec(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "requirements.txt", "requests @ https://files.pythonhosted.org/requests.tar.gz\nflask\n")
	deps := DetectExternalDeps(dir)
	if !contains(deps, "requests") {
		t.Errorf("should detect 'requests' from URL spec line, got %v", deps)
	}
}

// TestDetectExternalDeps_CargoNegativeDepth covers L206-208: a rogue "}" in
// Cargo.toml when depth is already 0 would make depth negative; the guard resets
// it to 0 so subsequent lines are still processed correctly.
func TestDetectExternalDeps_CargoNegativeDepth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", `[package]
name = "myapp"

[dependencies]
serde = "1.0"
}
anyhow = "1.0"
`)
	deps := DetectExternalDeps(dir)
	if !contains(deps, "serde") {
		t.Errorf("should detect 'serde' despite rogue }, got %v", deps)
	}
}

// TestDetectExternalDeps_NpmRuntimeCapAtMaxDeps covers L294-295: when deps is
// already at maxDeps (15) from non-npm sources, the npmRuntime loop breaks
// immediately at L294.
func TestDetectExternalDeps_NpmRuntimeCapAtMaxDeps(t *testing.T) {
	dir := t.TempDir()
	// 15 requirements.txt deps to fill the cap, plus one npm runtime dep.
	pyDeps := make([]string, 15)
	for i := range pyDeps {
		pyDeps[i] = "pydep" + strings.Repeat("x", i+1)
	}
	writeFile(t, dir, "requirements.txt", strings.Join(pyDeps, "\n"))
	writeFile(t, dir, "package.json", `{"dependencies":{"npm-extra":"^1.0"}}`)
	deps := DetectExternalDeps(dir)
	// Cap must be respected.
	if len(deps) > 15 {
		t.Errorf("deps should be capped at 15, got %d", len(deps))
	}
	// npm-extra should not appear because the cap was already hit.
	if contains(deps, "npm-extra") {
		t.Errorf("npm-extra should be excluded due to cap, got %v", deps)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// ── humanDuration ─────────────────────────────────────────────────────────────

func TestHumanDuration_Seconds(t *testing.T) {
	got := humanDuration(30 * time.Second)
	if !strings.Contains(got, "seconds") {
		t.Errorf("want 'seconds', got %q", got)
	}
	if !strings.Contains(got, "30") {
		t.Errorf("want '30', got %q", got)
	}
}

func TestHumanDuration_Minutes(t *testing.T) {
	got := humanDuration(5 * time.Minute)
	if !strings.Contains(got, "minutes") {
		t.Errorf("want 'minutes', got %q", got)
	}
	if !strings.Contains(got, "5") {
		t.Errorf("want '5', got %q", got)
	}
}

func TestHumanDuration_Hours(t *testing.T) {
	got := humanDuration(3 * time.Hour)
	if !strings.Contains(got, "hours") {
		t.Errorf("want 'hours', got %q", got)
	}
}

func TestHumanDuration_Days(t *testing.T) {
	got := humanDuration(48 * time.Hour)
	if !strings.Contains(got, "days") {
		t.Errorf("want 'days', got %q", got)
	}
	if !strings.Contains(got, "2") {
		t.Errorf("want '2', got %q", got)
	}
}

func TestHumanDuration_JustUnderMinute(t *testing.T) {
	got := humanDuration(59 * time.Second)
	if !strings.Contains(got, "seconds") {
		t.Errorf("59s should be seconds, got %q", got)
	}
}

// ── cleanPyDep ────────────────────────────────────────────────────────────────

func TestCleanPyDep_PlainName(t *testing.T) {
	if got := cleanPyDep("requests"); got != "requests" {
		t.Errorf("plain name: got %q", got)
	}
}

func TestCleanPyDep_VersionConstraint(t *testing.T) {
	for _, input := range []string{"requests>=2.0", "requests==2.28.0", "requests<=3.0", "requests!=1.0", "requests~=2.0", "requests>2", "requests<3"} {
		got := cleanPyDep(input)
		if got != "requests" {
			t.Errorf("cleanPyDep(%q) = %q, want 'requests'", input, got)
		}
	}
}

func TestCleanPyDep_InlineComment(t *testing.T) {
	got := cleanPyDep("requests>=2.0 # http library")
	if got != "requests" {
		t.Errorf("inline comment: got %q, want 'requests'", got)
	}
}

func TestCleanPyDep_Extras(t *testing.T) {
	got := cleanPyDep("requests[security]>=2.0")
	if got != "requests" {
		t.Errorf("extras: got %q, want 'requests'", got)
	}
}

func TestCleanPyDep_Semicolon(t *testing.T) {
	got := cleanPyDep("requests>=2.0;python_version>='3.6'")
	if got != "requests" {
		t.Errorf("semicolon: got %q, want 'requests'", got)
	}
}

func TestCleanPyDep_WithWhitespace(t *testing.T) {
	got := cleanPyDep("  requests  ")
	if got != "requests" {
		t.Errorf("whitespace: got %q, want 'requests'", got)
	}
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}

// ── truncateToTokenBudget additional branches ─────────────────────────────────

func TestTruncateToTokenBudget_CriticalFilesWithRelCount(t *testing.T) {
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Stats:    Stats{TotalFiles: 5},
		CriticalFiles: []CriticalFile{
			{Path: "core/db.go", RelationshipCount: 10},
			{Path: "util/helpers.go", RelationshipCount: 0},
		},
	}
	output, _, err := truncateToTokenBudget(g, "proj", RenderOptions{MaxTokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Critical Files") {
		t.Error("should contain Critical Files section")
	}
	if !strings.Contains(output, "relationships") {
		t.Errorf("file with RelationshipCount>0 should show 'relationships': %s", output)
	}
}

func TestTruncateToTokenBudget_CriticalFilesTruncatedByBudget(t *testing.T) {
	// Very tight budget: Critical Files header fits but individual file lines don't.
	files := make([]CriticalFile, 5)
	for i := range files {
		files[i] = CriticalFile{
			Path:              strings.Repeat("a", 80),
			RelationshipCount: i + 1,
		}
	}
	g := &ProjectGraph{
		Name:          "proj",
		Language:      "Go",
		Stats:         Stats{TotalFiles: 5},
		CriticalFiles: files,
	}
	// Small enough that not all files fit
	_, tokens, err := truncateToTokenBudget(g, "proj", RenderOptions{MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if tokens > 130 {
		t.Errorf("tokens %d should stay close to budget 100", tokens)
	}
}

func TestTruncateToTokenBudget_StaleBanner(t *testing.T) {
	staleAt := time.Now().Add(-2 * time.Hour)
	g := &ProjectGraph{Name: "proj", Language: "Go", Stats: Stats{TotalFiles: 1}}
	opts := RenderOptions{
		MaxTokens: 500,
		Stale:     true,
		StaleAt:   &staleAt,
	}
	output, _, err := truncateToTokenBudget(g, "proj", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "STALE") {
		t.Errorf("stale output should contain 'STALE': %s", output)
	}
}

// TestTruncateToTokenBudget_LocalMode covers L156-158: local mode banner appended
// in truncateToTokenBudget when opts.LocalMode is true.
func TestTruncateToTokenBudget_LocalMode(t *testing.T) {
	g := &ProjectGraph{Name: "proj", Language: "Go", Stats: Stats{TotalFiles: 1}}
	output, _, err := truncateToTokenBudget(g, "proj", RenderOptions{
		MaxTokens: 500,
		LocalMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "local mode") {
		t.Errorf("output should contain local mode banner: %s", output)
	}
}

// TestTruncateToTokenBudget_CircularOneCycle covers L163-168: the
// CircularDependencyCycles>0 branch and the ==1 singular "cycle" label.
func TestTruncateToTokenBudget_CircularOneCycle(t *testing.T) {
	g := &ProjectGraph{
		Name:     "proj",
		Language: "Go",
		Stats:    Stats{TotalFiles: 1, CircularDependencyCycles: 1},
	}
	output, _, err := truncateToTokenBudget(g, "proj", RenderOptions{MaxTokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "circular dependency") {
		t.Errorf("output should mention circular dependency: %s", output)
	}
	// Singular "cycle" (not "cycles") is used when count == 1.
	if !strings.Contains(output, "cycle") {
		t.Errorf("singular 'cycle' should appear for count=1: %s", output)
	}
}

// TestTruncateToTokenBudget_ClaudeMD covers L231-240: ClaudeMD section written
// when opts.ClaudeMD is non-empty and fits within remaining budget.
func TestTruncateToTokenBudget_ClaudeMD(t *testing.T) {
	g := &ProjectGraph{Name: "proj", Language: "Go", Stats: Stats{TotalFiles: 1}}
	output, _, err := truncateToTokenBudget(g, "proj", RenderOptions{
		MaxTokens: 1000,
		ClaudeMD:  "## Instructions\nDo the thing.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Do the thing.") {
		t.Errorf("output should contain ClaudeMD content: %s", output)
	}
}
