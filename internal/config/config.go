package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// Config captures the user editable settings stored in .wt/config.toml.
type Config struct {
	DefaultBranch string         `toml:"default_branch"`
	Bootstrap     BootstrapBlock `toml:"bootstrap"`
	Tidy          TidyBlock      `toml:"tidy"`
	Process       ProcessBlock   `toml:"process"`
	CI            CIBlock        `toml:"ci"`
}

// BootstrapBlock describes commands that run after creating a new worktree.
type BootstrapBlock struct {
	Run    string `toml:"run"`
	Strict *bool  `toml:"strict"`
}

// TidyBlock governs wt tidy behavior.
type TidyBlock struct {
	Policy            string `toml:"policy"`
	StaleDays         int    `toml:"stale_days"`
	DivergenceCommits int    `toml:"divergence_commits"`
}

func (t *TidyBlock) applyDefaults() {
	if t == nil {
		return
	}
	if t.Policy == "" {
		t.Policy = "auto"
	} else {
		t.Policy = strings.ToLower(t.Policy)
	}
	if t.StaleDays <= 0 {
		t.StaleDays = 14
	}
	if t.DivergenceCommits <= 0 {
		t.DivergenceCommits = 20
	}
}

func (t TidyBlock) Validate() error {
	switch t.Policy {
	case "auto", "safe", "all", "prompt":
		return nil
	default:
		return ErrInvalidTidyPolicy
	}
}

// ProcessBlock configures process handling behavior.
type ProcessBlock struct {
	KillTimeout string `toml:"kill_timeout"`
}

func (p *ProcessBlock) applyDefaults() {
	if p == nil {
		return
	}
	if strings.TrimSpace(p.KillTimeout) == "" {
		p.KillTimeout = "3s"
	}
}

func (p ProcessBlock) Validate() error {
	if strings.TrimSpace(p.KillTimeout) == "" {
		return nil
	}
	d, err := time.ParseDuration(p.KillTimeout)
	if err != nil || d <= 0 {
		return ErrInvalidProcessTimeout
	}
	return nil
}

// KillTimeoutDuration returns the configured timeout or the default when unset.
func (p ProcessBlock) KillTimeoutDuration() time.Duration {
	d, err := time.ParseDuration(p.KillTimeout)
	if err != nil || d <= 0 {
		return 3 * time.Second
	}
	return d
}

// CIBlock configures how wt discovers GitHub CI metadata.
type CIBlock struct {
	Remote string `toml:"remote"`
}

func (c *CIBlock) applyDefaults() {
	if c == nil {
		return
	}
	c.Remote = strings.TrimSpace(c.Remote)
	if c.Remote == "" {
		c.Remote = "origin"
	}
}

// RemoteName returns the configured remote, defaulting to "origin".
func (c CIBlock) RemoteName() string {
	if strings.TrimSpace(c.Remote) == "" {
		return "origin"
	}
	return c.Remote
}

// CIRemote returns the configured remote for CI metadata.
func (c Config) CIRemote() string {
	return c.CI.RemoteName()
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
	// ErrInvalidTidyPolicy indicates the tidy policy is not recognized.
	ErrInvalidTidyPolicy = errors.New("config.tidy.policy must be auto, safe, all, or prompt")
	// ErrInvalidProcessTimeout indicates the process kill timeout is invalid.
	ErrInvalidProcessTimeout = errors.New("config.process.kill_timeout must be a positive duration (e.g. 3s)")
)

// Default returns a baseline configuration for a project.
func Default(defaultBranch string) Config {
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	cfg := Config{
		DefaultBranch: defaultBranch,
		Bootstrap:     BootstrapBlock{},
		Process:       ProcessBlock{},
		CI:            CIBlock{},
	}
	cfg.applyDefaults()
	return cfg
}

func (c *Config) applyDefaults() {
	c.Tidy.applyDefaults()
	c.Process.applyDefaults()
	c.CI.applyDefaults()
}

// Validate ensures the configuration can guide wt's behavior.
func (c Config) Validate() error {
	if c.DefaultBranch == "" {
		return ErrMissingDefaultBranch
	}
	if err := c.Tidy.Validate(); err != nil {
		return err
	}
	if err := c.Process.Validate(); err != nil {
		return err
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
	cfg.applyDefaults()
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
