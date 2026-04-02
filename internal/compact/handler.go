package compact

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Language is a recognised source language.
type Language string

// Supported language values for Language.
const (
	Go         Language = "go"
	Python     Language = "python"
	TypeScript Language = "typescript"
	JavaScript Language = "javascript"
	Rust       Language = "rust"
	Unknown    Language = ""
)

// Stats records the aggregate result of a compaction run.
type Stats struct {
	Files          int
	OriginalBytes  int
	CompactedBytes int
}

// ByteReduction returns the percentage of bytes removed (0–100).
func (s Stats) ByteReduction() float64 {
	if s.OriginalBytes == 0 {
		return 0
	}
	return float64(s.OriginalBytes-s.CompactedBytes) / float64(s.OriginalBytes) * 100
}

// TokenReduction returns the estimated percentage of LLM tokens removed.
// Token count is approximated as bytes / 4.
func (s Stats) TokenReduction() float64 {
	return s.ByteReduction() // same ratio; we use bytes/4 throughout
}

// OriginalTokens returns the estimated token count of the original source.
func (s Stats) OriginalTokens() int {
	return (s.OriginalBytes + 3) / 4
}

// CompactedTokens returns the estimated token count after compaction.
func (s Stats) CompactedTokens() int {
	return (s.CompactedBytes + 3) / 4
}

// String returns a human-readable summary.
func (s Stats) String() string {
	return fmt.Sprintf(
		"%d files: %d → %d bytes (%.1f%% reduction, ~%d → ~%d tokens)",
		s.Files,
		s.OriginalBytes, s.CompactedBytes,
		s.ByteReduction(),
		s.OriginalTokens(), s.CompactedTokens(),
	)
}

// DetectLanguage returns the Language for the given filename, or Unknown.
func DetectLanguage(filename string) Language {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".go":
		return Go
	case ".py":
		return Python
	case ".ts", ".tsx":
		return TypeScript
	case ".js", ".jsx":
		return JavaScript
	case ".rs":
		return Rust
	}
	return Unknown
}

// CompactSource removes comments and blank lines from src.
//
// For Go it uses the standard AST parser so the result is guaranteed to be
// syntactically valid Go. Compiler directives (//go:build, //go:generate,
// //go:embed, // +build) are preserved because they affect compilation.
//
// For all other languages a string-aware state machine strips // and /* */
// comments (or # comments for Python) while correctly skipping content
// inside quoted string literals, then removes blank lines.
//
// Returns an error only for Go source that cannot be parsed.
func CompactSource(src []byte, lang Language) ([]byte, error) {
	switch lang {
	case Go:
		return compactGo(src)
	default:
		return compactGeneric(src, lang), nil
	}
}

// compactGo strips non-directive comments via the Go AST, shortens local
// identifiers, and removes blank lines.
func compactGo(src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	// Parsing without parser.ParseComments drops all comments from the AST.
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return nil, err
	}
	shortenIdents(f)
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, err
	}
	body := removeBlankLines(buf.Bytes())

	// Re-prepend any compiler directives that were in the original source,
	// since go/parser drops them without ParseComments.
	directives := scanGoDirectives(src)
	if len(directives) == 0 {
		return body, nil
	}
	// Build constraints require a blank line between them and the package clause.
	out := make([]byte, 0, len(directives)+2+len(body))
	out = append(out, directives...)
	out = append(out, '\n', '\n')
	out = append(out, body...)
	return out, nil
}

// scanGoDirectives extracts //go:build, // +build and //go:generate lines.
func scanGoDirectives(src []byte) []byte {
	var out []byte
	for _, line := range bytes.Split(src, []byte("\n")) {
		text := strings.TrimSpace(string(line))
		if strings.HasPrefix(text, "//go:build") ||
			strings.HasPrefix(text, "// +build") ||
			strings.HasPrefix(text, "//go:generate") ||
			strings.HasPrefix(text, "//go:embed") {
			out = append(out, bytes.TrimRight(line, " \t")...)
			out = append(out, '\n')
		}
	}
	return bytes.TrimRight(out, "\n")
}

