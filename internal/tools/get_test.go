package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/poul-kg/memgen/internal/knowledge"
)

func TestGet_FileExistsFresh(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	// Write a fresh knowledge file (timestamp within 24h).
	freshTime := time.Now().UTC().Format(time.RFC3339)
	content := "# SV1-240: Feature X\n\n**Last Refreshed**: " + freshTime + "\n\n## Decisions\n- Decision 1"
	if err := store.Write("org/repo", "SV1-240", content); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	result, err := Get(deps, "org/repo", "feature/SV1-240-work")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	// Should return content without staleness note.
	if !strings.Contains(result, "SV1-240: Feature X") {
		t.Errorf("result should contain knowledge content, got: %s", result)
	}
	if strings.Contains(result, "Warning:") {
		t.Errorf("result should not contain staleness warning for fresh content, got: %s", result)
	}
}

func TestGet_FileExistsStale(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	// Write a stale knowledge file (timestamp > 24h ago).
	staleTime := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	content := "# SV1-240: Feature X\n\n**Last Refreshed**: " + staleTime + "\n\n## Decisions\n"
	if err := store.Write("org/repo", "SV1-240", content); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	result, err := Get(deps, "org/repo", "feature/SV1-240-work")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	// Should contain original content.
	if !strings.Contains(result, "SV1-240: Feature X") {
		t.Errorf("result should contain knowledge content, got: %s", result)
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
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	result, err := Get(deps, "org/repo", "feature/SV1-240-work")
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
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	_, err := Get(deps, "org/repo", "main")
	if err == nil {
		t.Fatal("expected error for branch without ticket")
	}
	if !strings.Contains(err.Error(), "no JIRA ticket detected") {
		t.Errorf("error should mention no ticket detected, got: %s", err.Error())
	}
}
