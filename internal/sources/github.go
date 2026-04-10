package sources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// CommandExecutor abstracts command execution for testing.
type CommandExecutor interface {
	// Execute runs a command and returns stdout, stderr, and error.
	Execute(name string, args ...string) (stdout string, stderr string, err error)
}

// DefaultExecutor runs real commands via os/exec.
type DefaultExecutor struct{}

// Execute runs a command and returns stdout, stderr, and any error.
func (e *DefaultExecutor) Execute(name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// GitHubClient fetches PR and commit data via the gh CLI.
type GitHubClient struct {
	Repo     string // "owner/repo" format
	Executor CommandExecutor
}

// PR holds pull request data.
type PR struct {
	Number    int
	Title     string
	Body      string
	State     string // OPEN, CLOSED, MERGED
	Author    string
	URL       string
	CreatedAt time.Time
	UpdatedAt time.Time
	Branch    string
	Commits   []Commit
	Reviews   []Review
	Comments  []PRComment
}

// Commit holds git commit data.
type Commit struct {
	SHA     string
	Message string
	Author  string
	Date    time.Time
}

// Review holds a pull request review.
type Review struct {
	Author    string
	State     string // APPROVED, CHANGES_REQUESTED, COMMENTED
	Body      string
	CreatedAt time.Time
}

// PRComment holds a pull request comment (both issue comments and review comments).
type PRComment struct {
	ID        int // GitHub database ID
	Author    string
	Body      string
	Path      string // file path if it's a review comment
	CreatedAt time.Time
	UpdatedAt time.Time
	InReplyTo int  // parent comment ID, 0 if top-level
	Resolved  bool // whether the review thread is resolved
}

// ghPRListEntry represents the JSON from `gh pr list --json ...`.
type ghPRListEntry struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	Author      ghAuthor  `json:"author"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	HeadRefName string    `json:"headRefName"`
	URL         string    `json:"url"`
}

type ghAuthor struct {
	Login string `json:"login"`
}

// ghReviewEntry represents a review from the GitHub API.
type ghReviewEntry struct {
	ID          int    `json:"id"`
	User        ghUser `json:"user"`
	Body        string `json:"body"`
	State       string `json:"state"`
	SubmittedAt string `json:"submitted_at"`
}

type ghUser struct {
	Login string `json:"login"`
}

// ghReviewCommentEntry represents a review comment from the GitHub API.
type ghReviewCommentEntry struct {
	ID          int    `json:"id"`
	User        ghUser `json:"user"`
	Body        string `json:"body"`
	Path        string `json:"path"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	InReplyToID *int   `json:"in_reply_to_id"`
}

// ghCommitEntry represents a commit from the GitHub API.
type ghCommitEntry struct {
	SHA    string       `json:"sha"`
	Commit ghCommitData `json:"commit"`
	Author *ghUser      `json:"author"`
}

type ghCommitData struct {
	Message string         `json:"message"`
	Author  ghCommitAuthor `json:"author"`
}

type ghCommitAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

// ValidateRepo checks that the configured repo exists and is accessible via gh CLI.
func (g *GitHubClient) ValidateRepo() error {
	if g.Repo == "" || !strings.Contains(g.Repo, "/") {
		return fmt.Errorf("GitHub repository not configured. Got %q — expected \"owner/repo\" format. Check the x-mcp-repo header in your .mcp.json configuration", g.Repo)
	}
	log.Printf("GitHub: validating repo %s", g.Repo)
	_, stderr, err := g.Executor.Execute("gh", "repo", "view", g.Repo, "--json", "name")
	if err != nil {
		if strings.Contains(stderr, "Could not resolve") || strings.Contains(stderr, "Not Found") || strings.Contains(stderr, "not found") {
			return fmt.Errorf("GitHub repository %q not found. Check the x-mcp-repo header in your .mcp.json configuration", g.Repo)
		}
		return fmt.Errorf("failed to validate GitHub repo %q: %s", g.Repo, stderr)
	}
	log.Printf("GitHub: repo %s OK", g.Repo)
	return nil
}

