package tools

import (
	"fmt"
	"time"
)

// Get retrieves the knowledge file for a ticket, with staleness check.
func Get(deps *Deps, repo, branch string) (string, error) {
	// 1. Extract ticket ID.
	ticketID, err := extractTicket(branch)
	if err != nil {
		return "", err
	}

	// 2. If file doesn't exist, return init recommendation.
	if !deps.Store.Exists(repo, ticketID) {
		return fmt.Sprintf("No knowledge found for %s. Run memgen__init to initialize knowledge for this ticket.", ticketID), nil
	}

	// 3. Read file content.
	content, err := deps.Store.Read(repo, ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to read knowledge file for %s: %w", ticketID, err)
	}

	// 4. Check staleness (24h threshold), append StalenessNote if applicable.
	note := deps.Store.StalenessNote(content, 24*time.Hour)
	if note != "" {
		content = content + "\n\n---\n" + note + "\nRun memgen__refresh to update."
	}

	// 5. Return content.
	return content, nil
}
