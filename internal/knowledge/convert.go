package knowledge

import (
	"time"

	"github.com/poul-kg/memgen/internal/sources"
)

// FromSources creates a KnowledgeFile from source data (JIRA, GitHub PRs, main commits).
// Handles nil jira and nil/empty slices gracefully by initializing empty slices.
func FromSources(ticketID, branch string, jira *sources.JIRATicket, prs []sources.PR, mainCommits []sources.Commit) *KnowledgeFile {
	kf := &KnowledgeFile{
		TicketID:      ticketID,
		Branch:        branch,
		LastRefreshed: time.Now().UTC(),
		JIRA:          convertJIRA(jira),
		PullRequests:  convertPRs(prs),
		MainCommits:   convertCommits(mainCommits),
		Notes:         []Note{},
	}
	return kf
}

// convertJIRA converts a sources.JIRATicket to a JIRASection.
// Returns an empty JIRASection with initialized slices if jira is nil.
func convertJIRA(jira *sources.JIRATicket) JIRASection {
	if jira == nil {
		return JIRASection{
			Labels:   []string{},
			Comments: []JIRAComment{},
		}
	}

	labels := jira.Labels
	if labels == nil {
		labels = []string{}
	}

	comments := make([]JIRAComment, 0, len(jira.Comments))
	for _, c := range jira.Comments {
		comments = append(comments, JIRAComment{
			Author:  c.Author,
			Created: c.Created,
			Updated: c.Updated,
			Body:    c.Body,
		})
	}
	return JIRASection{
		Summary:     jira.Summary,
		Description: jira.Description,
		Status:      jira.Status,
		Priority:    jira.Priority,
		Assignee:    jira.Assignee,
		Reporter:    jira.Reporter,
		Labels:      labels,
		Comments:    comments,
	}
}

// convertPRs converts a slice of sources.PR to a slice of PullRequest.
func convertPRs(prs []sources.PR) []PullRequest {
	result := make([]PullRequest, 0, len(prs))
	for _, pr := range prs {
		result = append(result, convertPR(pr))
	}
	return result
}

// convertPR converts a single sources.PR to a PullRequest.
func convertPR(pr sources.PR) PullRequest {
	reviews := make([]PRReview, 0, len(pr.Reviews))
	for _, r := range pr.Reviews {
		reviews = append(reviews, PRReview{
			Author:    r.Author,
			State:     r.State,
			Body:      r.Body,
			CreatedAt: r.CreatedAt,
		})
	}

	comments := make([]PRComment, 0, len(pr.Comments))
	for _, c := range pr.Comments {
		comments = append(comments, PRComment{
			ID:        c.ID,
			Author:    c.Author,
			Body:      c.Body,
			Path:      c.Path,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			InReplyTo: c.InReplyTo,
			Resolved:  c.Resolved,
		})
	}

	commits := convertCommits(pr.Commits)

	return PullRequest{
		Number:    pr.Number,
		Title:     pr.Title,
		State:     pr.State,
		Author:    pr.Author,
		URL:       pr.URL,
		CreatedAt: pr.CreatedAt,
		UpdatedAt: pr.UpdatedAt,
		Branch:    pr.Branch,
		Body:      pr.Body,
		Reviews:   reviews,
		Comments:  comments,
		Commits:   commits,
	}
}

// convertCommits converts a slice of sources.Commit to a slice of CommitEntry.
func convertCommits(commits []sources.Commit) []CommitEntry {
	result := make([]CommitEntry, 0, len(commits))
	for _, c := range commits {
		result = append(result, CommitEntry{
			SHA:     c.SHA,
			Message: c.Message,
			Author:  c.Author,
			Date:    c.Date,
		})
	}
	return result
}
