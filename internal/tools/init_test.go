package tools

import (
	"fmt"
	"strings"
	"testing"
)

func TestInit_Success(t *testing.T) {
	ticket := &jiraTestTicket{
		Key:         "SV1-240",
		Summary:     "Implement feature X",
		Description: "<p>Detailed description</p>",
		Status:      "In Progress",
		Priority:    "High",
		Assignee:    "Alice",
		Reporter:    "Bob",
		Labels:      []string{"backend"},
	}
	comments := []jiraTestComment{
		{
			Author:  "Bob",
			Body:    "Please start with the API layer",
			Created: "2026-01-15T10:00:00.000+0000",
			Updated: "2026-01-15T10:00:00.000+0000",
		},
	}

	jiraServer := newJIRATestServer(t, "SV1-240", ticket, comments)
	defer jiraServer.Close()

	prList := ghPRListJSON([]ghPREntry{
		{
			Number:      101,
			Title:       "SV1-240: API implementation",
			Body:        "Implements the API for SV1-240",
			State:       "MERGED",
			Author:      ghAuth{Login: "alice"},
			CreatedAt:   "2026-01-16T12:00:00Z",
			UpdatedAt:   "2026-01-17T14:00:00Z",
			HeadRefName: "feature/SV1-240-api",
			URL:         "https://github.com/org/repo/pull/101",
		},
	})

	mainCommits := ghCommitsJSON([]ghCommitJSON{
		{
			SHA: "abc1234567890",
			Commit: ghCommitData{
				Message: "SV1-240: merge API changes",
				Author:  ghAuthorData{Name: "Alice", Email: "alice@example.com", Date: "2026-01-17T15:00:00Z"},
			},
			Author: &ghAuthJSON{Login: "alice"},
		},
	})

	ghExec := &MockGHExecutor{
		Responses: map[string]MockGHResponse{
			"gh pr list": {Stdout: prList},
			"gh api repos/org/repo/pulls/101/reviews":  {Stdout: "[]"},
			"gh api repos/org/repo/pulls/101/comments": {Stdout: "[]"},
			"gh api repos/org/repo/pulls/101/commits":  {Stdout: "[]"},
			"gh api repos/org/repo/commits":            {Stdout: mainCommits},
		},
	}

	claudeExec := &MockClaudeExecutor{
		Response: "# SV1-240: Implement feature X\n\n**Last Refreshed**: 2026-01-17T15:00:00Z\n\n## Decisions\n",
	}

	deps := testDeps(t, jiraServer, ghExec, claudeExec, "org/repo")

	result, err := Init(deps, "org/repo", "feature/SV1-240-api")
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	// Verify success message.
	if !strings.Contains(result, "SV1-240") {
		t.Errorf("result should mention ticket ID, got: %s", result)
	}
	if !strings.Contains(result, "1 PRs") {
		t.Errorf("result should mention 1 PR, got: %s", result)
	}
	if !strings.Contains(result, "1 commits on main") {
		t.Errorf("result should mention 1 commit, got: %s", result)
	}

	// Verify file was written.
	if !deps.Store.Exists("org/repo", "SV1-240") {
		t.Error("knowledge file should exist after init")
	}
	content, err := deps.Store.Read("org/repo", "SV1-240")
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if !strings.Contains(content, "SV1-240") {
		t.Errorf("written content should contain ticket ID, got: %s", content)
	}

	// Verify Claude was called with raw data containing JIRA ticket info.
	if len(claudeExec.Calls) != 1 {
		t.Fatalf("expected 1 Claude call, got %d", len(claudeExec.Calls))
	}
	stdin := claudeExec.Calls[0].Stdin
	if !strings.Contains(stdin, "SV1-240") {
		t.Error("Claude stdin should contain ticket ID")
	}
	if !strings.Contains(stdin, "Implement feature X") {
		t.Error("Claude stdin should contain ticket summary")
	}
}

func TestInit_LockContention(t *testing.T) {
	ticket := &jiraTestTicket{
		Key:     "SV1-100",
		Summary: "Test",
		Status:  "Open",
	}
	jiraServer := newJIRATestServer(t, "SV1-100", ticket, nil)
	defer jiraServer.Close()

	ghExec := &MockGHExecutor{
		Default: MockGHResponse{Stdout: "[]"},
	}
	claudeExec := &MockClaudeExecutor{Response: "knowledge content"}
	deps := testDeps(t, jiraServer, ghExec, claudeExec, "org/repo")

	// Pre-lock the ticket.
	deps.Locks.TryLock(lockKey("org/repo", "SV1-100"))

	_, err := Init(deps, "org/repo", "feature/SV1-100-work")
	if err == nil {
		t.Fatal("expected error due to lock contention")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("error should mention already in progress, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "SV1-100") {
		t.Errorf("error should mention ticket ID, got: %s", err.Error())
	}
}

func TestInit_NoTicketInBranch(t *testing.T) {
	deps := testDeps(t, nil, nil, nil, "org/repo")

	_, err := Init(deps, "org/repo", "main")
	if err == nil {
		t.Fatal("expected error for branch without ticket")
	}
	if !strings.Contains(err.Error(), "no JIRA ticket detected") {
		t.Errorf("error should mention no ticket detected, got: %s", err.Error())
	}
}

func TestInit_JIRAFailure(t *testing.T) {
	jiraServer := newJIRAFailServer(404)
	defer jiraServer.Close()

	ghExec := &MockGHExecutor{Default: MockGHResponse{Stdout: "[]"}}
	claudeExec := &MockClaudeExecutor{Response: "knowledge"}
	deps := testDeps(t, jiraServer, ghExec, claudeExec, "org/repo")

	_, err := Init(deps, "org/repo", "feature/SV1-300-test")
	if err == nil {
		t.Fatal("expected error when JIRA fails")
	}
	if !strings.Contains(err.Error(), "SV1-300") {
		t.Errorf("error should mention ticket ID, got: %s", err.Error())
	}
	// Should contain the browse URL.
	if !strings.Contains(err.Error(), "browse") {
		t.Errorf("error should contain browse URL, got: %s", err.Error())
	}
}

func TestInit_GitHubFailure_FailsEarly(t *testing.T) {
	ticket := &jiraTestTicket{
		Key:     "SV1-400",
		Summary: "Test early failure",
		Status:  "Open",
	}
	jiraServer := newJIRATestServer(t, "SV1-400", ticket, nil)
	defer jiraServer.Close()

	// GitHub commands all fail.
	ghExec := &MockGHExecutor{
		Default: MockGHResponse{
			Stderr: "gh: Could not resolve repo",
			Err:    fmt.Errorf("exit status 1"),
		},
	}

	claudeExec := &MockClaudeExecutor{}

	deps := testDeps(t, jiraServer, ghExec, claudeExec, "org/repo")

	_, err := Init(deps, "org/repo", "feature/SV1-400-work")
	if err == nil {
		t.Fatal("Init should fail when GitHub repo is not accessible")
	}

	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "GitHub") {
		t.Errorf("error should mention GitHub repo issue, got: %v", err)
	}

	// File should NOT be written.
	if deps.Store.Exists("org/repo", "SV1-400") {
		t.Error("knowledge file should not exist when init fails early")
	}
}
