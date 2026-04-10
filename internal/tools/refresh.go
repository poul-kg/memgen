package tools

import (
	"fmt"

	"github.com/poul-kg/memgen/internal/knowledge"
)

// Refresh re-fetches all data from JIRA and GitHub and rebuilds the knowledge file.
// Existing notes are preserved.
func Refresh(deps *Deps, repo, branch string) (string, error) {
	// 1. Extract ticket ID.
	ticketID, err := extractTicket(branch)
	if err != nil {
		return "", err
	}

	// 2. If file doesn't exist, return error telling to run init.
	if !deps.Store.Exists(repo, ticketID) {
		return "", fmt.Errorf("No knowledge file found for %s. Run memgen__init first.", ticketID)
	}

	// 3. Create per-request GitHub client and validate.
	gh := deps.githubClient(repo)
	if err := gh.ValidateRepo(); err != nil {
		return "", err
	}

	// 4. Try lock.
	key := lockKey(repo, ticketID)
	if !deps.Locks.TryLock(key) {
		return "", fmt.Errorf("An operation is already in progress for %s. Try again later.", ticketID)
	}
	defer deps.Locks.Unlock(key)

	// 5. Read existing knowledge, save notes.
	existingKF, err := deps.Store.ReadKnowledge(repo, ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to read knowledge file for %s: %w", ticketID, err)
	}
	savedNotes := existingKF.Notes

	// 6. Full re-fetch from all sources.
	jiraTicket, err := deps.JIRA.FetchTicket(ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch JIRA ticket for %s: %w", ticketID, err)
	}

	prs, err := gh.FetchPRs(ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch GitHub PRs for %s: %w", ticketID, err)
	}

	mainCommits, err := gh.FetchMainCommits(ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch main commits for %s: %w", ticketID, err)
	}

	// 7. Build new knowledge from fresh data.
	newKF := knowledge.FromSources(ticketID, branch, jiraTicket, prs, mainCommits)
	newKF.Notes = savedNotes // restore notes

	// 8. Write YAML.
	if err := deps.Store.WriteKnowledge(repo, ticketID, newKF); err != nil {
		return "", fmt.Errorf("failed to write knowledge file: %w", err)
	}

	// 9. Return summary.
	return fmt.Sprintf("Refreshed knowledge for %s: JIRA ticket + %d PRs + %d commits on main",
		ticketID, len(prs), len(mainCommits)), nil
}