// --- Identifier shortening ---------------------------------------------------

// minIdentLen is the minimum length for an identifier to be a shortening candidate.
const minIdentLen = 5

// goBuiltins is the set of predeclared Go identifiers that must never be renamed.
var goBuiltins = map[string]bool{
	// types
	"bool": true, "byte": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true, "int": true, "int8": true,
	"int16": true, "int32": true, "int64": true, "rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"uintptr": true, "any": true,
	// functions
	"append": true, "cap": true, "clear": true, "close": true, "complex": true,
	"copy": true, "delete": true, "imag": true, "len": true, "make": true,
	"max": true, "min": true, "new": true, "panic": true, "print": true,
	"println": true, "real": true, "recover": true,
	// constants / zero values
	"true": true, "false": true, "nil": true, "iota": true,
	// blank identifier
	"_": true,
}

// shortenIdents walks each function declaration in f and renames long
// unexported local identifiers (parameters, named returns, local variables)
// to short synthetic names. Exported names, built-ins, struct field
// accesses, and composite-literal keys are never renamed.
func shortenIdents(f *ast.File) {
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		shortenFuncLocals(fn)
		return true
	})
}

// shortenFuncLocals renames long local identifiers inside fn.
func shortenFuncLocals(fn *ast.FuncDecl) { //nolint:gocyclo
	// Phase 1: collect all existing identifier names to avoid collisions.
	existing := map[string]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			existing[id.Name] = true
		}
		return true
	})

	// Phase 2: mark struct-field accesses and composite-literal keys as
	// protected — these are field names, not local variables, and must
	// not be renamed even if the name matches a local variable.
	protected := map[*ast.Ident]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.SelectorExpr:
			protected[v.Sel] = true
		case *ast.KeyValueExpr:
			if id, ok := v.Key.(*ast.Ident); ok {
				protected[id] = true
			}
		}
		return true
	})

	// Phase 3: collect candidate names from declarations.
	candidates := map[string]bool{}
	addCandidate := func(name string) {
		if len(name) < minIdentLen || goBuiltins[name] {
			return
		}
		if name[0] >= 'A' && name[0] <= 'Z' {
			return // never rename exported identifiers
		}
		candidates[name] = true
	}

	// Params and named return values.
	for _, list := range []*ast.FieldList{fn.Type.Params, fn.Type.Results} {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			for _, id := range field.Names {
				addCandidate(id.Name)
			}
		}
	}

	// Local declarations inside the body.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.AssignStmt:
			if v.Tok == token.DEFINE {
				for _, lhs := range v.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						addCandidate(id.Name)
					}
				}
			}
		case *ast.RangeStmt:
			if v.Tok == token.DEFINE {
				if id, ok := v.Key.(*ast.Ident); ok {
					addCandidate(id.Name)
				}
				if v.Value != nil {
					if id, ok := v.Value.(*ast.Ident); ok {
						addCandidate(id.Name)
					}
				}
			}
		case *ast.ValueSpec:
			for _, id := range v.Names {
				addCandidate(id.Name)
			}
		}
		return true
	})

	if len(candidates) == 0 {
		return
	}

	// Phase 4: assign short names. Sort candidates for deterministic output.
	sorted := make([]string, 0, len(candidates))
	for name := range candidates {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	counter := 0
	mapping := make(map[string]string, len(sorted))
	for _, name := range sorted {
		short := nextShortName(&counter, existing)
		existing[short] = true
		mapping[name] = short
	}

	// Phase 5: apply mapping to all identifier references, skipping protected.
	ast.Inspect(fn, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || protected[id] {
			return true
		}
		if short, exists := mapping[id.Name]; exists {
			id.Name = short
		}
		return true
	})
}

