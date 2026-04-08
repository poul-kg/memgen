package tools

import (
	"fmt"
	"strings"
	"time"
)

// Set merges new decisions into an existing knowledge file.
func Set(deps *Deps, repo, branch, decisions string) (string, error) {
	// 1. Extract ticket ID.
	ticketID, err := extractTicket(branch)
	if err != nil {
		return "", err
	}

	// 2. If file doesn't exist, return error.
	if !deps.Store.Exists(repo, ticketID) {
		return "", fmt.Errorf("No knowledge file found for %s. Run memgen__init first.", ticketID)
	}

	// 3. Try lock.
	key := lockKey(repo, ticketID)
	if !deps.Locks.TryLock(key) {
		return "", fmt.Errorf("An operation is already in progress for %s. Try again later.", ticketID)
	}
	// 4. Defer unlock.
	defer deps.Locks.Unlock(key)

	// 5. Read existing knowledge.
	existing, err := deps.Store.Read(repo, ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to read knowledge file for %s: %w", ticketID, err)
	}

	// 6. Call Claude MergeDecisions with current UTC time.
	currentTime := time.Now().UTC().Format(time.RFC3339)
	result, err := deps.Claude.MergeDecisions(existing, decisions, currentTime)
	if err != nil {
		return "", fmt.Errorf("failed to merge decisions via Claude: %w", err)
	}

	// 7. If result is "no changes needed", return that.
	if strings.TrimSpace(result) == "no changes needed" {
		return "no changes needed", nil
	}

	// 8. Write merged result.
	if err := deps.Store.Write(repo, ticketID, result); err != nil {
		return "", fmt.Errorf("failed to write updated knowledge file: %w", err)
	}

	// 9. Return success summary.
	return fmt.Sprintf("Updated decisions for %s.", ticketID), nil
}
