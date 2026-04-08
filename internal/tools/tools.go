package tools

import (
	"fmt"
	"strings"
	"time"

	"github.com/poul-kg/memgen/internal/claude"
	"github.com/poul-kg/memgen/internal/knowledge"
	"github.com/poul-kg/memgen/internal/sources"
	"github.com/poul-kg/memgen/internal/ticket"
)

// Deps holds shared dependencies for all tools.
type Deps struct {
	Store       *knowledge.Store
	Locks       *knowledge.LockManager
	JIRA        *sources.JIRAClient
	GitHub      *sources.GitHubClient
	Claude      *claude.CLI
	JIRABaseURL string // for browse URLs in error messages
}

// lockKey returns the lock key for a repo+ticket combination.
func lockKey(repo, ticketID string) string {
	return repo + "/" + ticketID
}

// extractTicket extracts ticket ID from branch, returns formatted error if not found.
func extractTicket(branch string) (string, error) {
	return ticket.Extract(branch)
}

// formatRawData converts all source data into a single text block for Claude.
func formatRawData(jiraTicket *sources.JIRATicket, prs []sources.PR, mainCommits []sources.Commit) string {
	var b strings.Builder

	// === JIRA TICKET ===
	b.WriteString("=== JIRA TICKET ===\n")
	if jiraTicket != nil {
		_, _ = fmt.Fprintf(&b, "Key: %s\n", jiraTicket.Key)
		_, _ = fmt.Fprintf(&b, "Summary: %s\n", jiraTicket.Summary)
		_, _ = fmt.Fprintf(&b, "Description: %s\n", jiraTicket.Description)
		_, _ = fmt.Fprintf(&b, "Status: %s\n", jiraTicket.Status)
		_, _ = fmt.Fprintf(&b, "Priority: %s\n", jiraTicket.Priority)
		_, _ = fmt.Fprintf(&b, "Assignee: %s\n", jiraTicket.Assignee)
		_, _ = fmt.Fprintf(&b, "Reporter: %s\n", jiraTicket.Reporter)
		if len(jiraTicket.Labels) > 0 {
			_, _ = fmt.Fprintf(&b, "Labels: %s\n", strings.Join(jiraTicket.Labels, ", "))
		}

		// === JIRA COMMENTS ===
		b.WriteString("\n=== JIRA COMMENTS ===\n")
		for _, c := range jiraTicket.Comments {
			_, _ = fmt.Fprintf(&b, "[%s] %s: %s\n", c.Created.UTC().Format(time.RFC3339), c.Author, c.Body)
		}
	}

	// === PULL REQUESTS ===
	b.WriteString("\n=== PULL REQUESTS ===\n")
	for _, pr := range prs {
		_, _ = fmt.Fprintf(&b, "PR #%d: %s (%s)\n", pr.Number, pr.Title, pr.State)
		_, _ = fmt.Fprintf(&b, "Author: %s\n", pr.Author)
		_, _ = fmt.Fprintf(&b, "URL: %s\n", pr.URL)
		_, _ = fmt.Fprintf(&b, "Body: %s\n", pr.Body)

		// -- Reviews --
		if len(pr.Reviews) > 0 {
			b.WriteString("-- Reviews --\n")
			for _, r := range pr.Reviews {
				_, _ = fmt.Fprintf(&b, "[%s] %s (%s): %s\n",
					r.CreatedAt.UTC().Format("2006-01-02"),
					r.Author, r.State, r.Body)
			}
		}

		// -- Review Comments --
		if len(pr.Comments) > 0 {
			b.WriteString("-- Review Comments --\n")
			for _, c := range pr.Comments {
				if c.InReplyTo == 0 {
					_, _ = fmt.Fprintf(&b, "[%s] %s on %s: %s\n",
						c.CreatedAt.UTC().Format("2006-01-02"),
						c.Author, c.Path, c.Body)
				} else {
					_, _ = fmt.Fprintf(&b, "  Reply by %s: %s\n", c.Author, c.Body)
				}
			}
		}

		// -- Commits --
		if len(pr.Commits) > 0 {
			b.WriteString("-- Commits --\n")
			for _, c := range pr.Commits {
				sha := c.SHA
				if len(sha) > 7 {
					sha = sha[:7]
				}
				_, _ = fmt.Fprintf(&b, "%s - %s (%s, %s)\n",
					sha, c.Message, c.Author,
					c.Date.UTC().Format("2006-01-02"))
			}
		}
		b.WriteString("\n")
	}

	// === COMMITS ON MAIN ===
	b.WriteString("=== COMMITS ON MAIN ===\n")
	for _, c := range mainCommits {
		sha := c.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		_, _ = fmt.Fprintf(&b, "%s - %s (%s, %s)\n",
			sha, c.Message, c.Author,
			c.Date.UTC().Format("2006-01-02"))
	}

	return b.String()
}
