package tools

import (
	"fmt"
	"time"

	"github.com/poul-kg/memgen/internal/knowledge"
)

// Set appends a note to an existing knowledge file.
func Set(deps *Deps, repo, branch, note string) (string, error) {
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
	defer deps.Locks.Unlock(key)

	// 4. Read existing knowledge as struct.
	kf, err := deps.Store.ReadKnowledge(repo, ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to read knowledge file for %s: %w", ticketID, err)
	}

	// 5. Append note.
	kf.Notes = append(kf.Notes, knowledge.Note{
		Date: time.Now().UTC(),
		Body: note,
	})

	// 6. Write back.
	if err := deps.Store.WriteKnowledge(repo, ticketID, kf); err != nil {
		return "", fmt.Errorf("failed to write knowledge file: %w", err)
	}

	// 7. Return success.
	return fmt.Sprintf("Added note for %s.", ticketID), nil
}
