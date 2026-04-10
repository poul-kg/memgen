package tools

import (
	"fmt"

	"github.com/poul-kg/memgen/internal/knowledge"
	"github.com/poul-kg/memgen/internal/ticket"
)

// Init gathers data from all sources and creates a new knowledge file.
func Init(deps *Deps, repo, branch string) (string, error) {
	// 1. Extract ticket ID from branch.
	ticketID, err := extractTicket(branch)
	if err != nil {
		return "", err
	}

	// 2. Create per-request GitHub client and validate.
	gh := deps.githubClient(repo)
	if err := gh.ValidateRepo(); err != nil {
		return "", err
	}

	// 3. Fetch JIRA ticket (validates ticket exists).
	jiraTicket, err := deps.JIRA.FetchTicket(ticketID)
	if err != nil {
		browseURL := ticket.BrowseURL(deps.JIRABaseURL, ticketID)
		return "", fmt.Errorf("failed to fetch JIRA ticket %s (browse: %s): %w", ticketID, browseURL, err)
	}

	// 4. Try to acquire lock.
	key := lockKey(repo, ticketID)
	if !deps.Locks.TryLock(key) {
		return "", fmt.Errorf("An operation is already in progress for %s. Try again later.", ticketID)
	}
	defer deps.Locks.Unlock(key)

	// 5. Fetch GitHub PRs.
	prs, err := gh.FetchPRs(ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch GitHub PRs for %s: %w", ticketID, err)
	}

	// 6. Fetch main branch commits.
	mainCommits, err := gh.FetchMainCommits(ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch main branch commits for %s: %w", ticketID, err)
	}

	// 7. Build knowledge file directly from source data.
	kf := knowledge.FromSources(ticketID, branch, jiraTicket, prs, mainCommits)

	// 8. Write YAML knowledge file.
	if err := deps.Store.WriteKnowledge(repo, ticketID, kf); err != nil {
		return "", fmt.Errorf("failed to write knowledge file: %w", err)
	}

	// 9. Return success summary.
	return fmt.Sprintf("Initialized knowledge for %s: JIRA ticket + %d PRs + %d commits on main",
		ticketID, len(prs), len(mainCommits)), nil
}
