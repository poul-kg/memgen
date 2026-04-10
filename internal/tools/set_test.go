package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/poul-kg/memgen/internal/knowledge"
)

func TestSet_AppendsNote(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	// Write an existing YAML knowledge file.
	kf := &knowledge.KnowledgeFile{
		TicketID:      "SV1-240",
		Branch:        "feature/SV1-240-work",
		LastRefreshed: time.Now().UTC(),
		JIRA: knowledge.JIRASection{
			Summary:  "Feature X",
			Status:   "In Progress",
			Labels:   []string{},
			Comments: []knowledge.JIRAComment{},
		},
		PullRequests: []knowledge.PullRequest{},
		MainCommits:  []knowledge.CommitEntry{},
		Notes:        []knowledge.Note{},
	}
	if err := store.WriteKnowledge("org/repo", "SV1-240", kf); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	before := time.Now().UTC()
	result, err := Set(deps, "org/repo", "feature/SV1-240-work", "Use PostgreSQL for storage")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	// Verify success message.
	if !strings.Contains(result, "Added note for SV1-240") {
		t.Errorf("result should mention added note, got: %s", result)
	}

	// Verify note was appended by reading back.
	updated, err := store.ReadKnowledge("org/repo", "SV1-240")
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	if len(updated.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(updated.Notes))
	}
	if updated.Notes[0].Body != "Use PostgreSQL for storage" {
		t.Errorf("note body = %q, want %q", updated.Notes[0].Body, "Use PostgreSQL for storage")
	}
	if updated.Notes[0].Date.Before(before) {
		t.Errorf("note date should be recent, got: %v", updated.Notes[0].Date)
	}
}

func TestSet_AppendsMultipleNotes(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	// Write an existing YAML knowledge file with one note.
	kf := &knowledge.KnowledgeFile{
		TicketID:      "SV1-240",
		Branch:        "feature/SV1-240-work",
		LastRefreshed: time.Now().UTC(),
		JIRA: knowledge.JIRASection{
			Summary:  "Feature X",
			Labels:   []string{},
			Comments: []knowledge.JIRAComment{},
		},
		PullRequests: []knowledge.PullRequest{},
		MainCommits:  []knowledge.CommitEntry{},
		Notes: []knowledge.Note{
			{Date: time.Now().UTC().Add(-1 * time.Hour), Body: "First note"},
		},
	}
	if err := store.WriteKnowledge("org/repo", "SV1-240", kf); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	_, err := Set(deps, "org/repo", "feature/SV1-240-work", "Second note")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	updated, err := store.ReadKnowledge("org/repo", "SV1-240")
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	if len(updated.Notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(updated.Notes))
	}
	if updated.Notes[0].Body != "First note" {
		t.Errorf("first note body = %q, want %q", updated.Notes[0].Body, "First note")
	}
	if updated.Notes[1].Body != "Second note" {
		t.Errorf("second note body = %q, want %q", updated.Notes[1].Body, "Second note")
	}
}

func TestSet_FileMissing(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	_, err := Set(deps, "org/repo", "feature/SV1-240-work", "some note")
	if err == nil {
		t.Fatal("expected error when file doesn't exist")
	}
	if !strings.Contains(err.Error(), "No knowledge file found for SV1-240") {
		t.Errorf("error should mention no knowledge file, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "memgen__init") {
		t.Errorf("error should suggest memgen__init, got: %s", err.Error())
	}
}

func TestSet_LockContention(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)
	locks := knowledge.NewLockManager()

	kf := &knowledge.KnowledgeFile{
		TicketID:      "SV1-240",
		Branch:        "feature/SV1-240-work",
		LastRefreshed: time.Now().UTC(),
		JIRA: knowledge.JIRASection{
			Labels:   []string{},
			Comments: []knowledge.JIRAComment{},
		},
		PullRequests: []knowledge.PullRequest{},
		MainCommits:  []knowledge.CommitEntry{},
		Notes:        []knowledge.Note{},
	}
	if err := store.WriteKnowledge("org/repo", "SV1-240", kf); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Pre-lock.
	locks.TryLock(lockKey("org/repo", "SV1-240"))

	deps := &Deps{
		Store: store,
		Locks: locks,
	}

	_, err := Set(deps, "org/repo", "feature/SV1-240-work", "some note")
	if err == nil {
		t.Fatal("expected error due to lock contention")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("error should mention already in progress, got: %s", err.Error())
	}
}

func TestSet_NoTicketInBranch(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	_, err := Set(deps, "org/repo", "main", "some note")
	if err == nil {
		t.Fatal("expected error for branch without ticket")
	}
	if !strings.Contains(err.Error(), "no JIRA ticket detected") {
		t.Errorf("error should mention no ticket detected, got: %s", err.Error())
	}
}
