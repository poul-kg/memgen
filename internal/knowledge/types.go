package knowledge

import "time"

// KnowledgeFile represents the complete YAML knowledge file for a ticket.
type KnowledgeFile struct {
	TicketID      string        `yaml:"ticket_id"`
	Branch        string        `yaml:"branch"`
	LastRefreshed time.Time     `yaml:"last_refreshed"`
	JIRA          JIRASection   `yaml:"jira"`
	PullRequests  []PullRequest `yaml:"pull_requests"`
	MainCommits   []CommitEntry `yaml:"main_commits"`
	Notes         []Note        `yaml:"notes"`
}

// JIRASection holds parsed JIRA ticket data.
type JIRASection struct {
	Summary     string        `yaml:"summary"`
	Description string        `yaml:"description"`
	Status      string        `yaml:"status"`
	Priority    string        `yaml:"priority"`
	Assignee    string        `yaml:"assignee"`
	Reporter    string        `yaml:"reporter"`
	Labels      []string      `yaml:"labels"`
	Comments    []JIRAComment `yaml:"comments"`
}

// JIRAComment holds a single JIRA comment in the knowledge file.
type JIRAComment struct {
	Author  string    `yaml:"author"`
	Created time.Time `yaml:"created"`
	Updated time.Time `yaml:"updated"`
	Body    string    `yaml:"body"`
}

// PullRequest holds pull request data in the knowledge file.
type PullRequest struct {
	Number    int           `yaml:"number"`
	Title     string        `yaml:"title"`
	State     string        `yaml:"state"`
	Author    string        `yaml:"author"`
	URL       string        `yaml:"url"`
	CreatedAt time.Time     `yaml:"created_at"`
	UpdatedAt time.Time     `yaml:"updated_at"`
	Branch    string        `yaml:"branch"`
	Body      string        `yaml:"body"`
	Reviews   []PRReview    `yaml:"reviews"`
	Comments  []PRComment   `yaml:"comments"`
	Commits   []CommitEntry `yaml:"commits"`
}

// PRReview holds a pull request review.
type PRReview struct {
	Author    string    `yaml:"author"`
	State     string    `yaml:"state"`
	Body      string    `yaml:"body"`
	CreatedAt time.Time `yaml:"created_at"`
}

// PRComment holds a pull request review comment.
type PRComment struct {
	ID        int       `yaml:"id"`
	Author    string    `yaml:"author"`
	Body      string    `yaml:"body"`
	Path      string    `yaml:"path"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
	InReplyTo int       `yaml:"in_reply_to"`
	Resolved  bool      `yaml:"resolved"`
}

// CommitEntry holds git commit data.
type CommitEntry struct {
	SHA     string    `yaml:"sha"`
	Message string    `yaml:"message"`
	Author  string    `yaml:"author"`
	Date    time.Time `yaml:"date"`
}

// Note holds an agent-provided note with a timestamp.
type Note struct {
	Date time.Time `yaml:"date"`
	Body string    `yaml:"body"`
}
