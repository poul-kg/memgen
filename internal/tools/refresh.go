package tools

import (
	"fmt"
	"strings"
	"time"

	"github.com/poul-kg/memgen/internal/sources"
)

// Refresh fetches new data since the last refresh and updates the knowledge file.
func Refresh(deps *Deps, repo, branch string) (string, error) {
	// 1. Extract ticket ID.
	ticketID, err := extractTicket(branch)
	if err != nil {
		return "", err
	}

	// 2. If file doesn't exist, return error telling to run init.
	if !deps.Store.Exists(repo, ticketID) {
		return "", fmt.Errorf("No knowledge file found for %s. Run memgen__init first.", ticketID)
	}

	// 3. Validate sources are accessible before doing any work.
	if err := deps.GitHub.ValidateRepo(); err != nil {
		return "", err
	}

	// 4. Try lock.
	key := lockKey(repo, ticketID)
	if !deps.Locks.TryLock(key) {
		return "", fmt.Errorf("An operation is already in progress for %s. Try again later.", ticketID)
	}
	defer deps.Locks.Unlock(key)

	// 5. Read existing knowledge, extract LastRefreshed timestamp.
	existing, err := deps.Store.Read(repo, ticketID)
	if err != nil {
		return "", fmt.Errorf("failed to read knowledge file for %s: %w", ticketID, err)
	}

	since := deps.Store.LastRefreshed(existing)
	if since.IsZero() {
		// If no timestamp found, use a reasonable default (fetch everything).
		since = time.Time{}
	}

	// 6. Fetch new data since that timestamp from all sources.
	newComments, err := deps.JIRA.FetchCommentsSince(ticketID, since)
	if err != nil {
		return "", fmt.Errorf("failed to fetch new JIRA comments for %s: %w", ticketID, err)
	}

	newPRs, err := deps.GitHub.FetchPRsSince(ticketID, since)
	if err != nil {
		return "", fmt.Errorf("failed to fetch new GitHub PRs for %s: %w", ticketID, err)
	}

	newCommits, err := deps.GitHub.FetchMainCommitsSince(ticketID, since)
	if err != nil {
		return "", fmt.Errorf("failed to fetch new main commits for %s: %w", ticketID, err)
	}

	// 7. If no new data, update the Last Refreshed line and return "Knowledge is up to date."
	if len(newComments) == 0 && len(newPRs) == 0 && len(newCommits) == 0 {
		updated := updateLastRefreshed(existing)
		if err := deps.Store.Write(repo, ticketID, updated); err != nil {
			return "", fmt.Errorf("failed to update knowledge file timestamp: %w", err)
		}
		return "Knowledge is up to date.", nil
	}

	// 8. Format new raw data.
	rawData := formatRefreshRawData(newComments, newPRs, newCommits)

	// 9. Call Claude RefreshKnowledge.
	currentTime := time.Now().UTC().Format(time.RFC3339)
	result, err := deps.Claude.RefreshKnowledge(existing, rawData, currentTime)
	if err != nil {
		return "", fmt.Errorf("failed to refresh knowledge via Claude: %w", err)
	}

	// 10. Write result.
	if err := deps.Store.Write(repo, ticketID, result); err != nil {
		return "", fmt.Errorf("failed to write refreshed knowledge file: %w", err)
	}

	// 11. Return summary of what was added.
	return fmt.Sprintf("Refreshed knowledge for %s: %d new comments + %d updated PRs + %d new commits on main",
		ticketID, len(newComments), len(newPRs), len(newCommits)), nil
}

// updateLastRefreshed replaces or inserts the Last Refreshed timestamp in knowledge content.
func updateLastRefreshed(content string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	const prefix = "**Last Refreshed**: "

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = prefix + now
			return strings.Join(lines, "\n")
		}
	}

	// If no existing timestamp, add it after the first line.
	if len(lines) > 0 {
		result := []string{lines[0], prefix + now}
		result = append(result, lines[1:]...)
		return strings.Join(result, "\n")
	}

	return prefix + now + "\n" + content
}

// formatRefreshRawData formats only the new data fetched since last refresh into a text block.
func formatRefreshRawData(comments []sources.JIRAComment, prs []sources.PR, commits []sources.Commit) string {
	var b strings.Builder

	if len(comments) > 0 {
		b.WriteString("=== NEW JIRA COMMENTS ===\n")
		for _, c := range comments {
			_, _ = fmt.Fprintf(&b, "[%s] %s: %s\n", c.Created.UTC().Format(time.RFC3339), c.Author, c.Body)
		}
		b.WriteString("\n")
	}

	if len(prs) > 0 {
		b.WriteString("=== UPDATED PULL REQUESTS ===\n")
		for _, pr := range prs {
			_, _ = fmt.Fprintf(&b, "PR #%d: %s (%s)\n", pr.Number, pr.Title, pr.State)
			_, _ = fmt.Fprintf(&b, "Author: %s\n", pr.Author)
			_, _ = fmt.Fprintf(&b, "URL: %s\n", pr.URL)
			_, _ = fmt.Fprintf(&b, "Body: %s\n", pr.Body)

			if len(pr.Reviews) > 0 {
				b.WriteString("-- Reviews --\n")
				for _, r := range pr.Reviews {
					_, _ = fmt.Fprintf(&b, "[%s] %s (%s): %s\n",
						r.CreatedAt.UTC().Format("2006-01-02"),
						r.Author, r.State, r.Body)
				}
			}

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
	}

	if len(commits) > 0 {
		b.WriteString("=== NEW COMMITS ON MAIN ===\n")
		for _, c := range commits {
			sha := c.SHA
			if len(sha) > 7 {
				sha = sha[:7]
			}
			_, _ = fmt.Fprintf(&b, "%s - %s (%s, %s)\n",
				sha, c.Message, c.Author,
				c.Date.UTC().Format("2006-01-02"))
		}
	}

	return b.String()
}
