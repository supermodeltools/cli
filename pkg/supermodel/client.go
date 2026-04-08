// Package supermodel is the public Go SDK for the Supermodel API.
//
// Other Go programs can import this package to embed Supermodel graph
// analysis without shelling out to the CLI binary.
//
// Usage:
//
//	import "github.com/supermodeltools/cli/pkg/supermodel"
//
//	client := supermodel.NewClient("your-api-key")
//	graph, err := client.Analyze(ctx, "/path/to/repo")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("files:", len(graph.NodesByLabel("File")))
package supermodel

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

const defaultAPIBase = "https://api.supermodeltools.com"

// Client calls the Supermodel API.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the default API base URL.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient substitutes a custom HTTP client (useful for testing).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// NewClient returns a Client authenticated with apiKey.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultAPIBase,
		http:    &http.Client{Timeout: 15 * time.Minute},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// --- Public graph types ------------------------------------------------------

// Node is a node in the Supermodel graph.
type Node struct {
	ID         string         `json:"id"`
	Labels     []string       `json:"labels"`
	Properties map[string]any `json:"properties"`
}

// Prop returns the first non-empty string property for the given keys.
func (n Node) Prop(keys ...string) string {
	for _, k := range keys {
		if v, ok := n.Properties[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// HasLabel reports whether the node carries label.
func (n Node) HasLabel(label string) bool {
	for _, l := range n.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// Relationship is a directed edge between two nodes.
type Relationship struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	StartNode string `json:"startNode"`
	EndNode   string `json:"endNode"`
}

// Graph is the result of a Supermodel analysis.
type Graph struct {
	Nodes         []Node         `json:"nodes"`
	Edges         []Relationship `json:"edges"`
	Relationships []Relationship `json:"relationships"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// Rels returns all relationships regardless of which JSON field they came from.
func (g *Graph) Rels() []Relationship {
	if len(g.Relationships) > 0 {
		return g.Relationships
	}
	return g.Edges
}

// RepoID returns the repository ID from graph metadata.
func (g *Graph) RepoID() string {
	if g.Metadata == nil {
		return ""
	}
	s, _ := g.Metadata["repoId"].(string)
	return s
}

// NodesByLabel returns all nodes that carry label.
func (g *Graph) NodesByLabel(label string) []Node {
	var out []Node
	for _, n := range g.Nodes {
		if n.HasLabel(label) {
			out = append(out, n)
		}
	}
	return out
}

// APIError is returned for non-2xx responses.
type APIError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("API error %d (%s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// --- Client methods ----------------------------------------------------------

// Analyze archives repoPath and runs the full Supermodel analysis pipeline.
func (c *Client) Analyze(ctx context.Context, repoPath string) (*Graph, error) {
	zipPath, err := archiveRepo(repoPath)
	if err != nil {
		return nil, fmt.Errorf("archive repo: %w", err)
	}
	defer os.Remove(zipPath)

	hash, err := hashFile(zipPath)
	if err != nil {
		return nil, err
	}
	return c.AnalyzeZip(ctx, zipPath, "sdk-"+hash[:16])
}

const analyzeEndpoint = "/v1/graphs/supermodel"

// jobResponse is the async envelope returned by the API.
type jobResponse struct {
	Status     string          `json:"status"`
	JobID      string          `json:"jobId"`
	RetryAfter int             `json:"retryAfter"`
	Error      *string         `json:"error"`
	Result     json.RawMessage `json:"result"`
}

// jobResult is the inner result object wrapping the graph.
// The API response has shape: {"graph": {...}, "repo": "...", "domains": [...]}
type jobResult struct {
	Graph Graph  `json:"graph"`
	Repo  string `json:"repo"`
}

// AnalyzeZip uploads a pre-built ZIP to the Supermodel API and polls until
// the async job completes, returning the resulting Graph.
// idempotencyKey must be unique per logical request.
func (c *Client) AnalyzeZip(ctx context.Context, zipPath, idempotencyKey string) (*Graph, error) {
	post := func() (*jobResponse, error) { return c.postZip(ctx, zipPath, idempotencyKey) }

	job, err := post()
	if err != nil {
		return nil, err
	}
	for job.Status == "pending" || job.Status == "processing" {
		wait := time.Duration(job.RetryAfter) * time.Second
		if wait <= 0 {
			wait = 5 * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		job, err = post()
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
		return nil, fmt.Errorf("decode graph result: %w", err)
	}
	g := &result.Graph
	if result.Repo != "" {
		g.Metadata = map[string]any{"repoId": result.Repo}
	}
	return g, nil
}

// postZip sends the repository ZIP to the analyze endpoint and returns the raw job response.
func (c *Client) postZip(ctx context.Context, zipPath, idempotencyKey string) (*jobResponse, error) {
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
	if _, err := io.Copy(fw, f); err != nil {
		return nil, err
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+analyzeEndpoint, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var apiErr APIError
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
			apiErr.StatusCode = resp.StatusCode
			return nil, &apiErr
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var job jobResponse
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("decode job response: %w", err)
	}
	return &job, nil
}

// --- Archive helpers ---------------------------------------------------------

func archiveRepo(dir string) (string, error) {
	f, err := os.CreateTemp("", "supermodel-sdk-*.zip")
	if err != nil {
		return "", err
	}
	dest := f.Name()
	f.Close()

	if isGitRepo(dir) {
		cmd := exec.Command("git", "-C", dir, "archive", "--format=zip", "-o", dest, "HEAD")
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err == nil {
			return dest, nil
		}
	}
	if err := walkZip(dir, dest); err != nil {
		os.Remove(dest)
		return "", err
	}
	return dest, nil
}

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"__pycache__": true, ".venv": true, "venv": true,
	"dist": true, "build": true, "target": true,
	".next": true, ".terraform": true,
}

func walkZip(dir, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") || info.Size() > 10<<20 {
			return nil
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