// nextShortName returns the next available short identifier name that does
// not conflict with existing names or Go built-ins.
func nextShortName(counter *int, existing map[string]bool) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	for {
		n := *counter
		*counter++
		var name string
		if n < len(letters) {
			name = string(letters[n])
		} else {
			n -= len(letters)
			rows := len(letters)
			name = string([]byte{letters[(n/rows)%rows], letters[n%rows]})
		}
		if !existing[name] && !goBuiltins[name] {
			return name
		}
	}
}

// compactGeneric strips comments with a string-aware state machine, then
// removes blank lines.
func compactGeneric(src []byte, lang Language) []byte {
	return removeBlankLines(stripComments(src, lang))
}

// stripComments removes line and block comments from src.
// It tracks string literal state to avoid treating comment-like sequences
// inside strings as comments.
func stripComments(src []byte, lang Language) []byte { //nolint:gocyclo
	out := make([]byte, 0, len(src))
	i, n := 0, len(src)

	for i < n {
		// Python: triple-quoted strings must be checked before single-char quotes.
		if lang == Python && i+2 < n &&
			(src[i] == '"' || src[i] == '\'') &&
			src[i+1] == src[i] && src[i+2] == src[i] {
			q := src[i]
			out = append(out, src[i], src[i+1], src[i+2])
			i += 3
			for i < n {
				if i+2 < n && src[i] == q && src[i+1] == q && src[i+2] == q {
					out = append(out, src[i], src[i+1], src[i+2])
					i += 3
					break
				}
				out = append(out, src[i])
				i++
			}
			continue
		}

		// Python line comment.
		if lang == Python && src[i] == '#' {
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}

		// Single or double-quoted string (all languages).
		if src[i] == '"' || src[i] == '\'' {
			q := src[i]
			out = append(out, src[i])
			i++
			for i < n {
				c := src[i]
				out = append(out, c)
				i++
				if c == '\\' && i < n {
					out = append(out, src[i])
					i++
					continue
				}
				if c == q {
					break
				}
			}
			continue
		}

		// Backtick string: JS/TS template literal or Go raw string literal.
		if src[i] == '`' {
			out = append(out, src[i])
			i++
			for i < n {
				c := src[i]
				out = append(out, c)
				i++
				if c == '\\' && i < n {
					out = append(out, src[i])
					i++
					continue
				}
				if c == '`' {
					break
				}
			}
			continue
		}

		// C-style line comment (// ...) — not Python.
		if lang != Python && i+1 < n && src[i] == '/' && src[i+1] == '/' {
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}

		// C-style block comment (/* ... */) — not Python.
		if lang != Python && i+1 < n && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < n {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		out = append(out, src[i])
		i++
	}
	return out
}

// removeBlankLines removes all blank (whitespace-only) lines and trims
// trailing whitespace from every line. The result has no blank lines and
// no trailing whitespace, maximising token efficiency.
func removeBlankLines(src []byte) []byte {
	lines := bytes.Split(src, []byte("\n"))
	out := make([][]byte, 0, len(lines))
	for _, line := range lines {
		trimmed := bytes.TrimRight(line, " \t\r")
		if len(trimmed) > 0 {
			out = append(out, trimmed)
		}
	}
	return bytes.TrimSpace(bytes.Join(out, []byte("\n")))
}

// CompactDir walks dir and compacts every recognised source file, writing
// results to outDir. If outDir is empty files are written back in place.
// Unrecognised file types are skipped (not copied).
func CompactDir(dir, outDir string) (Stats, error) {
	var stats Stats
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		lang := DetectLanguage(path)
		if lang == Unknown {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		compacted, err := CompactSource(src, lang)
		if err != nil {
			// Skip files that don't parse (generated code, syntax errors).
			return nil
		}

		stats.Files++
		stats.OriginalBytes += len(src)
		stats.CompactedBytes += len(compacted)

		dest := path
		if outDir != "" {
			rel, _ := filepath.Rel(dir, path)
			dest = filepath.Join(outDir, rel)
			if mkErr := os.MkdirAll(filepath.Dir(dest), 0o750); mkErr != nil {
				return mkErr
			}
		}
		return os.WriteFile(dest, compacted, 0o600)
	})
	return stats, err
}
