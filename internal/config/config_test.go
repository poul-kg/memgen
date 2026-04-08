package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setBaseDir overrides the config base directory for test isolation
// and returns a cleanup function that restores the original value.
func setBaseDir(t *testing.T, dir string) {
	t.Helper()
	old := baseDir
	baseDir = dir
	t.Cleanup(func() { baseDir = old })
}

func TestConfigDir(t *testing.T) {
	tmp := t.TempDir()
	setBaseDir(t, tmp)

	got := ConfigDir()
	if got != tmp {
		t.Errorf("ConfigDir() = %q, want %q", got, tmp)
	}
}

func TestKnowledgeDir(t *testing.T) {
	tmp := t.TempDir()
	setBaseDir(t, tmp)

	want := filepath.Join(tmp, "knowledge") + string(os.PathSeparator)
	got := KnowledgeDir()
	if got != want {
		t.Errorf("KnowledgeDir() = %q, want %q", got, want)
	}
}

func TestConfigPath(t *testing.T) {
	tmp := t.TempDir()
	setBaseDir(t, tmp)

	want := filepath.Join(tmp, "config.toml")
	got := ConfigPath()
	if got != want {
		t.Errorf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestSampleConfigPath(t *testing.T) {
	tmp := t.TempDir()
	setBaseDir(t, tmp)

	want := filepath.Join(tmp, "config.sample.toml")
	got := SampleConfigPath()
	if got != want {
		t.Errorf("SampleConfigPath() = %q, want %q", got, want)
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		content   string // empty means no file written
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid config",
			content: `[server]
port = 8080

[jira]
url = "https://mycompany.atlassian.net"
email = "alice@mycompany.com"
token = "real-api-token-abc123"
`,
			wantErr: false,
		},
		{
			name: "missing jira.email field",
			content: `[server]
port = 8080

[jira]
url = "https://mycompany.atlassian.net"
token = "real-api-token-abc123"
`,
			wantErr:   true,
			errSubstr: "jira.email is required",
		},
		{
			name: "missing jira.token field",
			content: `[server]
port = 8080

[jira]
url = "https://mycompany.atlassian.net"
email = "alice@mycompany.com"
`,
			wantErr:   true,
			errSubstr: "jira.token is required",
		},
		{
			name: "missing jira.url field",
			content: `[server]
port = 8080

[jira]
email = "alice@mycompany.com"
token = "real-api-token-abc123"
`,
			wantErr:   true,
			errSubstr: "jira.url is required",
		},
		{
			name: "missing server.port field",
			content: `[jira]
url = "https://mycompany.atlassian.net"
email = "alice@mycompany.com"
token = "real-api-token-abc123"
`,
			wantErr:   true,
			errSubstr: "server.port is required",
		},
		{
			name: "placeholder email",
			content: `[server]
port = 8080

[jira]
url = "https://mycompany.atlassian.net"
email = "user@example.com"
token = "real-api-token-abc123"
`,
			wantErr:   true,
			errSubstr: "placeholder values",
		},
		{
			name: "placeholder token",
			content: `[server]
port = 8080

[jira]
url = "https://mycompany.atlassian.net"
email = "alice@mycompany.com"
token = "your-jira-api-token"
`,
			wantErr:   true,
			errSubstr: "placeholder values",
		},
		{
			name: "placeholder url with example",
			content: `[server]
port = 8080

[jira]
url = "https://example.atlassian.net"
email = "alice@mycompany.com"
token = "real-api-token-abc123"
`,
			wantErr:   true,
			errSubstr: "placeholder values",
		},
		{
			name:      "malformed TOML",
			content:   `this is not [valid toml = `,
			wantErr:   true,
			errSubstr: "failed to parse",
		},
		{
			name:      "missing file",
			content:   "", // no file written
			wantErr:   true,
			errSubstr: "failed to read config file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			setBaseDir(t, tmp)

			if tt.content != "" {
				path := ConfigPath()
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("failed to write test config: %v", err)
				}
			}

			cfg, err := Load()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() succeeded, want error containing %q", tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Load() error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg == nil {
				t.Fatal("Load() returned nil config without error")
			}
			if cfg.Server.Port != 8080 {
				t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
			}
			if cfg.JIRA.URL != "https://mycompany.atlassian.net" {
				t.Errorf("JIRA.URL = %q, want %q", cfg.JIRA.URL, "https://mycompany.atlassian.net")
			}
			if cfg.JIRA.Email != "alice@mycompany.com" {
				t.Errorf("JIRA.Email = %q, want %q", cfg.JIRA.Email, "alice@mycompany.com")
			}
			if cfg.JIRA.Token != "real-api-token-abc123" {
				t.Errorf("JIRA.Token = %q, want %q", cfg.JIRA.Token, "real-api-token-abc123")
			}
		})
	}
}

func TestIsPlaceholder(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "all real values",
			cfg: Config{
				Server: ServerConfig{Port: 8080},
				JIRA: JIRAConfig{
					URL:   "https://mycompany.atlassian.net",
					Email: "alice@mycompany.com",
					Token: "real-token",
				},
			},
			want: false,
		},
		{
			name: "placeholder email",
			cfg: Config{
				Server: ServerConfig{Port: 8080},
				JIRA: JIRAConfig{
					URL:   "https://mycompany.atlassian.net",
					Email: "user@example.com",
					Token: "real-token",
				},
			},
			want: true,
		},
		{
			name: "placeholder token",
			cfg: Config{
				Server: ServerConfig{Port: 8080},
				JIRA: JIRAConfig{
					URL:   "https://mycompany.atlassian.net",
					Email: "alice@mycompany.com",
					Token: "your-jira-api-token",
				},
			},
			want: true,
		},
		{
			name: "url containing example",
			cfg: Config{
				Server: ServerConfig{Port: 8080},
				JIRA: JIRAConfig{
					URL:   "https://example.atlassian.net",
					Email: "alice@mycompany.com",
					Token: "real-token",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPlaceholder(&tt.cfg)
			if got != tt.want {
				t.Errorf("IsPlaceholder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsureDir(t *testing.T) {
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "sub", "deep")
	setBaseDir(t, nested)

	if err := EnsureDir(); err != nil {
		t.Fatalf("EnsureDir() error: %v", err)
	}

	info, err := os.Stat(nested)
	if err != nil {
		t.Fatalf("config dir does not exist after EnsureDir(): %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("config dir path is not a directory")
	}

	// calling again should be idempotent
	if err := EnsureDir(); err != nil {
		t.Fatalf("EnsureDir() second call error: %v", err)
	}
}

func TestWriteSampleConfig(t *testing.T) {
	tmp := t.TempDir()
	setBaseDir(t, tmp)

	if err := WriteSampleConfig(); err != nil {
		t.Fatalf("WriteSampleConfig() error: %v", err)
	}

	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written sample config: %v", err)
	}

	content := string(data)

	// Verify key parts of the sample config are present.
	checks := []string{
		"[server]",
		"port = 3040",
		"[jira]",
		`url = "https://your-company.atlassian.net"`,
		`email = "user@example.com"`,
		`token = "your-jira-api-token"`,
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("sample config missing expected content %q\ngot:\n%s", check, content)
		}
	}
}

func TestConfigDirDefault(t *testing.T) {
	// With no override, ConfigDir should use the home directory.
	old := baseDir
	baseDir = ""
	t.Cleanup(func() { baseDir = old })

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	want := filepath.Join(home, ".config", "memgen") + string(os.PathSeparator)
	got := ConfigDir()
	if got != want {
		t.Errorf("ConfigDir() default = %q, want %q", got, want)
	}
}
