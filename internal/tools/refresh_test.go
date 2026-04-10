package tools

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/poul-kg/memgen/internal/knowledge"
	"github.com/poul-kg/memgen/internal/sources"
)

func makeRefreshDeps(t *testing.T, store *knowledge.Store, jiraServer *httptest.Server, ghExec *MockGHExecutor) *Deps {
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
		deps.GitHubExecutor = ghExec
	}
	return deps
}

func TestRefresh_NewDataAvailable(t *testing.T) {
	t.Parallel()
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

	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)

	// Write existing knowledge YAML with some notes.
	existingKF := &knowledge.KnowledgeFile{
		TicketID:      "SV1-240",
		Branch:        "feature/SV1-240-work",
		LastRefreshed: time.Now().UTC().Add(-2 * time.Hour),
		JIRA: knowledge.JIRASection{
			Summary:  "Feature X (old)",
			Status:   "Open",
			Labels:   []string{},
			Comments: []knowledge.JIRAComment{},
		},
		PullRequests: []knowledge.PullRequest{},
		MainCommits:  []knowledge.CommitEntry{},
		Notes: []knowledge.Note{
			{Date: time.Now().UTC().Add(-1 * time.Hour), Body: "Existing decision"},
		},
	}
	if err := store.WriteKnowledge("org/repo", "SV1-240", existingKF); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	deps := makeRefreshDeps(t, store, jiraServer, ghExec)

	result, err := Refresh(deps, "org/repo", "feature/SV1-240-work")
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	// Verify success message.
	if !strings.Contains(result, "Refreshed knowledge for SV1-240") {
		t.Errorf("result should mention refreshed knowledge, got: %s", result)
	}

	// Verify YAML was updated with fresh data.
	kf, err := store.ReadKnowledge("org/repo", "SV1-240")
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}

	// JIRA data should be refreshed.
	if kf.JIRA.Summary != "Feature X" {
		t.Errorf("JIRA.Summary = %q, want %q", kf.JIRA.Summary, "Feature X")
	}
	if kf.JIRA.Status != "In Progress" {
		t.Errorf("JIRA.Status = %q, want %q", kf.JIRA.Status, "In Progress")
	}

	// PRs should be refreshed.
	if len(kf.PullRequests) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(kf.PullRequests))
	}
	if kf.PullRequests[0].Number != 102 {
		t.Errorf("PR Number = %d, want 102", kf.PullRequests[0].Number)
	}

	// Main commits should be refreshed.
	if len(kf.MainCommits) != 1 {
		t.Fatalf("expected 1 main commit, got %d", len(kf.MainCommits))
	}
	if kf.MainCommits[0].SHA != "def567890abcdef" {
		t.Errorf("commit SHA = %q, want %q", kf.MainCommits[0].SHA, "def567890abcdef")
	}

	// Notes should be PRESERVED from the existing file.
	if len(kf.Notes) != 1 {
		t.Fatalf("expected 1 note (preserved), got %d", len(kf.Notes))
	}
	if kf.Notes[0].Body != "Existing decision" {
		t.Errorf("note body = %q, want %q", kf.Notes[0].Body, "Existing decision")
	}
}

func TestRefresh_NoNewData(t *testing.T) {
	t.Parallel()
	// JIRA returns ticket data, GitHub returns empty PRs and no matching commits.
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

	existingKF := &knowledge.KnowledgeFile{
		TicketID:      "SV1-240",
		Branch:        "feature/SV1-240-work",
		LastRefreshed: time.Now().UTC().Add(-1 * time.Hour),
		JIRA: knowledge.JIRASection{
			Summary:  "X",
			Status:   "Open",
			Labels:   []string{},
			Comments: []knowledge.JIRAComment{},
		},
		PullRequests: []knowledge.PullRequest{},
		MainCommits:  []knowledge.CommitEntry{},
		Notes:        []knowledge.Note{},
	}
	if err := store.WriteKnowledge("org/repo", "SV1-240", existingKF); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	deps := makeRefreshDeps(t, store, jiraServer, ghExec)

	result, err := Refresh(deps, "org/repo", "feature/SV1-240-work")
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	// Even with no new data, a full refresh still rebuilds the file.
	if !strings.Contains(result, "Refreshed knowledge for SV1-240") {
		t.Errorf("result should mention refreshed knowledge, got: %s", result)
	}

	// Verify file is valid YAML.
	kf, err := store.ReadKnowledge("org/repo", "SV1-240")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if kf.LastRefreshed.IsZero() {
		t.Error("LastRefreshed should be set after refresh")
	}
}

func TestRefresh_FileMissing(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	tmpDir := t.TempDir()
	store := knowledge.NewStore(tmpDir)
	locks := knowledge.NewLockManager()

	existingKF := &knowledge.KnowledgeFile{
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
	if err := store.WriteKnowledge("org/repo", "SV1-240", existingKF); err != nil {
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
		Store:          store,
		Locks:          locks,
		GitHubExecutor: ghExec,
	}

	_, err := Refresh(deps, "org/repo", "feature/SV1-240-work")
	if err == nil {
		t.Fatal("expected error due to lock contention")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("error should mention already in progress, got: %s", err.Error())
	}
}
