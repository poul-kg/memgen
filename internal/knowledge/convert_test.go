package knowledge

import (
	"testing"
	"time"

	"github.com/poul-kg/memgen/internal/sources"
)

func TestFromSources_FullData(t *testing.T) {
	t.Parallel()
	jira := &sources.JIRATicket{
		Key:         "SV1-100",
		Summary:     "Build login page",
		Description: "Detailed description of the login page",
		Status:      "In Progress",
		Priority:    "High",
		Assignee:    "Alice",
		Reporter:    "Bob",
		Labels:      []string{"frontend", "auth"},
		Comments: []sources.JIRAComment{
			{
				Author:  "Bob",
				Body:    "Start with the form",
				Created: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
				Updated: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	prs := []sources.PR{
		{
			Number:    42,
			Title:     "SV1-100: Login form",
			Body:      "Adds login form",
			State:     "OPEN",
			Author:    "alice",
			URL:       "https://github.com/org/repo/pull/42",
			CreatedAt: time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 1, 17, 14, 0, 0, 0, time.UTC),
			Branch:    "feature/SV1-100-login",
			Reviews: []sources.Review{
				{Author: "bob", State: "APPROVED", Body: "LGTM", CreatedAt: time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC)},
			},
			Comments: []sources.PRComment{
				{ID: 1, Author: "bob", Body: "Nice work", Path: "login.go", CreatedAt: time.Date(2026, 1, 17, 11, 0, 0, 0, time.UTC), Resolved: true},
			},
			Commits: []sources.Commit{
				{SHA: "abc123", Message: "SV1-100: add form", Author: "alice", Date: time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)},
			},
		},
	}

	mainCommits := []sources.Commit{
		{SHA: "def456", Message: "SV1-100: merge login", Author: "alice", Date: time.Date(2026, 1, 17, 15, 0, 0, 0, time.UTC)},
	}

	kf := FromSources("SV1-100", "feature/SV1-100-login", jira, prs, mainCommits)

	if kf.TicketID != "SV1-100" {
		t.Errorf("TicketID = %q, want %q", kf.TicketID, "SV1-100")
	}
	if kf.Branch != "feature/SV1-100-login" {
		t.Errorf("Branch = %q, want %q", kf.Branch, "feature/SV1-100-login")
	}
	if kf.LastRefreshed.IsZero() {
		t.Error("LastRefreshed should not be zero")
	}

	// JIRA section.
	if kf.JIRA.Summary != "Build login page" {
		t.Errorf("JIRA.Summary = %q, want %q", kf.JIRA.Summary, "Build login page")
	}
	if kf.JIRA.Status != "In Progress" {
		t.Errorf("JIRA.Status = %q, want %q", kf.JIRA.Status, "In Progress")
	}
	if len(kf.JIRA.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(kf.JIRA.Labels))
	}
	if len(kf.JIRA.Comments) != 1 {
		t.Errorf("expected 1 JIRA comment, got %d", len(kf.JIRA.Comments))
	}

	// PRs.
	if len(kf.PullRequests) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(kf.PullRequests))
	}
	pr := kf.PullRequests[0]
	if pr.Number != 42 {
		t.Errorf("PR.Number = %d, want 42", pr.Number)
	}
	if len(pr.Reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(pr.Reviews))
	}
	if len(pr.Comments) != 1 {
		t.Errorf("expected 1 PR comment, got %d", len(pr.Comments))
	}
	if !pr.Comments[0].Resolved {
		t.Error("PR comment should be resolved")
	}
	if len(pr.Commits) != 1 {
		t.Errorf("expected 1 PR commit, got %d", len(pr.Commits))
	}

	// Main commits.
	if len(kf.MainCommits) != 1 {
		t.Errorf("expected 1 main commit, got %d", len(kf.MainCommits))
	}

	// Notes should be empty (not nil).
	if kf.Notes == nil {
		t.Error("Notes should not be nil")
	}
	if len(kf.Notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(kf.Notes))
	}
}

func TestFromSources_NilJIRA(t *testing.T) {
	t.Parallel()
	kf := FromSources("SV1-200", "feature/SV1-200-work", nil, nil, nil)

	if kf.TicketID != "SV1-200" {
		t.Errorf("TicketID = %q, want %q", kf.TicketID, "SV1-200")
	}

	// JIRA section should have empty slices, not nil.
	if kf.JIRA.Labels == nil {
		t.Error("JIRA.Labels should not be nil")
	}
	if kf.JIRA.Comments == nil {
		t.Error("JIRA.Comments should not be nil")
	}
	if kf.JIRA.Summary != "" {
		t.Errorf("JIRA.Summary = %q, want empty", kf.JIRA.Summary)
	}

	// All slices should be non-nil.
	if kf.PullRequests == nil {
		t.Error("PullRequests should not be nil")
	}
	if kf.MainCommits == nil {
		t.Error("MainCommits should not be nil")
	}
	if kf.Notes == nil {
		t.Error("Notes should not be nil")
	}
}

func TestFromSources_EmptySlices(t *testing.T) {
	t.Parallel()
	jira := &sources.JIRATicket{
		Key:     "SV1-300",
		Summary: "Empty ticket",
		Status:  "Open",
		Labels:  []string{},
	}

	kf := FromSources("SV1-300", "feature/SV1-300-work", jira, []sources.PR{}, []sources.Commit{})

	if len(kf.PullRequests) != 0 {
		t.Errorf("expected 0 PRs, got %d", len(kf.PullRequests))
	}
	if len(kf.MainCommits) != 0 {
		t.Errorf("expected 0 main commits, got %d", len(kf.MainCommits))
	}
	if len(kf.JIRA.Labels) != 0 {
		t.Errorf("expected 0 labels, got %d", len(kf.JIRA.Labels))
	}
}
