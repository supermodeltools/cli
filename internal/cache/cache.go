package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/config"
)

// entry wraps a cached graph with provenance metadata.
type entry struct {
	Graph    *api.Graph `json:"graph"`
	CachedAt time.Time  `json:"cached_at"`
}

func dir() string {
	return filepath.Join(config.Dir(), "cache")
}

// Get loads a cached graph for hash. Returns (nil, nil) on cache miss.
func Get(hash string) (*api.Graph, error) {
	data, err := os.ReadFile(filepath.Join(dir(), hash+".json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse cache: %w", err)
	}
	return e.Graph, nil
}

// Put stores g in the cache under hash.
func Put(hash string, g *api.Graph) error {
	if err := os.MkdirAll(dir(), 0o700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	data, err := json.Marshal(entry{Graph: g, CachedAt: time.Now()})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir(), hash+".json"), data, 0o600); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}
	return nil
}

// HashFile returns the hex-encoded SHA-256 of the file at path.
func HashFile(path string) (string, error) {
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

// Evict removes the cached entry for hash. No-ops on cache miss.
func Evict(hash string) error {
	err := os.Remove(filepath.Join(dir(), hash+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
