package compact

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Go compaction -----------------------------------------------------------

func TestCompactGoStripsComments(t *testing.T) {
	src := []byte(`// Package foo does things.
package foo

// Doer does the thing.
type Doer interface {
	// Do performs the action.
	Do() error
}

// add returns the sum of a and b.
func add(a, b int) int {
	// simple addition
	return a + b /* trailing block comment */
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	text := string(got)

	// No comment text should survive.
	for _, fragment := range []string{"Package foo", "Doer does", "Do performs", "add returns", "simple addition", "trailing block"} {
		if strings.Contains(text, fragment) {
			t.Errorf("comment fragment %q still present in output:\n%s", fragment, text)
		}
	}

	// Core code must survive.
	for _, fragment := range []string{"package foo", "type Doer interface", "Do() error", "func add(", "return a + b"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("code fragment %q missing from output:\n%s", fragment, text)
		}
	}
}

func TestCompactGoOutputIsValidGo(t *testing.T) {
	src := []byte(`// Package example is an example.
package example

import (
	"fmt"
	"strings"
)

// upper returns s in upper case.
func upper(s string) string {
	// convert
	return strings.ToUpper(fmt.Sprintf("%s", s))
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	// Re-parse to verify syntactic validity.
	fset := token.NewFileSet()
	if _, parseErr := parser.ParseFile(fset, "", got, 0); parseErr != nil {
		t.Fatalf("compacted output is not valid Go: %v\noutput:\n%s", parseErr, got)
	}
}

func TestCompactGoNoBlankLines(t *testing.T) {
	src := []byte(`package foo

func a() {}

func b() {}

func c() {}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	if strings.Contains(string(got), "\n\n") {
		t.Errorf("output contains blank lines:\n%s", got)
	}
}

func TestCompactGoPreservesDirectives(t *testing.T) {
	src := []byte(`//go:build linux

package foo

func Foo() {}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	if !strings.Contains(string(got), "//go:build linux") {
		t.Errorf("build directive missing from output:\n%s", got)
	}
	// Output must still be valid Go.
	fset := token.NewFileSet()
	if _, parseErr := parser.ParseFile(fset, "", got, 0); parseErr != nil {
		t.Fatalf("compacted output with directive is not valid Go: %v\noutput:\n%s", parseErr, got)
	}
}

func TestCompactGoPreservesEmbedFiles(t *testing.T) {
	// //go:embed must stay adjacent to its var declaration and cannot be moved
	// to the file top. Files containing it should be returned unchanged.
	src := []byte(`package foo

import _ "embed"

//go:embed hello.txt
var hello string
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	if !bytes.Equal(got, src) {
		t.Errorf("expected source unchanged for //go:embed file, got:\n%s", got)
	}
}

func TestCompactGoParseError(t *testing.T) {
	// Invalid Go source should produce a parse error from compactGo.
	src := []byte("package foo\nfunc {{{")
	_, err := CompactSource(src, Go)
	if err == nil {
		t.Error("expected parse error for invalid Go source, got nil")
	}
}

func TestCompactGoReducesSize(t *testing.T) {
	src := []byte(`// Package math provides basic math utilities.
// It is intentionally simple.
package math

// Add returns the sum of a and b.
// Both values must be non-negative.
func Add(a, b int) int {
	// perform the addition
	return a + b
}

// Multiply returns the product of a and b.
func Multiply(a, b int) int {
	// multiply
	return a * b
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	if len(got) >= len(src) {
		t.Errorf("expected compacted output to be smaller: original=%d compacted=%d", len(src), len(got))
	}
	reduction := float64(len(src)-len(got)) / float64(len(src)) * 100
	t.Logf("reduction: %.1f%% (%d → %d bytes)", reduction, len(src), len(got))
	if reduction < 20 {
		t.Errorf("expected at least 20%% reduction, got %.1f%%", reduction)
	}
}

// --- Generic language compaction ---------------------------------------------

func TestCompactTypeScriptStripsLineComments(t *testing.T) {
	src := []byte(`// Module entry point
import { foo } from './foo'; // side-effect import

// Greet says hello.
function greet(name: string): string {
  // build the message
  const url = "http://example.com"; // not a comment inside string
  return ` + "`" + `Hello, ${name}` + "`" + `; // template literal
}
`)
	got, _ := CompactSource(src, TypeScript)
	text := string(got)

	// Comments must be gone.
	for _, fragment := range []string{"Module entry point", "Greet says", "build the message", "side-effect import"} {
		if strings.Contains(text, fragment) {
			t.Errorf("comment %q still present:\n%s", fragment, text)
		}
	}

	// String content must be intact.
	if !strings.Contains(text, "http://example.com") {
		t.Errorf("URL inside string was stripped:\n%s", text)
	}
	if !strings.Contains(text, "Hello, ${name}") {
		t.Errorf("template literal content was stripped:\n%s", text)
	}
}

func TestCompactTypeScriptStripsBlockComments(t *testing.T) {
	src := []byte(`/* copyright header */
export function add(a: number /* first */, b: number /* second */): number {
  return a + b;
}
`)
	got, _ := CompactSource(src, TypeScript)
	text := string(got)

	for _, fragment := range []string{"copyright header", "first", "second"} {
		if strings.Contains(text, fragment) {
			t.Errorf("block comment fragment %q still present:\n%s", fragment, text)
		}
	}
	if !strings.Contains(text, "return a + b") {
		t.Errorf("code missing from output:\n%s", text)
	}
}

func TestCompactTypeScriptPreservesSlashesInStrings(t *testing.T) {
	src := []byte(`const re = /https?:\/\//;
const msg = "see http://docs for details // not a comment";
const path = 'C:\\Users\\foo';
`)
	got, _ := CompactSource(src, TypeScript)
	text := string(got)

	if !strings.Contains(text, `"see http://docs for details // not a comment"`) {
		t.Errorf("string content with // was stripped:\n%s", text)
	}
}

func TestCompactPythonStripsHashComments(t *testing.T) {
	src := []byte(`# Module docstring replacement
import os  # stdlib

# Add two numbers.
def add(a, b):
    # perform addition
    return a + b  # result
`)
	got, _ := CompactSource(src, Python)
	text := string(got)

	for _, fragment := range []string{"Module docstring", "stdlib", "Add two numbers", "perform addition", "result"} {
		if strings.Contains(text, fragment) {
			t.Errorf("comment fragment %q present:\n%s", fragment, text)
		}
	}
	if !strings.Contains(text, "return a + b") {
		t.Errorf("code missing:\n%s", text)
	}
}

func TestCompactPythonPreservesHashInStrings(t *testing.T) {
	src := []byte(`color = "#ff0000"  # red hex
pattern = '#{name}'  # template
`)
	got, _ := CompactSource(src, Python)
	text := string(got)

	if !strings.Contains(text, `"#ff0000"`) {
		t.Errorf(`string "#ff0000" was stripped:\n%s`, text)
	}
	if !strings.Contains(text, `'#{name}'`) {
		t.Errorf(`string '#{name}' was stripped:\n%s`, text)
	}
}

func TestCompactPythonPreservesTripleQuotedStrings(t *testing.T) {
	src := []byte(`x = """this is a
multi-line string # not a comment
with content"""
y = 1  # strip this
`)
	got, _ := CompactSource(src, Python)
	text := string(got)

	if !strings.Contains(text, "multi-line string # not a comment") {
		t.Errorf("triple-quoted string content was stripped:\n%s", text)
	}
	if strings.Contains(text, "strip this") {
		t.Errorf("comment outside string was not stripped:\n%s", text)
	}
}

func TestCompactRustStripsComments(t *testing.T) {
	src := []byte(`// Crate root
fn main() {
    /* setup */
    let x = 1; // assign
    println!("{}", x);
}
`)
	got, _ := CompactSource(src, Rust)
	text := string(got)

	for _, fragment := range []string{"Crate root", "setup", "assign"} {
		if strings.Contains(text, fragment) {
			t.Errorf("comment %q present:\n%s", fragment, text)
		}
	}
	if !strings.Contains(text, `println!("{}", x)`) {
		t.Errorf("code missing:\n%s", text)
	}
}

// --- Identifier shortening ---------------------------------------------------

func TestShortenParamNames(t *testing.T) {
	src := []byte(`package foo
func process(authToken string, userCredentials []byte) error {
	result := doSomething(authToken, userCredentials)
	return result
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	text := string(got)

	// Long param names must be gone.
	if strings.Contains(text, "authToken") {
		t.Errorf("long param 'authToken' was not shortened:\n%s", text)
	}
	if strings.Contains(text, "userCredentials") {
		t.Errorf("long param 'userCredentials' was not shortened:\n%s", text)
	}

	// Output must be valid Go.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", got, 0); err != nil {
		t.Fatalf("output is not valid Go: %v\n%s", err, got)
	}
}

func TestShortenLocalVars(t *testing.T) {
	src := []byte(`package foo
func buildResult() string {
	headerValue := "hello"
	footerValue := "world"
	combined := headerValue + " " + footerValue
	return combined
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	text := string(got)

	for _, name := range []string{"headerValue", "footerValue", "combined"} {
		if strings.Contains(text, name) {
			t.Errorf("long local var %q was not shortened:\n%s", name, text)
		}
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", got, 0); err != nil {
		t.Fatalf("output is not valid Go: %v\n%s", err, got)
	}
}

func TestDoNotShortenStructFields(t *testing.T) {
	src := []byte(`package foo
type server struct{ timeout int }
func getTimeout(svr *server) int {
	timeout := svr.timeout
	return timeout
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	text := string(got)

	// The struct field access svr.timeout must still say .timeout.
	if !strings.Contains(text, ".timeout") {
		t.Errorf("struct field .timeout was incorrectly renamed:\n%s", text)
	}
	// The local var 'timeout' should be shortened (5 chars).
	// The param 'svr' is short (3 chars) and should stay.

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", got, 0); err != nil {
		t.Fatalf("output is not valid Go: %v\n%s", err, got)
	}
}

func TestDoNotShortenExportedNames(t *testing.T) {
	src := []byte(`package foo
func Process(RequestData []byte) error {
	result := handle(RequestData)
	return result
}
func handle(data []byte) error { return nil }
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	text := string(got)

	// Exported param name must not be renamed.
	if !strings.Contains(text, "RequestData") {
		t.Errorf("exported param 'RequestData' was incorrectly renamed:\n%s", text)
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", got, 0); err != nil {
		t.Fatalf("output is not valid Go: %v\n%s", err, got)
	}
}

func TestDoNotShortenBuiltins(t *testing.T) {
	src := []byte(`package foo
func countItems(items []string) int {
	length := len(items)
	result := make([]string, length)
	_ = result
	return length
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	text := string(got)

	// Built-in names must survive.
	if !strings.Contains(text, "len(") {
		t.Errorf("built-in 'len' was renamed:\n%s", text)
	}
	if !strings.Contains(text, "make(") {
		t.Errorf("built-in 'make' was renamed:\n%s", text)
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", got, 0); err != nil {
		t.Fatalf("output is not valid Go: %v\n%s", err, got)
	}
}

// --- Stats -------------------------------------------------------------------

func TestStats(t *testing.T) {
	s := Stats{
		Files:          3,
		OriginalBytes:  1000,
		CompactedBytes: 700,
	}
	if got := s.ByteReduction(); got != 30 {
		t.Errorf("ByteReduction = %.1f, want 30.0", got)
	}
	if got := s.OriginalTokens(); got != 250 {
		t.Errorf("OriginalTokens = %d, want 250", got)
	}
	if got := s.CompactedTokens(); got != 175 {
		t.Errorf("CompactedTokens = %d, want 175", got)
	}
}

func TestStatsZero(t *testing.T) {
	var s Stats
	if s.ByteReduction() != 0 {
		t.Error("expected 0 reduction for zero stats")
	}
}

// --- Language detection ------------------------------------------------------

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		file string
		want Language
	}{
		{"main.go", Go},
		{"index.ts", TypeScript},
		{"App.tsx", TypeScript},
		{"app.js", JavaScript},
		{"app.jsx", JavaScript},
		{"main.py", Python},
		{"main.rs", Rust},
		{"README.md", Unknown},
		{"Makefile", Unknown},
	}
	for _, c := range cases {
		if got := DetectLanguage(c.file); got != c.want {
			t.Errorf("DetectLanguage(%q) = %q, want %q", c.file, got, c.want)
		}
	}
}

// --- Stats.TokenReduction / Stats.String -------------------------------------

func TestStats_TokenReduction(t *testing.T) {
	s := Stats{OriginalBytes: 1000, CompactedBytes: 600}
	if got := s.TokenReduction(); got != 40 {
		t.Errorf("TokenReduction = %.1f, want 40.0", got)
	}
}

func TestStats_TokenReductionZero(t *testing.T) {
	var s Stats
	if got := s.TokenReduction(); got != 0 {
		t.Errorf("zero stats: TokenReduction = %.1f, want 0", got)
	}
}

func TestStats_String(t *testing.T) {
	s := Stats{Files: 5, OriginalBytes: 2000, CompactedBytes: 1000}
	got := s.String()
	for _, want := range []string{"5 files", "2000", "1000", "50.0%"} {
		if !strings.Contains(got, want) {
			t.Errorf("Stats.String() should contain %q, got: %s", want, got)
		}
	}
}

func TestStats_StringTokenApproximation(t *testing.T) {
	s := Stats{Files: 1, OriginalBytes: 400, CompactedBytes: 200}
	got := s.String()
	// 400/4 = 100 original tokens, 200/4 = 50 compacted tokens
	if !strings.Contains(got, "100") {
		t.Errorf("Stats.String() should contain original token count ~100, got: %s", got)
	}
	if !strings.Contains(got, "50") {
		t.Errorf("Stats.String() should contain compacted token count ~50, got: %s", got)
	}
}

// --- CompactDir --------------------------------------------------------------

func TestCompactDir_InPlace(t *testing.T) {
	dir := t.TempDir()
	src := []byte("// Package foo\npackage foo\n\n// Add adds.\nfunc Add(a, b int) int {\n\treturn a + b\n}\n")
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), src, 0600); err != nil {
		t.Fatal(err)
	}

	stats, err := CompactDir(dir, "")
	if err != nil {
		t.Fatalf("CompactDir: %v", err)
	}
	if stats.Files != 1 {
		t.Errorf("files: want 1, got %d", stats.Files)
	}
	if stats.OriginalBytes != len(src) {
		t.Errorf("original bytes: want %d, got %d", len(src), stats.OriginalBytes)
	}
	if stats.CompactedBytes >= stats.OriginalBytes {
		t.Errorf("expected compaction, original=%d compacted=%d", stats.OriginalBytes, stats.CompactedBytes)
	}
}

func TestCompactDir_ToOutDir(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	code := []byte("// Package x\npackage x\nfunc Noop() {}\n")
	if err := os.WriteFile(filepath.Join(src, "x.go"), code, 0600); err != nil {
		t.Fatal(err)
	}

	stats, err := CompactDir(src, out)
	if err != nil {
		t.Fatalf("CompactDir: %v", err)
	}
	if stats.Files != 1 {
		t.Errorf("files: want 1, got %d", stats.Files)
	}
	// Output file must exist
	if _, err := os.Stat(filepath.Join(out, "x.go")); err != nil {
		t.Errorf("output file not created: %v", err)
	}
	// Source file must be unchanged
	orig, _ := os.ReadFile(filepath.Join(src, "x.go"))
	if string(orig) != string(code) {
		t.Error("source file should be unchanged when outDir is set")
	}
}

func TestCompactDir_SkipsUnknownFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stats, err := CompactDir(dir, "")
	if err != nil {
		t.Fatalf("CompactDir: %v", err)
	}
	if stats.Files != 1 {
		t.Errorf("should skip README.md, want 1 file, got %d", stats.Files)
	}
}

func TestCompactDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	stats, err := CompactDir(dir, "")
	if err != nil {
		t.Fatalf("CompactDir empty: %v", err)
	}
	if stats.Files != 0 {
		t.Errorf("empty dir: want 0 files, got %d", stats.Files)
	}
}

func TestCompactDir_SkipsParseError(t *testing.T) {
	// CompactDir should skip (not fail) files that fail to parse.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package foo\nfunc {{{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "good.go"), []byte("package foo\nfunc Noop() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stats, err := CompactDir(dir, "")
	if err != nil {
		t.Fatalf("CompactDir should not error on parse failure: %v", err)
	}
	// Only the valid file is counted
	if stats.Files != 1 {
		t.Errorf("want 1 file (broken skipped), got %d", stats.Files)
	}
}

// ── shortenFuncLocals: range value and var spec ───────────────────────────────

func TestShortenRangeValueVar(t *testing.T) {
	src := []byte(`package foo
func processItems(items []string) {
	for _, itemValue := range items {
		_ = itemValue
	}
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	text := string(got)
	if strings.Contains(text, "itemValue") {
		t.Errorf("range value 'itemValue' should be shortened:\n%s", text)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", got, 0); err != nil {
		t.Fatalf("output is not valid Go: %v\n%s", err, got)
	}
}

func TestShortenVarStatement(t *testing.T) {
	src := []byte(`package foo
func buildMessage() string {
	var messageContent string
	messageContent = "hello"
	return messageContent
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	text := string(got)
	if strings.Contains(text, "messageContent") {
		t.Errorf("var 'messageContent' should be shortened:\n%s", text)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", got, 0); err != nil {
		t.Fatalf("output is not valid Go: %v\n%s", err, got)
	}
}

// ── nextShortName ─────────────────────────────────────────────────────────────

func TestNextShortName_SingleLetters(t *testing.T) {
	counter := 0
	existing := map[string]bool{}
	const letters = "abcdefghijklmnopqrstuvwxyz"
	for i := 0; i < 26; i++ {
		got := nextShortName(&counter, existing)
		want := string(letters[i])
		if got != want {
			t.Errorf("call %d: got %q, want %q", i, got, want)
		}
	}
}

func TestNextShortName_TwoLetterOverflow(t *testing.T) {
	counter := 0
	existing := map[string]bool{}
	// Exhaust all 26 single-char names.
	for i := 0; i < 26; i++ {
		nextShortName(&counter, existing)
	}
	if got := nextShortName(&counter, existing); got != "aa" {
		t.Errorf("first two-char name: got %q, want %q", got, "aa")
	}
	if got := nextShortName(&counter, existing); got != "ab" {
		t.Errorf("second two-char name: got %q, want %q", got, "ab")
	}
}

func TestNextShortName_SkipsExisting(t *testing.T) {
	counter := 0
	existing := map[string]bool{"a": true, "b": true}
	got := nextShortName(&counter, existing)
	if got != "c" {
		t.Errorf("expected 'c' (skipping a, b), got %q", got)
	}
}

func TestNextShortName_SkipsBuiltins(t *testing.T) {
	// '_' is a Go builtin; if the counter somehow produces it, it must be skipped.
	// We test indirectly by filling every single-char slot except 'z' with existing
	// names, then verifying we get 'z' (the only remaining free single-char slot).
	existing := map[string]bool{}
	const letters = "abcdefghijklmnopqrstuvwxyz"
	for _, ch := range letters {
		if ch != 'z' {
			existing[string(ch)] = true
		}
	}
	counter := 0
	got := nextShortName(&counter, existing)
	if got != "z" {
		t.Errorf("expected 'z' as only free single-char slot, got %q", got)
	}
}

// ── shortenFuncLocals: KeyValueExpr branch ────────────────────────────────────

// TestDoNotShortenStructLiteralKey covers the KeyValueExpr branch (L221-224):
// a struct literal key inside a function must be protected from renaming because
// the key is a field name, not a local variable.
func TestDoNotShortenStructLiteralKey(t *testing.T) {
	src := []byte(`package foo
type Point struct{ x, y int }
func makePoint() Point {
	longXValue := 5
	longYValue := 10
	return Point{x: longXValue, y: longYValue}
}
`)
	got, err := CompactSource(src, Go)
	if err != nil {
		t.Fatalf("CompactSource error: %v", err)
	}
	text := string(got)
	// Struct literal keys 'x' and 'y' are field names — they must not be renamed.
	if !strings.Contains(text, "x:") {
		t.Errorf("struct literal key 'x' should not be renamed:\n%s", text)
	}
	if !strings.Contains(text, "y:") {
		t.Errorf("struct literal key 'y' should not be renamed:\n%s", text)
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", got, 0); err != nil {
		t.Fatalf("output is not valid Go: %v\n%s", err, got)
	}
}

// ── stripComments: backtick string with backslash ─────────────────────────────

// TestStripComments_BacktickWithBackslash covers L406-409: when a backtick string
// (JS/TS template literal) contains a backslash, the next character must be
// consumed together to avoid mistaking an escaped backtick (\`) for a terminator.
func TestStripComments_BacktickWithBackslash(t *testing.T) {
	// TypeScript template literal: const s = `foo\nbar`;  // comment
	// The \n inside the backtick string hits c=='\\' at L406.
	src := []byte("const s = `foo\\nbar`; // comment")
	got, _ := CompactSource(src, TypeScript)
	text := string(got)
	if !strings.Contains(text, "foo") {
		t.Errorf("backtick string content should be preserved:\n%s", text)
	}
	// The trailing comment must be stripped.
	if strings.Contains(text, "comment") {
		t.Errorf("line comment after backtick string should be stripped:\n%s", text)
	}
}

// ── CompactDir error paths ────────────────────────────────────────────────────

// TestCompactDir_ReadFileError covers L477-479: WalkDir lists a file that cannot
// be read → os.ReadFile returns an error → CompactDir returns error.
func TestCompactDir_ReadFileError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	if err := os.WriteFile(f, []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(f, 0600) }) //nolint:errcheck
	_, err := CompactDir(dir, "")
	if err == nil {
		t.Error("CompactDir should fail when a source file is unreadable")
	}
}

// TestCompactDir_OutDirMkdirAllError covers L494-496: when outDir is set and a
// blocking file exists where a subdirectory would be created, MkdirAll fails.
func TestCompactDir_OutDirMkdirAllError(t *testing.T) {
	dir := t.TempDir()
	out := t.TempDir()

	// Create dir/sub/main.go so the relative path is "sub/main.go".
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Block out/sub creation by placing a regular file there.
	if err := os.WriteFile(filepath.Join(out, "sub"), []byte("blocker"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := CompactDir(dir, out)
	if err == nil {
		t.Error("CompactDir should fail when output subdirectory cannot be created")
	}
}

// TestCompactDir_WalkDirError covers L466-468: when WalkDir calls the callback
// with a non-nil error (unreadable subdirectory), CompactDir returns that error.
func TestCompactDir_WalkDirError(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping chmod-based test in CI")
	}
	dir := t.TempDir()
	subdir := filepath.Join(dir, "locked")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(subdir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(subdir, 0o700) }) //nolint:errcheck
	_, err := CompactDir(dir, "")
	if err == nil {
		t.Error("CompactDir should fail when a subdirectory is unreadable")
	}
}
