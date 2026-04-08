package tools

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/poul-kg/memgen/internal/claude"
	"github.com/poul-kg/memgen/internal/knowledge"
	"github.com/poul-kg/memgen/internal/sources"
)

func makeRefreshDeps(t *testing.T, store *knowledge.Store, jiraServer *httptest.Server, ghExec *MockGHExecutor, claudeExec *MockClaudeExecutor) *Deps {
	t.Helper()
	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}
	if jiraServer != nil {
		deps.JIRA = &sources.JIRAClient{
			BaseURL:    jiraServer.URL,
			Email:      "test@example.com",
			Token:      "test-token",
			HTTPClient: jiraServer.Client(),
		}
		deps.JIRABaseURL = jiraServer.URL
	}
	if ghExec != nil {
		deps.GitHub = &sources.GitHubClient{
			Repo:     "org/repo",
			Executor: ghExec,
		}
	}
	if claudeExec != nil {
		deps.Claude = &claude.CLI{Executor: claudeExec}
	}
	return deps
}

func TestRefresh_NewDataAvailable(t *testing.T) {
	ticket := &jiraTestTicket{
		Key:     "SV1-240",
		Summary: "Feature X",
		Status:  "In Progress",
	}
	newComment := jiraTestComment{
		Author:  "Carol",
		Body:    "New update on the feature",
		Created: "2026-04-08T10:00:00.000+0000",
		Updated: "2026-04-08T10:00:00.000+0000",
	}
	jiraServer := newJIRATestServer(t, "SV1-240", ticket, []jiraTestComment{newComment})
	defer jiraServer.Close()

	// GitHub returns a new PR updated since last refresh.
	prList := ghPRListJSON([]ghPREntry{
		{
			Number:      102,
			Title:       "SV1-240: Fix bug",
			Body:        "Bug fix",
			State:       "OPEN",
			Author:      ghAuth{Login: "bob"},
			CreatedAt:   "2026-04-08T09:00:00Z",
			UpdatedAt:   "2026-04-08T11:00:00Z",
			HeadRefName: "fix/SV1-240-bug",
			URL:         "https://github.com/org/repo/pull/102",
		},
	})

	newMainCommit := ghCommitsJSON([]ghCommitJSON{
		{
			SHA: "def567890abcdef",
			Commit: ghCommitData{
				Message: "SV1-240: hotfix on main",
				Author:  ghAuthorData{Name: "Bob", Date: "2026-04-08T12:00:00Z"},
			},
			Author: &ghAuthJSON{Login: "bob"},
		},
	})

	ghExec := &MockGHExecutor{
		Responses: map[string]MockGHResponse{
			"gh pr list": {Stdout: prList},
			"gh api repos/org/repo/pulls/102/reviews":  {Stdout: "[]"},
			"gh api repos/org/repo/pulls/102/comments": {Stdout: "[]"},
			"gh api repos/org/repo/pulls/102/commits":  {Stdout: "[]"},
			"gh api repos/org/repo/commits":            {Stdout: newMainCommit},
		},
	}

	claudeExec := &MockClaudeExecutor{
		Response: "# SV1-240: Feature X\n\n**Last Refreshed**: 2026-04-08T14:00:00Z\n\n## Decisions\n- Existing decision\n\n## New data integrated",
	}

	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	// Write existing knowledge with a Last Refreshed timestamp.
	lastRefreshed := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	existingContent := "# SV1-240: Feature X\n\n**Last Refreshed**: " + lastRefreshed + "\n\n## Decisions\n- Existing decision"
	if err := store.Write("org/repo", "SV1-240", existingContent); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	deps := makeRefreshDeps(t, store, jiraServer, ghExec, claudeExec)

	result, err := Refresh(deps, "org/repo", "feature/SV1-240-work")
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	// Verify success message.
	if !strings.Contains(result, "Refreshed knowledge for SV1-240") {
		t.Errorf("result should mention refreshed knowledge, got: %s", result)
	}

	// Verify file was updated.
	content, err := store.Read("org/repo", "SV1-240")
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	if !strings.Contains(content, "New data integrated") {
		t.Errorf("updated content should contain Claude's output, got: %s", content)
	}

	// Verify Claude was called with existing knowledge and new raw data.
	if len(claudeExec.Calls) != 1 {
		t.Fatalf("expected 1 Claude call, got %d", len(claudeExec.Calls))
	}
	stdin := claudeExec.Calls[0].Stdin
	if !strings.Contains(stdin, "Existing decision") {
		t.Error("Claude stdin should contain existing knowledge")
	}
}

func TestRefresh_NoNewData(t *testing.T) {
	// JIRA returns no comments since last refresh, GitHub returns empty.
	jiraServer := newJIRATestServer(t, "SV1-240", &jiraTestTicket{Key: "SV1-240", Summary: "X", Status: "Open"}, nil)
	defer jiraServer.Close()

	ghExec := &MockGHExecutor{
		Responses: map[string]MockGHResponse{
			"gh pr list": {Stdout: "[]"},
			"gh api":     {Stdout: "[]"},
		},
	}

	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	lastRefreshed := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	existingContent := "# SV1-240: Feature X\n\n**Last Refreshed**: " + lastRefreshed + "\n\n## Decisions\n"
	if err := store.Write("org/repo", "SV1-240", existingContent); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	claudeExec := &MockClaudeExecutor{}
	deps := makeRefreshDeps(t, store, jiraServer, ghExec, claudeExec)

	result, err := Refresh(deps, "org/repo", "feature/SV1-240-work")
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	if result != "Knowledge is up to date." {
		t.Errorf("result should be 'Knowledge is up to date.', got: %s", result)
	}

	// Verify Claude was NOT called (no new data to process).
	if len(claudeExec.Calls) != 0 {
		t.Errorf("expected 0 Claude calls when no new data, got %d", len(claudeExec.Calls))
	}

	// Verify the Last Refreshed timestamp was updated in the file.
	content, err := store.Read("org/repo", "SV1-240")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	updatedTime := store.LastRefreshed(content)
	if updatedTime.IsZero() {
		t.Error("Last Refreshed timestamp should be present after refresh")
	}
}

func TestRefresh_FileMissing(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	deps := &Deps{
		Store: store,
		Locks: knowledge.NewLockManager(),
	}

	_, err := Refresh(deps, "org/repo", "feature/SV1-240-work")
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

func TestRefresh_LockContention(t *testing.T) {
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)
	locks := knowledge.NewLockManager()

	existingContent := "# SV1-240: Feature X\n\n## Decisions\n"
	if err := store.Write("org/repo", "SV1-240", existingContent); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Pre-lock.
	locks.TryLock(lockKey("org/repo", "SV1-240"))

	ghExec := &MockGHExecutor{
		Responses: map[string]MockGHResponse{
			"gh repo view org/repo --json name": {Stdout: `{"name":"repo"}`},
		},
	}

	deps := &Deps{
		Store:  store,
		Locks:  locks,
		GitHub: &sources.GitHubClient{Repo: "org/repo", Executor: ghExec},
	}

	_, err := Refresh(deps, "org/repo", "feature/SV1-240-work")
	if err == nil {
		t.Fatal("expected error due to lock contention")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("error should mention already in progress, got: %s", err.Error())
	}
}
