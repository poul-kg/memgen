package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/poul-kg/memgen/internal/knowledge"
	"github.com/poul-kg/memgen/internal/sources"
)

// --- Mock GitHub CommandExecutor ---

// MockGHExecutor records calls and returns canned responses for the gh CLI.
type MockGHExecutor struct {
	Calls     []string
	Responses map[string]MockGHResponse
	Default   MockGHResponse
}

// MockGHResponse holds canned stdout, stderr, and error for a mock gh command.
type MockGHResponse struct {
	Stdout string
	Stderr string
	Err    error
}

func (m *MockGHExecutor) Execute(name string, args ...string) (string, string, error) {
	call := name + " " + strings.Join(args, " ")
	m.Calls = append(m.Calls, call)

	bestKey := ""
	for key := range m.Responses {
		if strings.HasPrefix(call, key) && len(key) > len(bestKey) {
			bestKey = key
		}
	}
	if bestKey != "" {
		r := m.Responses[bestKey]
		return r.Stdout, r.Stderr, r.Err
	}
	return m.Default.Stdout, m.Default.Stderr, m.Default.Err
}

// --- JIRA test server helpers ---

// newJIRATestServer creates an httptest server that serves a canned JIRA ticket and comments.
func newJIRATestServer(t *testing.T, ticketID string, ticket *jiraTestTicket, comments []jiraTestComment) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == fmt.Sprintf("/rest/api/3/issue/%s", ticketID):
			if ticket == nil {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			data, _ := json.Marshal(ticket.toResponse())
			_, _ = w.Write(data)

		case r.URL.Path == fmt.Sprintf("/rest/api/3/issue/%s/comment", ticketID):
			w.Header().Set("Content-Type", "application/json")
			resp := jiraCommentsResp{Comments: make([]jiraCommentResp, 0)}
			for _, c := range comments {
				resp.Comments = append(resp.Comments, c.toResponse())
			}
			resp.Total = len(resp.Comments)
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// newJIRAFailServer creates a JIRA server that always returns the given status code.
func newJIRAFailServer(statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(`{"errorMessages":["error"]}`))
	}))
}

// jiraTestTicket is a helper for building JIRA test fixtures.
type jiraTestTicket struct {
	Key         string
	Summary     string
	Description string
	Status      string
	Priority    string
	Assignee    string
	Reporter    string
	Labels      []string
}

type jiraTestComment struct {
	Author  string
	Body    string
	Created string // RFC3339-ish format for JIRA: "2006-01-02T15:04:05.000+0000"
	Updated string
}

type jiraIssueResp struct {
	Key            string           `json:"key"`
	RenderedFields jiraRenderedResp `json:"renderedFields"`
	Fields         jiraFieldsResp   `json:"fields"`
}

type jiraRenderedResp struct {
	Description string `json:"description"`
}

type jiraFieldsResp struct {
	Summary  string         `json:"summary"`
	Status   jiraStatusResp `json:"status"`
	Priority jiraPriorResp  `json:"priority"`
	Assignee *jiraUserResp  `json:"assignee"`
	Reporter *jiraUserResp  `json:"reporter"`
	Labels   []string       `json:"labels"`
}

type jiraStatusResp struct {
	Name string `json:"name"`
}

type jiraPriorResp struct {
	Name string `json:"name"`
}

type jiraUserResp struct {
	DisplayName string `json:"displayName"`
}

type jiraCommentsResp struct {
	Comments []jiraCommentResp `json:"comments"`
	Total    int               `json:"total"`
}

type jiraCommentResp struct {
	Author       *jiraUserResp   `json:"author"`
	RenderedBody string          `json:"renderedBody"`
	Body         json.RawMessage `json:"body"`
	Created      string          `json:"created"`
	Updated      string          `json:"updated"`
}

func (t *jiraTestTicket) toResponse() jiraIssueResp {
	resp := jiraIssueResp{
		Key:            t.Key,
		RenderedFields: jiraRenderedResp{Description: t.Description},
		Fields: jiraFieldsResp{
			Summary:  t.Summary,
			Status:   jiraStatusResp{Name: t.Status},
			Priority: jiraPriorResp{Name: t.Priority},
			Labels:   t.Labels,
		},
	}
	if t.Assignee != "" {
		resp.Fields.Assignee = &jiraUserResp{DisplayName: t.Assignee}
	}
	if t.Reporter != "" {
		resp.Fields.Reporter = &jiraUserResp{DisplayName: t.Reporter}
	}
	if resp.Fields.Labels == nil {
		resp.Fields.Labels = []string{}
	}
	return resp
}

func (c *jiraTestComment) toResponse() jiraCommentResp {
	return jiraCommentResp{
		Author:       &jiraUserResp{DisplayName: c.Author},
		RenderedBody: c.Body,
		Body:         json.RawMessage(`"` + c.Body + `"`),
		Created:      c.Created,
		Updated:      c.Updated,
	}
}

// --- Test deps builder ---

// testDeps creates a Deps with real Store (temp dir), real LockManager, and injected mocks.
func testDeps(t *testing.T, jiraServer *httptest.Server, ghExec *MockGHExecutor) *Deps {
	t.Helper()
	tmpDir := t.TempDir()

	var jiraClient *sources.JIRAClient
	if jiraServer != nil {
		jiraClient = &sources.JIRAClient{
			BaseURL:    jiraServer.URL,
			Email:      "test@example.com",
			Token:      "test-token",
			HTTPClient: jiraServer.Client(),
		}
	}

	baseURL := ""
	if jiraServer != nil {
		baseURL = jiraServer.URL
	}

	return &Deps{
		Store:          knowledge.NewStore(tmpDir),
		Locks:          knowledge.NewLockManager(),
		JIRA:           jiraClient,
		GitHubExecutor: ghExec,
		JIRABaseURL:    baseURL,
	}
}

// ghPRListJSON generates a minimal gh pr list JSON response.
func ghPRListJSON(prs []ghPREntry) string {
	data, _ := json.Marshal(prs)
	return string(data)
}

type ghPREntry struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	State       string `json:"state"`
	Author      ghAuth `json:"author"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	HeadRefName string `json:"headRefName"`
	URL         string `json:"url"`
}

type ghAuth struct {
	Login string `json:"login"`
}

// ghCommitsJSON generates a minimal GitHub commits API JSON response.
func ghCommitsJSON(commits []ghCommitJSON) string {
	data, _ := json.Marshal(commits)
	return string(data)
}

type ghCommitJSON struct {
	SHA    string       `json:"sha"`
	Commit ghCommitData `json:"commit"`
	Author *ghAuthJSON  `json:"author"`
}

type ghCommitData struct {
	Message string       `json:"message"`
	Author  ghAuthorData `json:"author"`
}

type ghAuthorData struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

type ghAuthJSON struct {
	Login string `json:"login"`
}