// ownerRepo splits the Repo field into owner and repo.
func (g *GitHubClient) ownerRepo() (string, string) {
	parts := strings.SplitN(g.Repo, "/", 2)
	if len(parts) != 2 {
		return g.Repo, ""
	}
	return parts[0], parts[1]
}

// FetchPRs finds all PRs (open, closed, merged) matching the ticket ID.
func (g *GitHubClient) FetchPRs(ticketID string) ([]PR, error) {
	log.Printf("GitHub: searching PRs for %s in %s", ticketID, g.Repo)
	stdout, stderr, err := g.Executor.Execute("gh", "pr", "list",
		"--repo", g.Repo,
		"--search", ticketID,
		"--state", "all",
		"--json", "number,title,body,state,author,createdAt,updatedAt,headRefName,url",
		"--limit", "100",
	)
	if err != nil {
		log.Printf("GitHub: PR search for %s FAILED: %v", ticketID, err)
		return nil, fmt.Errorf("gh pr list failed: %w (stderr: %s)", err, stderr)
	}

	var entries []ghPRListEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		return nil, fmt.Errorf("parsing gh pr list output: %w", err)
	}
	log.Printf("GitHub: found %d PRs for %s", len(entries), ticketID)

	// Post-filter: GitHub search is fuzzy, filter to exact ticket ID matches.
	entries = filterPREntriesByTicket(entries, ticketID)
	log.Printf("GitHub: %d PRs remain after exact-match filtering for %s", len(entries), ticketID)

	owner, repo := g.ownerRepo()
	prs := make([]PR, 0, len(entries))
	for _, e := range entries {
		pr := PR{
			Number:    e.Number,
			Title:     e.Title,
			Body:      e.Body,
			State:     e.State,
			Author:    e.Author.Login,
			URL:       e.URL,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
			Branch:    e.HeadRefName,
		}

		log.Printf("GitHub: fetching details for PR #%d (%s)", e.Number, e.Title)

		// Fetch reviews for each PR.
		reviews, err := g.fetchReviews(owner, repo, e.Number)
		if err != nil {
			return nil, fmt.Errorf("fetching reviews for PR #%d: %w", e.Number, err)
		}
		pr.Reviews = reviews

		// Fetch review comments for each PR.
		comments, err := g.fetchReviewComments(owner, repo, e.Number)
		if err != nil {
			return nil, fmt.Errorf("fetching comments for PR #%d: %w", e.Number, err)
		}
		pr.Comments = comments

		// Fetch commits for each PR.
		commits, err := g.fetchPRCommits(owner, repo, e.Number)
		if err != nil {
			return nil, fmt.Errorf("fetching commits for PR #%d: %w", e.Number, err)
		}
		pr.Commits = commits
		log.Printf("GitHub: PR #%d -> %d reviews, %d comments, %d commits", e.Number, len(reviews), len(comments), len(commits))

		prs = append(prs, pr)
	}

	return prs, nil
}

// FetchMainCommits finds commits on the default branch containing the ticket ID.
func (g *GitHubClient) FetchMainCommits(ticketID string) ([]Commit, error) {
	log.Printf("GitHub: fetching default branch commits for %s in %s", ticketID, g.Repo)

	// Try common default branch names: main, then master.
	var stdout, stderr string
	var err error
	for _, branch := range []string{"main", "master"} {
		stdout, stderr, err = g.Executor.Execute("gh", "api",
			fmt.Sprintf("repos/%s/commits?sha=%s&per_page=100", g.Repo, branch),
		)
		if err == nil {
			log.Printf("GitHub: using branch %q for %s", branch, g.Repo)
			break
		}
	}
	if err != nil {
		log.Printf("GitHub: fetch default branch commits FAILED: %v", err)
		return nil, fmt.Errorf("gh api commits failed: %w (stderr: %s)", err, stderr)
	}

	var entries []ghCommitEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		return nil, fmt.Errorf("parsing commits response: %w", err)
	}

	result := filterCommitsByTicket(entries, ticketID)
	log.Printf("GitHub: found %d/%d commits on main matching %s", len(result), len(entries), ticketID)
	return result, nil
}

