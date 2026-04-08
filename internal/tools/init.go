package tools

import (
	"fmt"
	"log"

	"github.com/poul-kg/memgen/internal/ticket"
)

// Init gathers data from all sources and creates a new knowledge file.
func Init(deps *Deps, repo, branch string) (string, error) {
	// 1. Extract ticket ID from branch.
	ticketID, err := extractTicket(branch)
	if err != nil {
		return "", err
	}

	// 2. Try to acquire lock.
	key := lockKey(repo, ticketID)
	if !deps.Locks.TryLock(key) {
		return "", fmt.Errorf("An operation is already in progress for %s. Try again later.", ticketID)
	}
	// 3. Defer unlock.
	defer deps.Locks.Unlock(key)

	// 4. Fetch JIRA ticket.
	jiraTicket, err := deps.JIRA.FetchTicket(ticketID)
	if err != nil {
		browseURL := ticket.BrowseURL(deps.JIRABaseURL, ticketID)
		return "", fmt.Errorf("failed to fetch JIRA ticket %s (browse: %s): %w", ticketID, browseURL, err)
	}

	// 5. Fetch GitHub PRs (log warning but continue if fails).
	prs, err := deps.GitHub.FetchPRs(ticketID)
	if err != nil {
		log.Printf("Warning: failed to fetch GitHub PRs for %s: %v", ticketID, err)
		prs = nil
	}

	// 6. Fetch main branch commits (log warning but continue if fails).
	mainCommits, err := deps.GitHub.FetchMainCommits(ticketID)
	if err != nil {
		log.Printf("Warning: failed to fetch main branch commits for %s: %v", ticketID, err)
		mainCommits = nil
	}

	// 7. Format all raw data.
	rawData := formatRawData(jiraTicket, prs, mainCommits)

	// 8. Call Claude CLI InitKnowledge.
	result, err := deps.Claude.InitKnowledge(rawData, branch, ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to initialize knowledge via Claude: %w", err)
	}

	// 9. Write result to store.
	if err := deps.Store.Write(repo, ticketID, result); err != nil {
		return "", fmt.Errorf("failed to write knowledge file: %w", err)
	}

	// 10. Return success summary.
	return fmt.Sprintf("Initialized knowledge for %s: JIRA ticket + %d PRs + %d commits on main",
		ticketID, len(prs), len(mainCommits)), nil
}
