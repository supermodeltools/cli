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
	job, err := c.postZip(ctx, zipPath, idempotencyKey)
	if err != nil {
		return nil, err
	}

	// Poll until the job completes.
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

		job, err = c.postZip(ctx, zipPath, idempotencyKey)
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
	return &result.Graph, nil
}

// postZip sends the repository ZIP to the analyze endpoint and returns the
// raw job response (which may be pending, processing, or completed).
func (c *Client) postZip(ctx context.Context, zipPath, idempotencyKey string) (*JobResponse, error) {
	return c.postZipTo(ctx, zipPath, idempotencyKey, analyzeEndpoint)
}

// deadCodeEndpoint is the API path for dead code analysis.
const deadCodeEndpoint = "/v1/analysis/dead-code"

// DeadCode uploads a repository ZIP and runs dead code analysis,
// polling until the async job completes and returning the result.
func (c *Client) DeadCode(ctx context.Context, zipPath, idempotencyKey string, minConfidence string, limit int) (*DeadCodeResult, error) {
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