// FetchPRsSince finds PRs updated after the given timestamp.
// Currently unused — refresh does a full re-fetch. Kept for potential future incremental refresh support.
func (g *GitHubClient) FetchPRsSince(ticketID string, since time.Time) ([]PR, error) {
	prs, err := g.FetchPRs(ticketID)
	if err != nil {
		return nil, err
	}

	var filtered []PR
	for _, pr := range prs {
		if pr.UpdatedAt.After(since) {
			filtered = append(filtered, pr)
		}
	}

	if filtered == nil {
		filtered = []PR{}
	}
	return filtered, nil
}

// FetchMainCommitsSince finds commits on the default branch after the given timestamp containing ticket ID.
// Currently unused — refresh does a full re-fetch. Kept for potential future incremental refresh support.
func (g *GitHubClient) FetchMainCommitsSince(ticketID string, since time.Time) ([]Commit, error) {
	log.Printf("GitHub: fetching default branch commits since %s for %s", since.Format(time.RFC3339), ticketID)

	var stdout, stderr string
	var err error
	for _, branch := range []string{"main", "master"} {
		stdout, stderr, err = g.Executor.Execute("gh", "api",
			fmt.Sprintf("repos/%s/commits?sha=%s&per_page=100&since=%s", g.Repo, branch, since.Format(time.RFC3339)),
		)
		if err == nil {
			break
		}
	}
	if err != nil {
		log.Printf("GitHub: fetch commits since FAILED: %v", err)
		return nil, fmt.Errorf("gh api commits failed: %w (stderr: %s)", err, stderr)
	}

	var entries []ghCommitEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		return nil, fmt.Errorf("parsing commits response: %w", err)
	}

	return filterCommitsByTicket(entries, ticketID), nil
}

// fetchReviews retrieves reviews for a specific PR.
func (g *GitHubClient) fetchReviews(owner, repo string, prNumber int) ([]Review, error) {
	stdout, stderr, err := g.Executor.Execute("gh", "api",
		fmt.Sprintf("repos/%s/%s/pulls/%d/reviews?per_page=100", owner, repo, prNumber),
	)
	if err != nil {
		return nil, fmt.Errorf("gh api reviews failed: %w (stderr: %s)", err, stderr)
	}

	var entries []ghReviewEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		return nil, fmt.Errorf("parsing reviews JSON: %w", err)
	}

	reviews := make([]Review, 0, len(entries))
	for _, e := range entries {
		createdAt, _ := time.Parse(time.RFC3339, e.SubmittedAt)
		reviews = append(reviews, Review{
			Author:    e.User.Login,
			State:     e.State,
			Body:      e.Body,
			CreatedAt: createdAt,
		})
	}

	return reviews, nil
}

// fetchReviewComments retrieves review comments for a specific PR.
func (g *GitHubClient) fetchReviewComments(owner, repo string, prNumber int) ([]PRComment, error) {
	stdout, stderr, err := g.Executor.Execute("gh", "api",
		fmt.Sprintf("repos/%s/%s/pulls/%d/comments?per_page=100", owner, repo, prNumber),
	)
	if err != nil {
		return nil, fmt.Errorf("gh api review comments failed: %w (stderr: %s)", err, stderr)
	}

	var entries []ghReviewCommentEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		return nil, fmt.Errorf("parsing review comments JSON: %w", err)
	}

	// Fetch resolved status via GraphQL. If it fails, default all to unresolved.
	resolvedMap, threadErr := g.fetchReviewThreads(owner, repo, prNumber)
	if threadErr != nil {
		log.Printf("GitHub: warning: failed to fetch review thread resolved status for PR #%d: %v (defaulting to unresolved)", prNumber, threadErr)
		resolvedMap = map[int]bool{}
	}

	comments := make([]PRComment, 0, len(entries))
	for _, e := range entries {
		createdAt, _ := time.Parse(time.RFC3339, e.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, e.UpdatedAt)
		inReplyTo := 0
		if e.InReplyToID != nil {
			inReplyTo = *e.InReplyToID
		}
		comments = append(comments, PRComment{
			ID:        e.ID,
			Author:    e.User.Login,
			Body:      e.Body,
			Path:      e.Path,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			InReplyTo: inReplyTo,
			Resolved:  resolvedMap[e.ID],
		})
	}

	return comments, nil
}

