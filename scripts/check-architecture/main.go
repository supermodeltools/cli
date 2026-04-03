// Command check-architecture validates the vertical slice architecture of the
// Supermodel CLI using the Supermodel static analysis API.
//
// Rules enforced:
//
//  1. Slice packages must not import other slice packages.
//  2. Slice packages may only import the shared kernel or external dependencies.
//  3. SkipDirs must only be declared in internal/archive (no duplicate definitions in slices).
//
// A "slice" is any package under internal/ that is NOT listed in sharedKernel.
//
// Usage:
//
//	SUPERMODEL_API_KEY=<key> go run ./scripts/check-architecture
//
// Environment:
//
//	SUPERMODEL_API_KEY   required — API key issued by api.supermodeltools.com
//	SUPERMODEL_API_BASE  optional — override the API base URL (default: https://api.supermodeltools.com)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// module is the Go module path for this repository.
const module = "github.com/supermodeltools/cli"

// sharedKernel lists internal packages that slices are permitted to import.
// When adding a new cross-cutting infrastructure package, add it here.
var sharedKernel = map[string]bool{
	"internal/api":     true,
	"internal/archive": true,
	"internal/build":   true,
	"internal/cache":   true,
	"internal/config":  true,
	"internal/ui":      true,
	// pkg/ is a public SDK, not subject to slice rules.
}

// --- Supermodel API response types -------------------------------------------
// Schema: DisplayGraphResponse from api.supermodeltools.com/v1/graphs/supermodel

type graphResponse struct {
	Nodes         []graphNode    `json:"nodes"`
	Edges         []relationship `json:"edges"`
	Relationships []relationship `json:"relationships"`
}

func (g *graphResponse) rels() []relationship {
	if len(g.Relationships) > 0 {
		return g.Relationships
	}
	return g.Edges
}

// jobResponse is the async envelope returned by the API.
type jobResponse struct {
	Status     string          `json:"status"`
	RetryAfter int             `json:"retryAfter"`
	Error      *string         `json:"error"`
	Result     json.RawMessage `json:"result"`
}

type jobResult struct {
	Graph graphResponse `json:"graph"`
}

type graphNode struct {
	ID         string         `json:"id"`
	Labels     []string       `json:"labels"`
	Properties map[string]any `json:"properties"`
}

type relationship struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	StartNode string `json:"startNode"`
	EndNode   string `json:"endNode"`
}

// -----------------------------------------------------------------------------

func main() {
	apiKey := os.Getenv("SUPERMODEL_API_KEY")
	if apiKey == "" {
		fatalf("SUPERMODEL_API_KEY environment variable is required\n" +
			"  Get an API key at https://supermodeltools.com")
	}
	apiBase := envOr("SUPERMODEL_API_BASE", "https://api.supermodeltools.com")

	zipPath, err := gitArchive()
	if err != nil {
		fatalf("git archive failed: %v", err)
	}
	fmt.Println("→ Uploading repository to Supermodel API...")
	graph, err := callAPI(apiBase, apiKey, zipPath)
	os.Remove(zipPath)
	if err != nil {
		fatalf("API call failed: %v", err)
	}
	fmt.Printf("→ Checking %d nodes, %d relationships\n", len(graph.Nodes), len(graph.rels()))

	// Build nodeID → internal package path (e.g. "internal/analyze")
	nodePackage := buildPackageMap(graph.Nodes)

	var violations []string
	for _, rel := range graph.rels() {
		if rel.Type != "imports" && rel.Type != "wildcard_imports" {
			continue
		}
		src := nodePackage[rel.StartNode]
		dst := nodePackage[rel.EndNode]
		if src == "" || dst == "" {
			continue
		}
		if isSlice(src) && isSlice(dst) && src != dst {
			violations = append(violations, fmt.Sprintf("  %-38s →  %s", src, dst))
		}
	}

	if len(violations) > 0 {
		fmt.Fprintln(os.Stderr, "\n✗  Vertical slice violations detected:")
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, v)
		}
		fmt.Fprintln(os.Stderr, "\nSlices must only import from the shared kernel:")
		for k := range sharedKernel {
			fmt.Fprintf(os.Stderr, "  %s\n", k)
		}
		fmt.Fprintln(os.Stderr, "\nSee docs/architecture.md for the full rules.")
		os.Exit(1)
	}

	fmt.Println("✓  Architecture check passed — no cross-slice imports found.")

	// Rule 3: Slices must not declare their own SkipDirs — use internal/archive.
	if dupes := checkDuplicateSkipDirs(); len(dupes) > 0 {
		fmt.Fprintln(os.Stderr, "\n✗  Duplicate skipDirs declarations found:")
		for _, d := range dupes {
			fmt.Fprintf(os.Stderr, "  %s\n", d)
		}
		fmt.Fprintln(os.Stderr, "\nSkipDirs must only be declared in internal/archive/archive.go.")
		fmt.Fprintln(os.Stderr, "Slice zip.go files should import internal/archive instead.")
		os.Exit(1)
	}
	fmt.Println("✓  No duplicate skipDirs — single source of truth in internal/archive.")
}

