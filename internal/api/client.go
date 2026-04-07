package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/supermodeltools/cli/internal/config"
)

const defaultTimeout = 15 * time.Minute

// Client is an authenticated Supermodel API client.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// New returns a Client configured from cfg.
func New(cfg *config.Config) *Client {
	return &Client{
		apiKey:  cfg.APIKey,
		baseURL: cfg.APIBase,
		http:    &http.Client{Timeout: defaultTimeout},
	}
}

// analyzeEndpoint is the API path for the supermodel graph analysis.
const analyzeEndpoint = "/v1/graphs/supermodel"

// Analyze uploads a repository ZIP and runs the full analysis pipeline,
// polling until the async job completes and returning the Graph.
func (c *Client) Analyze(ctx context.Context, zipPath, idempotencyKey string) (*Graph, error) {
	job, err := c.pollUntilComplete(ctx, zipPath, idempotencyKey)
	if err != nil {
		return nil, err
	}
	var result jobResult
	if err := json.Unmarshal(job.Result, &result); err != nil {
		return nil, fmt.Errorf("decode graph result: %w", err)
	}
	return &result.Graph, nil
}

// AnalyzeRaw uploads a repository ZIP and runs the full analysis pipeline,
// returning the raw result JSON from the completed job. Use this when you need
// the full response payload (e.g. for graph2md / docs generation).
func (c *Client) AnalyzeRaw(ctx context.Context, zipPath, idempotencyKey string) (json.RawMessage, error) {
	job, err := c.pollUntilComplete(ctx, zipPath, idempotencyKey)
	if err != nil {
		return nil, err
	}
	return job.Result, nil
}

// AnalyzeDomains uploads a repository ZIP and runs the full analysis pipeline,
// returning the complete SupermodelIR response (domains, summary, metadata, graph).
// Use this instead of Analyze when you need high-level domain information.
func (c *Client) AnalyzeDomains(ctx context.Context, zipPath, idempotencyKey string) (*SupermodelIR, error) {
	job, err := c.pollUntilComplete(ctx, zipPath, idempotencyKey)
	if err != nil {
		return nil, err
	}
	var ir SupermodelIR
	if err := json.Unmarshal(job.Result, &ir); err != nil {
		return nil, fmt.Errorf("decode domain result: %w", err)
	}
	return &ir, nil
}

// pollUntilComplete submits a ZIP to the analyze endpoint and polls until the
// async job reaches "completed" status, then returns the raw JobResponse.
func (c *Client) pollUntilComplete(ctx context.Context, zipPath, idempotencyKey string) (*JobResponse, error) {
	post := func() (*JobResponse, error) { return c.postZip(ctx, zipPath, idempotencyKey) }
	return c.pollLoop(ctx, post)
}

// pollLoop calls post() for the initial submission, then keeps calling it until
// the job reaches a terminal state. post() is called on every poll so all
// request fields (including incremental changedFiles) are sent on each retry.
func (c *Client) pollLoop(ctx context.Context, post func() (*JobResponse, error)) (*JobResponse, error) {
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
	return job, nil
}

// AnalyzeSidecars uploads a repository ZIP and runs the full analysis pipeline,
// returning the complete SidecarIR response with full Node/Relationship data
// required for sidecar rendering (IDs, labels, properties preserved).
func (c *Client) AnalyzeSidecars(ctx context.Context, zipPath, idempotencyKey string) (*SidecarIR, error) {
	job, err := c.pollUntilComplete(ctx, zipPath, idempotencyKey)
	if err != nil {
		return nil, err
	}
	var ir SidecarIR
	if err := json.Unmarshal(job.Result, &ir); err != nil {
		return nil, fmt.Errorf("decode sidecar result: %w", err)
	}
	return &ir, nil
}

// AnalyzeIncremental uploads a zip of changed files and requests an incremental
// graph update from the API. changedFiles is sent on every request (initial and
// retries) so the server always has the full context.
func (c *Client) AnalyzeIncremental(ctx context.Context, zipPath string, changedFiles []string, idempotencyKey string) (*SidecarIR, error) {
	post := func() (*JobResponse, error) {
		return c.postIncrementalZip(ctx, zipPath, changedFiles, idempotencyKey)
	}
	job, err := c.pollLoop(ctx, post)
	if err != nil {
		return nil, err
	}

	var ir SidecarIR
	if err := json.Unmarshal(job.Result, &ir); err != nil {
		return nil, fmt.Errorf("decode incremental sidecar result: %w", err)
	}
	return &ir, nil
}

// postZip sends the repository ZIP to the analyze endpoint and returns the
// raw job response (which may be pending, processing, or completed).
func (c *Client) postZip(ctx context.Context, zipPath, idempotencyKey string) (*JobResponse, error) {
	return c.postZipTo(ctx, zipPath, idempotencyKey, analyzeEndpoint)
}

