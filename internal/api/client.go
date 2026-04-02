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

// Analyze uploads a repository ZIP and runs the full analysis pipeline,
// returning the DisplayGraphResponse.
func (c *Client) Analyze(ctx context.Context, zipPath, idempotencyKey string) (*Graph, error) {
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

	var g Graph
	if err := c.request(ctx, http.MethodPost, "/v1/graphs/supermodel", mw.FormDataContentType(), &buf, idempotencyKey, &g); err != nil {
		return nil, err
	}
	return &g, nil
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
