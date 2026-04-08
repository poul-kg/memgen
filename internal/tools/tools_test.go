package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/poul-kg/memgen/internal/sources"
)

func TestLockKey(t *testing.T) {
	key := lockKey("org/repo", "SV1-240")
	if key != "org/repo/SV1-240" {
		t.Errorf("lockKey = %q, want %q", key, "org/repo/SV1-240")
	}
}

func TestExtractTicket(t *testing.T) {
	id, err := extractTicket("feature/SV1-240-work")
	if err != nil {
		t.Fatalf("extractTicket returned error: %v", err)
	}
	if id != "SV1-240" {
		t.Errorf("extractTicket = %q, want %q", id, "SV1-240")
	}

	_, err = extractTicket("main")
	if err == nil {
		t.Error("expected error for branch without ticket")
	}
}

func TestFormatRawData(t *testing.T) {
	jiraTicket := &sources.JIRATicket{
		Key:         "SV1-240",
		Summary:     "Implement feature X",
		Description: "<p>Detailed description</p>",
		Status:      "In Progress",
		Priority:    "High",
		Assignee:    "Alice",
		Reporter:    "Bob",
		Labels:      []string{"backend", "api"},
		Comments: []sources.JIRAComment{
			{
				Author:  "Bob",
				Body:    "Please start with the API layer",
				Created: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	prs := []sources.PR{
		{
			Number: 123,
			Title:  "SV1-240: API implementation",
			Body:   "Implements the API",
			State:  "MERGED",
			Author: "alice",
			URL:    "https://github.com/org/repo/pull/123",
			Reviews: []sources.Review{
				{
					Author:    "bob",
					State:     "APPROVED",
					Body:      "LGTM",
					CreatedAt: time.Date(2026, 1, 16, 14, 0, 0, 0, time.UTC),
				},
			},
			Comments: []sources.PRComment{
				{
					Author:    "bob",
					Body:      "Consider adding a test here",
					Path:      "file.go",
					CreatedAt: time.Date(2026, 1, 16, 14, 30, 0, 0, time.UTC),
				},
				{
					Author:    "alice",
					Body:      "Done, added test",
					InReplyTo: 1,
					CreatedAt: time.Date(2026, 1, 16, 15, 0, 0, 0, time.UTC),
				},
			},
			Commits: []sources.Commit{
				{
					SHA:     "abc1234567890",
					Message: "SV1-240: initial implementation",
					Author:  "alice",
					Date:    time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	mainCommits := []sources.Commit{
		{
			SHA:     "def567890abcdef",
			Message: "SV1-240: merge API changes",
			Author:  "alice",
			Date:    time.Date(2026, 1, 17, 15, 0, 0, 0, time.UTC),
		},
	}

	result := formatRawData(jiraTicket, prs, mainCommits)

	// Verify JIRA ticket section.
	if !strings.Contains(result, "=== JIRA TICKET ===") {
		t.Error("missing JIRA TICKET header")
	}
	if !strings.Contains(result, "Key: SV1-240") {
		t.Error("missing ticket key")
	}
	if !strings.Contains(result, "Summary: Implement feature X") {
		t.Error("missing ticket summary")
	}
	if !strings.Contains(result, "Status: In Progress") {
		t.Error("missing ticket status")
	}
	if !strings.Contains(result, "Labels: backend, api") {
		t.Error("missing labels")
	}

	// Verify JIRA comments section.
	if !strings.Contains(result, "=== JIRA COMMENTS ===") {
		t.Error("missing JIRA COMMENTS header")
	}
	if !strings.Contains(result, "Bob: Please start with the API layer") {
		t.Error("missing JIRA comment")
	}

	// Verify PR section.
	if !strings.Contains(result, "=== PULL REQUESTS ===") {
		t.Error("missing PULL REQUESTS header")
	}
	if !strings.Contains(result, "PR #123: SV1-240: API implementation (MERGED)") {
		t.Error("missing PR header")
	}
	if !strings.Contains(result, "-- Reviews --") {
		t.Error("missing reviews section")
	}
	if !strings.Contains(result, "bob (APPROVED): LGTM") {
		t.Error("missing review content")
	}
	if !strings.Contains(result, "-- Review Comments --") {
		t.Error("missing review comments section")
	}
	if !strings.Contains(result, "bob on file.go: Consider adding a test here") {
		t.Error("missing review comment")
	}
	if !strings.Contains(result, "Reply by alice: Done, added test") {
		t.Error("missing reply comment")
	}
	if !strings.Contains(result, "-- Commits --") {
		t.Error("missing commits section")
	}
	if !strings.Contains(result, "abc1234 - SV1-240: initial implementation") {
		t.Error("missing PR commit")
	}

	// Verify main commits section.
	if !strings.Contains(result, "=== COMMITS ON MAIN ===") {
		t.Error("missing COMMITS ON MAIN header")
	}
	if !strings.Contains(result, "def5678 - SV1-240: merge API changes") {
		t.Error("missing main commit")
	}
}

func TestFormatRawData_NilJIRATicket(t *testing.T) {
	result := formatRawData(nil, nil, nil)
	if !strings.Contains(result, "=== JIRA TICKET ===") {
		t.Error("should still have JIRA TICKET header even with nil ticket")
	}
	if !strings.Contains(result, "=== PULL REQUESTS ===") {
		t.Error("should still have PULL REQUESTS header")
	}
	if !strings.Contains(result, "=== COMMITS ON MAIN ===") {
		t.Error("should still have COMMITS ON MAIN header")
	}
}

func TestFormatRawData_EmptyData(t *testing.T) {
	jiraTicket := &sources.JIRATicket{
		Key:     "TEST-1",
		Summary: "Empty ticket",
		Status:  "Open",
		Labels:  []string{},
	}
	result := formatRawData(jiraTicket, nil, nil)
	if !strings.Contains(result, "Key: TEST-1") {
		t.Error("should contain ticket key")
	}
	// Should not have Labels line when empty.
	if strings.Contains(result, "Labels:") {
		t.Error("should not have Labels line when empty")
	}
}

func TestUpdateLastRefreshed(t *testing.T) {
	content := "# SV1-240: Feature X\n\n**Last Refreshed**: 2026-01-01T00:00:00Z\n\n## Decisions\n"
	updated := updateLastRefreshed(content)

	// Should still contain the header.
	if !strings.Contains(updated, "# SV1-240: Feature X") {
		t.Error("should preserve content header")
	}
	// Should have an updated timestamp.
	if strings.Contains(updated, "2026-01-01T00:00:00Z") {
		t.Error("should have replaced the old timestamp")
	}
	if !strings.Contains(updated, "**Last Refreshed**: ") {
		t.Error("should still contain Last Refreshed prefix")
	}
	// Decisions section should be preserved.
	if !strings.Contains(updated, "## Decisions") {
		t.Error("should preserve Decisions section")
	}
}

func TestUpdateLastRefreshed_NoExistingTimestamp(t *testing.T) {
	content := "# SV1-240: Feature X\n\n## Decisions\n"
	updated := updateLastRefreshed(content)

	if !strings.Contains(updated, "**Last Refreshed**: ") {
		t.Error("should add a Last Refreshed line")
	}
	if !strings.Contains(updated, "# SV1-240: Feature X") {
		t.Error("should preserve content")
	}
}
