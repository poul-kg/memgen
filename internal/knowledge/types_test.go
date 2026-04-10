package knowledge

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestKnowledgeFile_YAMLRoundTrip(t *testing.T) {
	t.Parallel()

	original := &KnowledgeFile{
		TicketID:      "SV1-100",
		Branch:        "feature/SV1-100-login",
		LastRefreshed: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		JIRA: JIRASection{
			Summary:     "Build login page",
			Description: "A detailed\nmultiline\ndescription",
			Status:      "In Progress",
			Priority:    "High",
			Assignee:    "Alice",
			Reporter:    "Bob",
			Labels:      []string{"frontend", "auth"},
			Comments: []JIRAComment{
				{
					Author:  "Bob",
					Created: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
					Updated: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
					Body:    "Start with the form",
				},
			},
		},
		PullRequests: []PullRequest{
			{
				Number:    42,
				Title:     "SV1-100: Login form",
				State:     "OPEN",
				Author:    "alice",
				URL:       "https://github.com/org/repo/pull/42",
				CreatedAt: time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 1, 17, 14, 0, 0, 0, time.UTC),
				Branch:    "feature/SV1-100-login",
				Body:      "Adds login form",
				Reviews: []PRReview{
					{Author: "bob", State: "APPROVED", Body: "LGTM", CreatedAt: time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC)},
				},
				Comments: []PRComment{
					{ID: 1, Author: "bob", Body: "Nice work", Path: "login.go", CreatedAt: time.Date(2026, 1, 17, 11, 0, 0, 0, time.UTC), Resolved: true},
				},
				Commits: []CommitEntry{
					{SHA: "abc123", Message: "SV1-100: add form", Author: "alice", Date: time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)},
				},
			},
		},
		MainCommits: []CommitEntry{
			{SHA: "def456", Message: "SV1-100: merge login", Author: "alice", Date: time.Date(2026, 1, 17, 15, 0, 0, 0, time.UTC)},
		},
		Notes: []Note{
			{Date: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC), Body: "Use PostgreSQL for storage"},
		},
	}

	// Marshal to YAML.
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal KnowledgeFile: %v", err)
	}

	// Unmarshal back.
	var roundTripped KnowledgeFile
	if err := yaml.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("failed to unmarshal KnowledgeFile: %v", err)
	}

	// Verify top-level fields.
	if roundTripped.TicketID != original.TicketID {
		t.Errorf("TicketID = %q, want %q", roundTripped.TicketID, original.TicketID)
	}
	if roundTripped.Branch != original.Branch {
		t.Errorf("Branch = %q, want %q", roundTripped.Branch, original.Branch)
	}
	if !roundTripped.LastRefreshed.Equal(original.LastRefreshed) {
		t.Errorf("LastRefreshed = %v, want %v", roundTripped.LastRefreshed, original.LastRefreshed)
	}

	// Verify JIRA section.
	if roundTripped.JIRA.Summary != original.JIRA.Summary {
		t.Errorf("JIRA.Summary = %q, want %q", roundTripped.JIRA.Summary, original.JIRA.Summary)
	}
	if roundTripped.JIRA.Description != original.JIRA.Description {
		t.Errorf("JIRA.Description = %q, want %q", roundTripped.JIRA.Description, original.JIRA.Description)
	}
	if len(roundTripped.JIRA.Labels) != len(original.JIRA.Labels) {
		t.Errorf("JIRA.Labels len = %d, want %d", len(roundTripped.JIRA.Labels), len(original.JIRA.Labels))
	}
	if len(roundTripped.JIRA.Comments) != len(original.JIRA.Comments) {
		t.Errorf("JIRA.Comments len = %d, want %d", len(roundTripped.JIRA.Comments), len(original.JIRA.Comments))
	}

	// Verify PRs.
	if len(roundTripped.PullRequests) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(roundTripped.PullRequests))
	}
	pr := roundTripped.PullRequests[0]
	if pr.Number != 42 {
		t.Errorf("PR.Number = %d, want 42", pr.Number)
	}
	if len(pr.Reviews) != 1 {
		t.Errorf("PR.Reviews len = %d, want 1", len(pr.Reviews))
	}
	if len(pr.Comments) != 1 {
		t.Errorf("PR.Comments len = %d, want 1", len(pr.Comments))
	}
	if !pr.Comments[0].Resolved {
		t.Error("PR comment Resolved should be true after round-trip")
	}
	if pr.Comments[0].ID != 1 {
		t.Errorf("PR comment ID = %d, want 1", pr.Comments[0].ID)
	}

	// Verify main commits.
	if len(roundTripped.MainCommits) != 1 {
		t.Errorf("MainCommits len = %d, want 1", len(roundTripped.MainCommits))
	}

	// Verify notes.
	if len(roundTripped.Notes) != 1 {
		t.Fatalf("Notes len = %d, want 1", len(roundTripped.Notes))
	}
	if roundTripped.Notes[0].Body != "Use PostgreSQL for storage" {
		t.Errorf("Note.Body = %q, want %q", roundTripped.Notes[0].Body, "Use PostgreSQL for storage")
	}
}

func TestKnowledgeFile_EmptySlicesRoundTrip(t *testing.T) {
	t.Parallel()

	original := &KnowledgeFile{
		TicketID:      "SV1-200",
		Branch:        "feature/SV1-200-work",
		LastRefreshed: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		JIRA: JIRASection{
			Summary:  "Empty ticket",
			Labels:   []string{},
			Comments: []JIRAComment{},
		},
		PullRequests: []PullRequest{},
		MainCommits:  []CommitEntry{},
		Notes:        []Note{},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var roundTripped KnowledgeFile
	if err := yaml.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if roundTripped.TicketID != "SV1-200" {
		t.Errorf("TicketID = %q, want %q", roundTripped.TicketID, "SV1-200")
	}
	// YAML empty slices unmarshal to nil; the ReadKnowledge method handles this.
	// Here we just verify the basic structure survives round-trip.
	if roundTripped.JIRA.Summary != "Empty ticket" {
		t.Errorf("JIRA.Summary = %q, want %q", roundTripped.JIRA.Summary, "Empty ticket")
	}
}