// postIncrementalZip builds a multipart form with both the ZIP and the
// changedFiles JSON array, then submits it to the analyze endpoint.
func (c *Client) postIncrementalZip(ctx context.Context, zipPath string, changedFiles []string, idempotencyKey string) (*JobResponse, error) {
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
	changedJSON, err := json.Marshal(changedFiles)
	if err != nil {
		return nil, err
	}
	if err := mw.WriteField("changedFiles", string(changedJSON)); err != nil {
		return nil, err
	}
	mw.Close()

	var job JobResponse
	if err := c.request(ctx, http.MethodPost, analyzeEndpoint, mw.FormDataContentType(), &buf, idempotencyKey, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// deadCodeEndpoint is the API path for dead code analysis.
const deadCodeEndpoint = "/v1/analysis/dead-code"

// DeadCode uploads a repository ZIP and runs dead code analysis,
// polling until the async job completes and returning the result.
func (c *Client) DeadCode(ctx context.Context, zipPath, idempotencyKey, minConfidence string, limit int) (*DeadCodeResult, error) {
	endpoint := deadCodeEndpoint
	sep := "?"
	if minConfidence != "" {
		endpoint += sep + "min_confidence=" + minConfidence
		sep = "&"
	}
	if limit > 0 {
		endpoint += sep + fmt.Sprintf("limit=%d", limit)
	}

	job, err := c.postZipTo(ctx, zipPath, idempotencyKey, endpoint)
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

		job, err = c.postZipTo(ctx, zipPath, idempotencyKey, endpoint)
		if err != nil {
			return nil, err
		}
	}

	if job.Error != nil {
		return nil, fmt.Errorf("dead code analysis failed: %s", *job.Error)
	}
	if job.Status != "completed" {
		return nil, fmt.Errorf("unexpected job status: %s", job.Status)
	}

	var result DeadCodeResult
	if err := json.Unmarshal(job.Result, &result); err != nil {
		return nil, fmt.Errorf("decode dead code result: %w", err)
	}
	return &result, nil
}

// postZipTo sends a repository ZIP to the given endpoint and returns the job response.
func (c *Client) postZipTo(ctx context.Context, zipPath, idempotencyKey, endpoint string) (*JobResponse, error) {
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

	var job JobResponse
	if err := c.request(ctx, http.MethodPost, endpoint, mw.FormDataContentType(), &buf, idempotencyKey, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// impactEndpoint is the API path for impact analysis.
const impactEndpoint = "/v1/analysis/impact"

// Impact uploads a repository ZIP (and optional diff) and runs impact analysis,
// polling until the async job completes and returning the result.
func (c *Client) Impact(ctx context.Context, zipPath, idempotencyKey, targets, diffPath string) (*ImpactResult, error) {
	endpoint := impactEndpoint
	if targets != "" {
		endpoint += "?targets=" + targets
	}

	job, err := c.postImpact(ctx, zipPath, diffPath, idempotencyKey, endpoint)
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
		job, err = c.postImpact(ctx, zipPath, diffPath, idempotencyKey, endpoint)
		if err != nil {
			return nil, err
		}
	}

	if job.Error != nil {
		return nil, fmt.Errorf("impact analysis failed: %s", *job.Error)
	}
	if job.Status != "completed" {
		return nil, fmt.Errorf("unexpected job status: %s", job.Status)
	}

	var result ImpactResult
	if err := json.Unmarshal(job.Result, &result); err != nil {
		return nil, fmt.Errorf("decode impact result: %w", err)
	}
	return &result, nil
}

// postImpact sends the repo ZIP and optional diff to the impact endpoint.
func (c *Client) postImpact(ctx context.Context, zipPath, diffPath, idempotencyKey, endpoint string) (*JobResponse, error) {
	if diffPath == "" {
		return c.postZipTo(ctx, zipPath, idempotencyKey, endpoint)
	}

	// Multipart with both zip and diff.
	zipFile, err := os.Open(zipPath)
	if err != nil {
		return nil, err
	}
	defer zipFile.Close()

	diffFile, err := os.Open(diffPath)
	if err != nil {
		return nil, err
	}
	defer diffFile.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", filepath.Base(zipPath))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(fw, zipFile); err != nil {
		return nil, err
	}

	dw, err := mw.CreateFormFile("diff", filepath.Base(diffPath))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(dw, diffFile); err != nil {
		return nil, err
	}
	mw.Close()

	var job JobResponse
	if err := c.request(ctx, http.MethodPost, endpoint, mw.FormDataContentType(), &buf, idempotencyKey, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// DisplayGraph fetches the composed display graph for an already-analyzed repo.
func (c *Client) DisplayGraph(ctx context.Context, repoID, idempotencyKey string) (*Graph, error) {
	var g Graph
	if err := c.request(ctx, http.MethodGet, "/v1/repos/"+repoID+"/graph/display", "application/json", nil, idempotencyKey, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (c *Client) request(ctx context.Context, method, path, contentType string, body io.Reader, idempotencyKey string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		var apiErr Error
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Message != "" {
			apiErr.StatusCode = resp.StatusCode
			return &apiErr
		}
		snippet := string(respBody)
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
