package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/poul-kg/memgen/internal/knowledge"
)

func TestGet_FileExistsFresh(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	// Write a fresh YAML knowledge file.
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

	result, err := Get(deps, "org/repo", "feature/SV1-240-work", "")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	// Should return content without staleness note.
	if !strings.Contains(result, "SV1-240") {
		t.Errorf("result should contain ticket ID, got: %s", result)
	}
	if strings.Contains(result, "Warning:") {
		t.Errorf("result should not contain staleness warning for fresh content, got: %s", result)
	}
}

func TestGet_FileExistsStale(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	// Write a stale YAML knowledge file (timestamp > 24h ago).
	kf := &knowledge.KnowledgeFile{
		TicketID:      "SV1-240",
		Branch:        "feature/SV1-240-work",
		LastRefreshed: time.Now().UTC().Add(-48 * time.Hour),
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

	result, err := Get(deps, "org/repo", "feature/SV1-240-work", "")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	// Should contain original content.
	if !strings.Contains(result, "SV1-240") {
		t.Errorf("result should contain ticket ID, got: %s", result)
	}
	// Should contain staleness warning.
	if !strings.Contains(result, "Warning:") {
		t.Errorf("result should contain staleness warning, got: %s", result)
	}
	// Should suggest refresh.
	if !strings.Contains(result, "memgen__refresh") {
		t.Errorf("result should suggest memgen__refresh, got: %s", result)
	}
}

func TestGet_FileMissing(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	result, err := Get(deps, "org/repo", "feature/SV1-240-work", "")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	// Should return init recommendation.
	if !strings.Contains(result, "No knowledge found for SV1-240") {
		t.Errorf("result should mention no knowledge found, got: %s", result)
	}
	if !strings.Contains(result, "memgen__init") {
		t.Errorf("result should suggest memgen__init, got: %s", result)
	}
}

func TestGet_NoTicketInBranch(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	_, err := Get(deps, "org/repo", "main", "")
	if err == nil {
		t.Fatal("expected error for branch without ticket")
	}
	if !strings.Contains(err.Error(), "no JIRA ticket detected") {
		t.Errorf("error should mention no ticket detected, got: %s", err.Error())
	}
}

func TestGet_ScopeJIRA(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	kf := &knowledge.KnowledgeFile{
		TicketID:      "SV1-240",
		Branch:        "feature/SV1-240-work",
		LastRefreshed: time.Now().UTC(),
		JIRA: knowledge.JIRASection{
			Summary:  "Feature X",
			Status:   "In Progress",
			Labels:   []string{"backend"},
			Comments: []knowledge.JIRAComment{},
		},
		PullRequests: []knowledge.PullRequest{},
		MainCommits:  []knowledge.CommitEntry{},
		Notes:        []knowledge.Note{{Date: time.Now().UTC(), Body: "A note"}},
	}
	if err := store.WriteKnowledge("org/repo", "SV1-240", kf); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	result, err := Get(deps, "org/repo", "feature/SV1-240-work", "jira")
	if err != nil {
		t.Fatalf("Get with jira scope returned error: %v", err)
	}

	if !strings.Contains(result, "Feature X") {
		t.Errorf("jira scope should contain JIRA summary, got: %s", result)
	}
	// Should NOT contain notes.
	if strings.Contains(result, "A note") {
		t.Errorf("jira scope should not contain notes, got: %s", result)
	}
}

func TestGet_ScopeNotes(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

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
		Notes:        []knowledge.Note{{Date: time.Now().UTC(), Body: "Important decision"}},
	}
	if err := store.WriteKnowledge("org/repo", "SV1-240", kf); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	result, err := Get(deps, "org/repo", "feature/SV1-240-work", "notes")
	if err != nil {
		t.Fatalf("Get with notes scope returned error: %v", err)
	}

	if !strings.Contains(result, "Important decision") {
		t.Errorf("notes scope should contain notes body, got: %s", result)
	}
	// Should NOT contain JIRA summary (scope is notes only).
	if strings.Contains(result, "Feature X") {
		t.Errorf("notes scope should not contain JIRA summary, got: %s", result)
	}
}

func TestGet_InvalidScope(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

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

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	_, err := Get(deps, "org/repo", "feature/SV1-240-work", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
	if !strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("error should mention unknown scope, got: %s", err.Error())
	}
}
