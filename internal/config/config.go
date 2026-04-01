package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultAPIBase = "https://api.supermodeltools.com"
	defaultOutput  = "human"
)

// Config holds user-level settings persisted at ~/.supermodel/config.yaml.
type Config struct {
	APIKey  string `yaml:"api_key,omitempty"`
	APIBase string `yaml:"api_base,omitempty"`
	Output  string `yaml:"output,omitempty"` // "human" | "json"
}

// Dir returns the Supermodel config directory (~/.supermodel).
func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".supermodel")
}

// Path returns the full path to the config file.
func Path() string {
	return filepath.Join(Dir(), "config.yaml")
}

// Load reads the config file. Returns defaults when the file does not exist.
func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if os.IsNotExist(err) {
		return defaults(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

// Save writes the config to disk, creating the directory if necessary.
// The file is written with mode 0600 (owner-readable only).
func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.WriteFile(Path(), data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// RequireAPIKey returns an actionable error if no API key is configured.
func (c *Config) RequireAPIKey() error {
	if c.APIKey == "" {
		return fmt.Errorf("not authenticated — run `supermodel login` first")
	}
	return nil
}

func defaults() *Config {
	return &Config{APIBase: DefaultAPIBase, Output: defaultOutput}
}

func (c *Config) applyDefaults() {
	if c.APIBase == "" {
		c.APIBase = DefaultAPIBase
	}
	if c.Output == "" {
		c.Output = defaultOutput
	}
}
