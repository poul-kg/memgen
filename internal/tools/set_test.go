package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/poul-kg/memgen/internal/claude"
	"github.com/poul-kg/memgen/internal/knowledge"
)

func TestSet_SuccessfulMerge(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	// Write an existing knowledge file.
	existingContent := "# SV1-240: Feature X\n\n**Last Refreshed**: " + time.Now().UTC().Format(time.RFC3339) + "\n\n## Decisions\n"
	if err := store.Write("org/repo", "SV1-240", existingContent); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	claudeExec := &MockClaudeExecutor{
		Response: "# SV1-240: Feature X\n\n**Last Refreshed**: 2026-04-08T12:00:00Z\n\n## Decisions\n- Use PostgreSQL for storage (2026-04-08T12:00:00Z)",
	}

	deps := &Deps{
		Store:  store,
		Locks:  knowledge.NewLockManager(),
		Claude: &claude.CLI{Executor: claudeExec},
	}

	result, err := Set(deps, "org/repo", "feature/SV1-240-work", "Use PostgreSQL for storage")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	// Verify success message.
	if !strings.Contains(result, "Updated decisions for SV1-240") {
		t.Errorf("result should mention updated decisions, got: %s", result)
	}

	// Verify file was updated.
	content, err := store.Read("org/repo", "SV1-240")
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	if !strings.Contains(content, "PostgreSQL") {
		t.Errorf("updated content should contain new decision, got: %s", content)
	}

	// Verify Claude received the correct input.
	if len(claudeExec.Calls) != 1 {
		t.Fatalf("expected 1 Claude call, got %d", len(claudeExec.Calls))
	}
	stdin := claudeExec.Calls[0].Stdin
	if !strings.Contains(stdin, existingContent) {
		t.Error("Claude stdin should contain existing knowledge")
	}
	if !strings.Contains(stdin, "Use PostgreSQL for storage") {
		t.Error("Claude stdin should contain new decisions")
	}
}

func TestSet_NoChangesNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	existingContent := "# SV1-240: Feature X\n\n## Decisions\n- Use PostgreSQL"
	if err := store.Write("org/repo", "SV1-240", existingContent); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	claudeExec := &MockClaudeExecutor{
		Response: "no changes needed",
	}

	deps := &Deps{
		Store:  store,
		Locks:  knowledge.NewLockManager(),
		Claude: &claude.CLI{Executor: claudeExec},
	}

	result, err := Set(deps, "org/repo", "feature/SV1-240-work", "Use PostgreSQL")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	if result != "no changes needed" {
		t.Errorf("result should be 'no changes needed', got: %s", result)
	}

	// Verify file was NOT updated (still has original content).
	content, err := store.Read("org/repo", "SV1-240")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if content != existingContent {
		t.Errorf("file should not have been modified")
	}
}

func TestSet_FileMissing(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	_, err := Set(deps, "org/repo", "feature/SV1-240-work", "some decisions")
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
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)
	locks := knowledge.NewLockManager()

	existingContent := "# SV1-240: Feature X\n\n## Decisions\n"
	if err := store.Write("org/repo", "SV1-240", existingContent); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Pre-lock.
	locks.TryLock(lockKey("org/repo", "SV1-240"))

	deps := &Deps{
		Store: store,
		Locks: locks,
	}

	_, err := Set(deps, "org/repo", "feature/SV1-240-work", "some decisions")
	if err == nil {
		t.Fatal("expected error due to lock contention")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("error should mention already in progress, got: %s", err.Error())
	}
}

func TestSet_NoTicketInBranch(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	_, err := Set(deps, "org/repo", "main", "some decisions")
	if err == nil {
		t.Fatal("expected error for branch without ticket")
	}
	if !strings.Contains(err.Error(), "no JIRA ticket detected") {
		t.Errorf("error should mention no ticket detected, got: %s", err.Error())
	}
}
