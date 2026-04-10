package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/poul-kg/memgen/internal/config"
	"github.com/poul-kg/memgen/internal/knowledge"
	"github.com/poul-kg/memgen/internal/server"
	"github.com/poul-kg/memgen/internal/sources"
	"github.com/poul-kg/memgen/internal/tools"
)

func main() {
	// 1. Ensure config directory exists.
	if err := config.EnsureDir(); err != nil {
		log.Fatalf("Failed to create config directory: %v", err)
	}

	// 2. Load config (handles first-run sample creation).
	cfg, err := config.Load()
	if err != nil {
		// If config file doesn't exist, create a sample and exit.
		if os.IsNotExist(err) {
			if writeErr := config.WriteSampleConfig(); writeErr != nil {
				log.Fatalf("Failed to write sample config: %v", writeErr)
			}
			fmt.Printf("Created sample configuration at %s\n", config.ConfigPath())
			fmt.Println("Edit the config file with your JIRA credentials and restart.")
			os.Exit(0)
		}
		log.Fatalf("Configuration error: %v", err)
	}

	// 3. Check for placeholder values.
	if config.IsPlaceholder(cfg) {
		fmt.Printf("Configuration at %s contains placeholder values.\n", config.ConfigPath())
		fmt.Println("Edit the config file with your JIRA credentials and restart.")
		os.Exit(1)
	}

	// 4. Validate gh CLI.
	ghExecutor := &sources.DefaultExecutor{}
	if err := checkGitHubCLI(ghExecutor); err != nil {
		log.Fatalf("GitHub CLI check failed: %v\n\n%s", err, ghInstallInstructions())
	}
	fmt.Println("GitHub CLI is available and authenticated")

	// 5. Build dependencies.
	deps := &tools.Deps{
		Store: knowledge.NewStore(config.KnowledgeDir()),
		Locks: knowledge.NewLockManager(),
		JIRA: &sources.JIRAClient{
			BaseURL: cfg.JIRA.URL,
			Email:   cfg.JIRA.Email,
			Token:   cfg.JIRA.Token,
		},
		GitHubExecutor: ghExecutor,
		JIRABaseURL:    cfg.JIRA.URL,
	}

	// 6. Create and start server.
	mcpSrv := server.New(deps)
	httpSrv := server.NewHTTP(mcpSrv)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	fmt.Printf("MemGen MCP server listening on %s\n", addr)
	fmt.Printf("Endpoint: http://localhost:%d/mcp\n", cfg.Server.Port)
	if err := httpSrv.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func checkGitHubCLI(exec sources.CommandExecutor) error {
	_, stderr, err := exec.Execute("gh", "auth", "status")
	if err != nil {
		if strings.Contains(stderr, "not logged") {
			return fmt.Errorf("GitHub CLI is not authenticated. Run `gh auth login` to authenticate")
		}
		return fmt.Errorf("GitHub CLI error: %s", stderr)
	}
	return nil
}

func ghInstallInstructions() string {
	return `GitHub CLI Installation Instructions:

Fedora:
  sudo dnf install gh

Ubuntu/Debian:
  curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
  sudo apt update && sudo apt install gh

macOS:
  brew install gh

Windows:
  winget install --id GitHub.cli

After installing, run: gh auth login`
}
