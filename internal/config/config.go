package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	envConfigPath        = "OBI_CONFIG"
	defaultConfigName    = "obi.toml"
	DefaultSummaryPrompt = `You will receive commit summaries and detailed notes for every bead completed in this epic. Your job is to write one cohesive, multi-line commit message (subject line + detailed body) that captures the entire story so humans can understand what shipped.

Guidelines:
- Highlight major functional threads (features, bugs, migrations) rather than restating every bead verbatim.
- Call out tests, docs, and follow-ups when they matter.
- If information appears truncated or missing, acknowledge the limitation rather than inventing details.`
	DefaultSummaryMaxCommits = 20
	DefaultSummaryChunkSize  = 5
)

// Config represents the root obi configuration stored in TOML.
type Config struct {
	ResultsLog       string                `toml:"results_log"`
	BasePrompt       string                `toml:"base_prompt"`
	Codex            CodexConfig           `toml:"codex"`
	Epics            map[string]EpicConfig `toml:"epic"`
	Issues           *IssuesConfig         `toml:"issues outside epics"`
	ConfirmBeforeRun *bool                 `toml:"confirm_before_run"`
	Summary          SummaryConfig         `toml:"summary"`
}

// EpicConfig declares how a specific domain/epic should be handled.
type EpicConfig struct {
	Name          string       `toml:"name"`
	ID            string       `toml:"id"`
	Prompt        string       `toml:"prompt"`
	Tool          string       `toml:"tool"`
	Alias         string       `toml:"alias"`
	Filters       EpicFilters  `toml:"filters"`
	CodexOverride *CodexConfig `toml:"codex"`
}

// EpicFilters are optional bd filters that scope ready issues.
type EpicFilters struct {
	Labels     []string `toml:"labels"`
	Types      []string `toml:"types"`
	Priorities []int    `toml:"priorities"`
}

// IssuesConfig governs standalone issues not attached to epics.
type IssuesConfig struct {
	Prompt  string      `toml:"prompt"`
	Filters EpicFilters `toml:"filters"`
}

// SummaryConfig controls the omnibus commit summarizer.
type SummaryConfig struct {
	Prompt     string `toml:"prompt"`
	MaxCommits int    `toml:"max_commits"`
	ChunkSize  int    `toml:"chunk_size"`
}

// CodexConfig controls how codex CLI should be invoked.
type CodexConfig struct {
	Binary    string   `toml:"binary"`
	Model     string   `toml:"model"`
	Sandbox   string   `toml:"sandbox"`
	Approval  string   `toml:"approval"`
	ExtraArgs []string `toml:"extra_args"`
}

// Load reads and parses the provided TOML file.
func Load(path string) (*Config, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(bytes, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Epics == nil {
		cfg.Epics = map[string]EpicConfig{}
	}
	if len(cfg.Epics) == 0 && cfg.Issues == nil {
		return nil, errors.New("config must define at least one [epic.*] section or an \"issues outside epics\" block")
	}

	return &cfg, nil
}

// ResolvePath picks the config location via precedence: flag, env, default path.
func ResolvePath(flagPath string) (string, error) {
	if flagPath != "" {
		return expandPath(flagPath)
	}
	if env := os.Getenv(envConfigPath); env != "" {
		return expandPath(env)
	}
	return searchLocalConfig()
}

func expandPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		return filepath.Join(home, path[1:]), nil
	}
	return filepath.Abs(path)
}

func searchLocalConfig() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working dir: %w", err)
	}
	dir := wd
	for {
		candidate := filepath.Join(dir, defaultConfigName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find %s in current directory or parents", defaultConfigName)
}

// Epic fetches a named epic by key, alias, or epic ID.
func (c *Config) Epic(name string) (string, EpicConfig, error) {
	requested := strings.TrimSpace(name)
	if requested == "" {
		return "", EpicConfig{}, errors.New("epic must be specified")
	}
	if epic, ok := c.Epics[requested]; ok {
		return requested, epic, nil
	}

	var matchedKey string
	setMatch := func(key string) error {
		if matchedKey != "" && matchedKey != key {
			return fmt.Errorf("epic identifier %q is ambiguous between %s and %s", requested, matchedKey, key)
		}
		matchedKey = key
		return nil
	}

	for key, epic := range c.Epics {
		if strings.EqualFold(epic.ID, requested) {
			if err := setMatch(key); err != nil {
				return "", EpicConfig{}, err
			}
		}
	}

	for key, epic := range c.Epics {
		candidate := strings.TrimSpace(epic.Alias)
		if candidate == "" {
			candidate = key
		}
		if strings.EqualFold(candidate, requested) {
			if err := setMatch(key); err != nil {
				return "", EpicConfig{}, err
			}
		}
	}

	if matchedKey != "" {
		return matchedKey, c.Epics[matchedKey], nil
	}
	return "", EpicConfig{}, fmt.Errorf("unknown epic %q", requested)
}

// ConfirmBeforeRunValue returns whether obi go should pause before executing Codex.
func (c *Config) ConfirmBeforeRunValue() bool {
	if c.ConfirmBeforeRun == nil {
		return true
	}
	return *c.ConfirmBeforeRun
}

// SummaryConfigValue returns the summary config with defaults applied.
func (c *Config) SummaryConfigValue() SummaryConfig {
	cfg := c.Summary
	if strings.TrimSpace(cfg.Prompt) == "" {
		cfg.Prompt = DefaultSummaryPrompt
	}
	if cfg.MaxCommits <= 0 {
		cfg.MaxCommits = DefaultSummaryMaxCommits
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = DefaultSummaryChunkSize
	}
	return cfg
}

// ResultsLogPath returns the configured results log location (with default).
func (c *Config) ResultsLogPath() (string, error) {
	if c.ResultsLog != "" {
		return expandPath(c.ResultsLog)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, "obi", "results.log"), nil
}

// EffectiveCodex merges default codex config with optional epic override.
func (c *Config) EffectiveCodex(t EpicConfig) CodexConfig {
	if t.CodexOverride == nil {
		return c.Codex
	}
	return mergeCodex(c.Codex, *t.CodexOverride)
}

func mergeCodex(base, override CodexConfig) CodexConfig {
	merged := base
	if override.Binary != "" {
		merged.Binary = override.Binary
	}
	if override.Model != "" {
		merged.Model = override.Model
	}
	if override.Sandbox != "" {
		merged.Sandbox = override.Sandbox
	}
	if override.Approval != "" {
		merged.Approval = override.Approval
	}
	if len(override.ExtraArgs) > 0 {
		merged.ExtraArgs = append([]string{}, override.ExtraArgs...)
	}
	return merged
}
