package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Config captures the user editable settings stored in .wt/config.toml.
type Config struct {
	DefaultBranch string         `toml:"default_branch"`
	Bootstrap     BootstrapBlock `toml:"bootstrap"`
}

// BootstrapBlock describes commands that run after creating a new worktree.
type BootstrapBlock struct {
	Run    string `toml:"run"`
	Strict *bool  `toml:"strict"`
}

// StrictEnabled reports whether strict shell options should be enabled.
func (b BootstrapBlock) StrictEnabled() bool {
	if b.Strict == nil {
		return true
	}
	return *b.Strict
}

var (
	// ErrMissingDefaultBranch indicates the config omitted the required branch.
	ErrMissingDefaultBranch = errors.New("config.default_branch must be set")
)

// Default returns a baseline configuration for a project.
func Default(defaultBranch string) Config {
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	return Config{
		DefaultBranch: defaultBranch,
		Bootstrap:     BootstrapBlock{},
	}
}

// Validate ensures the configuration can guide wt's behavior.
func (c Config) Validate() error {
	if c.DefaultBranch == "" {
		return ErrMissingDefaultBranch
	}
	return nil
}

// Load reads configuration from disk. Missing files return a default config.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default("main"), nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Save writes configuration to disk, creating parent directories as needed.
func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
