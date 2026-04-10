package tools

import (
	"fmt"
	"time"

	"github.com/poul-kg/memgen/internal/knowledge"
	"gopkg.in/yaml.v3"
)

// Get retrieves the knowledge file for a ticket, with optional scope filtering and staleness check.
func Get(deps *Deps, repo, branch, scope string) (string, error) {
	// 1. Extract ticket ID.
	ticketID, err := extractTicket(branch)
	if err != nil {
		return "", err
	}

	// 2. If file doesn't exist, return init recommendation.
	if !deps.Store.Exists(repo, ticketID) {
		return fmt.Sprintf("No knowledge found for %s. Run memgen__init to initialize knowledge for this ticket.", ticketID), nil
	}

	// 3. If scope provided, use ReadSection.
	if scope != "" {
		content, err := deps.Store.ReadSection(repo, ticketID, scope)
		if err != nil {
			return "", fmt.Errorf("failed to read %s section for %s: %w", scope, ticketID, err)
		}
		return content, nil
	}

	// 4. No scope — read and parse.
	kf, err := deps.Store.ReadKnowledge(repo, ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to read knowledge file for %s: %w", ticketID, err)
	}

	// 5. Marshal back to YAML string for output.
	content, err := yaml.Marshal(kf)
	if err != nil {
		return "", fmt.Errorf("failed to marshal knowledge for %s: %w", ticketID, err)
	}

	// 6. Staleness check.
	note := knowledge.StalenessNoteFromTime(kf.LastRefreshed, 24*time.Hour)
	if note != "" {
		return string(content) + "\n---\n" + note + "\nRun memgen__refresh to update.", nil
	}

	return string(content), nil
}