// checkDuplicateSkipDirs scans Go source files for skipDirs/SkipDirs variable
// declarations outside of internal/archive. Returns file paths of violators.
func checkDuplicateSkipDirs() []string {
	var dupes []string
	_ = filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Skip the canonical location.
		if strings.HasPrefix(filepath.ToSlash(path), "internal/archive/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		if strings.Contains(content, "var skipDirs") || strings.Contains(content, "var SkipDirs") {
			dupes = append(dupes, path)
		}
		return nil
	})
	return dupes
}

// buildPackageMap maps each node ID to an "internal/X" package path.
// Nodes that don't resolve to an internal package are omitted.
func buildPackageMap(nodes []graphNode) map[string]string {
	m := make(map[string]string, len(nodes))
	for _, n := range nodes {
		if pkg := internalPackageOf(n); pkg != "" {
			m[n.ID] = pkg
		}
	}
	return m
}

// internalPackageOf returns the top-level internal package (e.g. "internal/analyze")
// for a node, or "" if the node doesn't correspond to an internal package.
func internalPackageOf(n graphNode) string {
	path := stringProp(n.Properties, "path", "name", "file")
	if path == "" {
		return ""
	}
	return resolvePackage(path)
}

// resolvePackage converts either a file path or a Go import path to a
// normalised "internal/X" package string (top-level only, not nested).
//
// Handles two forms:
//
//	File path:   "internal/analyze/handler.go" → "internal/analyze"
//	Import path: "github.com/supermodeltools/cli/internal/analyze" → "internal/analyze"
func resolvePackage(raw string) string {
	raw = strings.TrimPrefix(raw, "./")

	// Import path style
	if strings.HasPrefix(raw, module+"/") {
		rel := strings.TrimPrefix(raw, module+"/")
		return topLevelInternal(rel)
	}

	// File path style (with .go extension)
	if strings.HasSuffix(raw, ".go") && strings.HasPrefix(raw, "internal/") {
		return topLevelInternal(filepath.Dir(raw))
	}

	return ""
}

// topLevelInternal returns "internal/X" for a path rooted at "internal/X/..."
// or "" if the path doesn't start with internal/.
func topLevelInternal(rel string) string {
	if !strings.HasPrefix(rel, "internal/") {
		return ""
	}
	// "internal/analyze/sub/pkg" → "internal/analyze"
	rest := strings.TrimPrefix(rel, "internal/")
	top := strings.SplitN(rest, "/", 2)[0]
	if top == "" {
		return ""
	}
	return "internal/" + top
}

// isSlice returns true if pkg is an internal package subject to slice isolation.
func isSlice(pkg string) bool {
	return strings.HasPrefix(pkg, "internal/") && !sharedKernel[pkg]
}

// stringProp returns the first non-empty string value from a properties map.
func stringProp(props map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := props[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// --- API helpers -------------------------------------------------------------

func gitArchive() (string, error) {
	f, err := os.CreateTemp("", "supermodel-arch-*.zip")
	if err != nil {
		return "", err
	}
	f.Close()
	cmd := exec.Command("git", "archive", "-o", f.Name(), "HEAD")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("git archive: %w", err)
	}
	return f.Name(), nil
}

func callAPI(apiBase, apiKey, zipPath string) (*graphResponse, error) {
	ikey := idempotencyKey()
	job, err := postZip(apiBase, apiKey, zipPath, ikey)
	if err != nil {
		return nil, err
	}

	for job.Status == "pending" || job.Status == "processing" {
		wait := time.Duration(job.RetryAfter) * time.Second
		if wait <= 0 {
			wait = 5 * time.Second
		}
		fmt.Printf("→ Job %s, retrying in %s...\n", job.Status, wait)
		time.Sleep(wait)
		job, err = postZip(apiBase, apiKey, zipPath, ikey)
		if err != nil {
			return nil, err
		}
	}

	if job.Error != nil {
		return nil, fmt.Errorf("analysis failed: %s", *job.Error)
	}
	if job.Status != "completed" {
		return nil, fmt.Errorf("unexpected job status: %s", job.Status)
	}

	var result jobResult
	if err := json.Unmarshal(job.Result, &result); err != nil {
		return nil, fmt.Errorf("parse job result: %w", err)
	}
	return &result.Graph, nil
}

func postZip(apiBase, apiKey, zipPath, ikey string) (*jobResponse, error) {
	f, err := os.Open(zipPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filepath.Base(zipPath))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(fw, f); err != nil {
		return nil, err
	}
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, apiBase+"/v1/graphs/supermodel", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Idempotency-Key", ikey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		snippet := string(body)
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}

	var job jobResponse
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &job, nil
}

func idempotencyKey() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "arch-check"
	}
	return "arch-check-" + strings.TrimSpace(string(out))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
