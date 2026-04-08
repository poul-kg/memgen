package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// baseDir is the override for the config base directory.
// When empty, the default ~/.config/memgen/ is used.
// Tests can set this to a temp directory for isolation.
var baseDir string

// Config holds the full application configuration.
type Config struct {
	Server ServerConfig
	JIRA   JIRAConfig
}

// ServerConfig holds server-related settings.
type ServerConfig struct {
	Port int
}

// JIRAConfig holds JIRA integration settings.
type JIRAConfig struct {
	URL   string
	Email string
	Token string
}

const sampleConfig = `[server]
port = 3040

[jira]
url = "https://your-company.atlassian.net"
email = "user@example.com"
token = "your-jira-api-token"
`

// ConfigDir returns the path to the memgen configuration directory.
func ConfigDir() string {
	if baseDir != "" {
		return baseDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "~"
	}
	return filepath.Join(home, ".config", "memgen") + string(os.PathSeparator)
}

// KnowledgeDir returns the path to the knowledge subdirectory.
func KnowledgeDir() string {
	return filepath.Join(ConfigDir(), "knowledge") + string(os.PathSeparator)
}

// ConfigPath returns the full path to the config.toml file.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.toml")
}

// SampleConfigPath returns the full path to config.sample.toml.
func SampleConfigPath() string {
	return filepath.Join(ConfigDir(), "config.sample.toml")
}

// Load reads, parses, and validates the configuration from ConfigPath().
// It returns an error if the file is missing, malformed, has missing required
// fields, or contains placeholder values.
func Load() (*Config, error) {
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if IsPlaceholder(&cfg) {
		return nil, fmt.Errorf("config contains placeholder values; please edit %s", path)
	}

	return &cfg, nil
}

// validate checks that all required fields are present and non-empty.
func validate(cfg *Config) error {
	if cfg.Server.Port == 0 {
		return fmt.Errorf("server.port is required")
	}
	if cfg.JIRA.URL == "" {
		return fmt.Errorf("jira.url is required")
	}
	if cfg.JIRA.Email == "" {
		return fmt.Errorf("jira.email is required")
	}
	if cfg.JIRA.Token == "" {
		return fmt.Errorf("jira.token is required")
	}
	return nil
}

// IsPlaceholder reports whether the config contains placeholder/default values
// that indicate the user has not yet customized their configuration.
func IsPlaceholder(cfg *Config) bool {
	if cfg.JIRA.Email == "user@example.com" {
		return true
	}
	if cfg.JIRA.Token == "your-jira-api-token" {
		return true
	}
	if strings.Contains(cfg.JIRA.URL, "example") {
		return true
	}
	return false
}

// EnsureDir creates the config directory (and parents) if it doesn't exist.
func EnsureDir() error {
	dir := ConfigDir()
	return os.MkdirAll(dir, 0o755)
}

// WriteSampleConfig writes the sample configuration to ConfigPath().
func WriteSampleConfig() error {
	if err := EnsureDir(); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	path := ConfigPath()
	if err := os.WriteFile(path, []byte(sampleConfig), 0o644); err != nil {
		return fmt.Errorf("failed to write sample config to %s: %w", path, err)
	}
	return nil
}