// ghGraphQLResponse represents the top-level GraphQL response structure.
type ghGraphQLResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []ghReviewThread `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// ghReviewThread represents a single review thread from the GraphQL API.
type ghReviewThread struct {
	IsResolved bool `json:"isResolved"`
	Comments   struct {
		Nodes []struct {
			DatabaseID int `json:"databaseId"`
		} `json:"nodes"`
	} `json:"comments"`
}

// fetchReviewThreads fetches the resolved status of review threads for a PR via GraphQL.
// Returns a map from the first comment's database ID in each thread to its resolved status.
func (g *GitHubClient) fetchReviewThreads(owner, repo string, prNumber int) (map[int]bool, error) {
	query := fmt.Sprintf(`query {
  repository(owner: "%s", name: "%s") {
    pullRequest(number: %d) {
      reviewThreads(first: 100) {
        nodes {
          isResolved
          comments(first: 1) {
            nodes { databaseId }
          }
        }
      }
    }
  }
}`, owner, repo, prNumber)

	stdout, stderr, err := g.Executor.Execute("gh", "api", "graphql", "-f", "query="+query)
	if err != nil {
		return nil, fmt.Errorf("gh api graphql failed: %w (stderr: %s)", err, stderr)
	}

	var resp ghGraphQLResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		return nil, fmt.Errorf("parsing GraphQL response: %w", err)
	}

	result := make(map[int]bool)
	for _, thread := range resp.Data.Repository.PullRequest.ReviewThreads.Nodes {
		if len(thread.Comments.Nodes) > 0 {
			commentID := thread.Comments.Nodes[0].DatabaseID
			result[commentID] = thread.IsResolved
		}
	}
	return result, nil
}

// fetchPRCommits retrieves commits for a specific PR.
func (g *GitHubClient) fetchPRCommits(owner, repo string, prNumber int) ([]Commit, error) {
	stdout, stderr, err := g.Executor.Execute("gh", "api",
		fmt.Sprintf("repos/%s/%s/pulls/%d/commits?per_page=100", owner, repo, prNumber),
	)
	if err != nil {
		return nil, fmt.Errorf("gh api PR commits failed: %w (stderr: %s)", err, stderr)
	}

	var entries []ghCommitEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		return nil, fmt.Errorf("parsing PR commits JSON: %w", err)
	}

	return convertCommits(entries), nil
}

// filterPREntriesByTicket keeps only PR entries where the ticket ID appears
// in the title, body, or branch name. GitHub's search is fuzzy and may return
// false positives (e.g., "404" matching in unrelated contexts).
func filterPREntriesByTicket(entries []ghPRListEntry, ticketID string) []ghPRListEntry {
	needle := strings.ToLower(ticketID)
	var filtered []ghPRListEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Title), needle) ||
			strings.Contains(strings.ToLower(e.Body), needle) ||
			strings.Contains(strings.ToLower(e.HeadRefName), needle) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// filterCommitsByTicket filters commits whose message contains the ticket ID.
func filterCommitsByTicket(entries []ghCommitEntry, ticketID string) []Commit {
	var result []Commit
	for _, e := range entries {
		if strings.Contains(e.Commit.Message, ticketID) {
			result = append(result, convertCommitEntry(e))
		}
	}
	if result == nil {
		result = []Commit{}
	}
	return result
}

// convertCommits converts API commit entries to our Commit type.
func convertCommits(entries []ghCommitEntry) []Commit {
	commits := make([]Commit, 0, len(entries))
	for _, e := range entries {
		commits = append(commits, convertCommitEntry(e))
	}
	return commits
}

// convertCommitEntry converts a single API commit entry to our Commit type.
func convertCommitEntry(e ghCommitEntry) Commit {
	date, _ := time.Parse(time.RFC3339, e.Commit.Author.Date)
	author := e.Commit.Author.Name
	if e.Author != nil && e.Author.Login != "" {
		author = e.Author.Login
	}
	return Commit{
		SHA:     e.SHA,
		Message: e.Commit.Message,
		Author:  author,
		Date:    date,
	}
}
