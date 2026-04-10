package tools

import (
	"github.com/poul-kg/memgen/internal/knowledge"
	"github.com/poul-kg/memgen/internal/sources"
	"github.com/poul-kg/memgen/internal/ticket"
)

// Deps holds shared dependencies for all tools.
type Deps struct {
	Store          *knowledge.Store
	Locks          *knowledge.LockManager
	JIRA           *sources.JIRAClient
	GitHubExecutor sources.CommandExecutor // shared executor, repo set per-request
	JIRABaseURL    string                  // for browse URLs in error messages
}

// githubClient creates a per-request GitHubClient with the correct repo.
func (d *Deps) githubClient(repo string) *sources.GitHubClient {
	return &sources.GitHubClient{
		Repo:     repo,
		Executor: d.GitHubExecutor,
	}
}

// lockKey returns the lock key for a repo+ticket combination.
func lockKey(repo, ticketID string) string {
	return repo + "/" + ticketID
}

// extractTicket extracts ticket ID from branch, returns formatted error if not found.
func extractTicket(branch string) (string, error) {
	return ticket.Extract(branch)
}
