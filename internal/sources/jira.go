package sources

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// JIRAClient fetches ticket data from JIRA Cloud.
type JIRAClient struct {
	BaseURL    string // e.g. "https://stitchai.atlassian.net"
	Email      string
	Token      string
	HTTPClient *http.Client
}

// JIRATicket holds parsed JIRA ticket data.
type JIRATicket struct {
	Key         string
	Summary     string
	Description string // plain text description (HTML stripped)
	Status      string
	Assignee    string
	Reporter    string
	Priority    string
	Labels      []string
	Comments    []JIRAComment
}

// JIRAComment holds a single JIRA comment.
type JIRAComment struct {
	Author  string
	Body    string
	Created time.Time
	Updated time.Time
}

// jiraIssueResponse represents the JSON structure of a JIRA issue API response.
type jiraIssueResponse struct {
	Key            string             `json:"key"`
	RenderedFields jiraRenderedFields `json:"renderedFields"`
	Fields         jiraFields         `json:"fields"`
}

type jiraRenderedFields struct {
	Description string `json:"description"`
}

type jiraFields struct {
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description"` // ADF format, we prefer renderedFields
	Status      jiraStatus      `json:"status"`
	Priority    jiraPriority    `json:"priority"`
	Assignee    *jiraUser       `json:"assignee"`
	Reporter    *jiraUser       `json:"reporter"`
	Labels      []string        `json:"labels"`
}

type jiraStatus struct {
	Name string `json:"name"`
}

type jiraPriority struct {
	Name string `json:"name"`
}

type jiraUser struct {
	DisplayName string `json:"displayName"`
}

// jiraCommentsResponse represents the JSON structure of the JIRA comments API response.
type jiraCommentsResponse struct {
	Comments []jiraCommentEntry `json:"comments"`
	Total    int                `json:"total"`
}

type jiraCommentEntry struct {
	Author       *jiraUser       `json:"author"`
	Body         json.RawMessage `json:"body"` // ADF object in v3; we prefer renderedBody
	RenderedBody string          `json:"renderedBody"`
	Created      string          `json:"created"`
	Updated      string          `json:"updated"`
}

// FetchTicket fetches a JIRA ticket by key including all comments.
// Returns a descriptive error if ticket not found (404) with the browse URL.
func (c *JIRAClient) FetchTicket(ticketID string) (*JIRATicket, error) {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	// Fetch the issue with rendered fields.
	issueURL := fmt.Sprintf("%s/rest/api/3/issue/%s?expand=renderedFields", c.BaseURL, ticketID)
	log.Printf("JIRA: fetching ticket %s -> %s", ticketID, issueURL)
	issueReq, err := http.NewRequest("GET", issueURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating issue request: %w", err)
	}
	c.setAuth(issueReq)

	issueResp, err := httpClient.Do(issueReq)
	if err != nil {
		log.Printf("JIRA: fetch ticket %s FAILED: %v", ticketID, err)
		return nil, fmt.Errorf("fetching JIRA issue: %w", err)
	}
	defer func() { _ = issueResp.Body.Close() }()
	log.Printf("JIRA: fetch ticket %s -> %d", ticketID, issueResp.StatusCode)

	issueBody, err := io.ReadAll(issueResp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading issue response body: %w", err)
	}

	if issueResp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("could not find ticket %q in JIRA (browse: %s/browse/%s)", ticketID, c.BaseURL, ticketID)
	}
	if issueResp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("JIRA authentication failed (401): check email and API token")
	}
	if issueResp.StatusCode != http.StatusOK {
		snippet := string(issueBody)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("JIRA API returned status %d: %s", issueResp.StatusCode, snippet)
	}

	var issue jiraIssueResponse
	if err := json.Unmarshal(issueBody, &issue); err != nil {
		return nil, fmt.Errorf("parsing JIRA issue JSON: %w", err)
	}

	ticket := &JIRATicket{
		Key:     issue.Key,
		Summary: issue.Fields.Summary,
		Status:  issue.Fields.Status.Name,
		Labels:  issue.Fields.Labels,
	}

	// Use rendered description if available (strip HTML), fall back to summary.
	if issue.RenderedFields.Description != "" {
		ticket.Description = stripHTML(issue.RenderedFields.Description)
	} else {
		ticket.Description = issue.Fields.Summary
	}

	if issue.Fields.Priority.Name != "" {
		ticket.Priority = issue.Fields.Priority.Name
	}
	if issue.Fields.Assignee != nil {
		ticket.Assignee = issue.Fields.Assignee.DisplayName
	}
	if issue.Fields.Reporter != nil {
		ticket.Reporter = issue.Fields.Reporter.DisplayName
	}
	if ticket.Labels == nil {
		ticket.Labels = []string{}
	}

	// Fetch comments.
	comments, err := c.fetchAllComments(httpClient, ticketID)
	if err != nil {
		return nil, fmt.Errorf("fetching comments for %s: %w", ticketID, err)
	}
	ticket.Comments = comments

	return ticket, nil
}

// FetchCommentsSince fetches only comments created or updated after the given timestamp.
// Currently unused — refresh does a full re-fetch. Kept for potential future incremental refresh support.
func (c *JIRAClient) FetchCommentsSince(ticketID string, since time.Time) ([]JIRAComment, error) {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	allComments, err := c.fetchAllComments(httpClient, ticketID)
	if err != nil {
		return nil, err
	}

	var filtered []JIRAComment
	for _, comment := range allComments {
		if comment.Created.After(since) || comment.Updated.After(since) {
			filtered = append(filtered, comment)
		}
	}

	if filtered == nil {
		filtered = []JIRAComment{}
	}
	return filtered, nil
}

// fetchAllComments retrieves all comments for a ticket.
func (c *JIRAClient) fetchAllComments(httpClient *http.Client, ticketID string) ([]JIRAComment, error) {
	commentsURL := fmt.Sprintf("%s/rest/api/3/issue/%s/comment?orderBy=created", c.BaseURL, ticketID)
	log.Printf("JIRA: fetching comments for %s -> %s", ticketID, commentsURL)
	commentsReq, err := http.NewRequest("GET", commentsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating comments request: %w", err)
	}
	c.setAuth(commentsReq)

	commentsResp, err := httpClient.Do(commentsReq)
	if err != nil {
		log.Printf("JIRA: fetch comments %s FAILED: %v", ticketID, err)
		return nil, fmt.Errorf("fetching JIRA comments: %w", err)
	}
	defer func() { _ = commentsResp.Body.Close() }()
	log.Printf("JIRA: fetch comments %s -> %d", ticketID, commentsResp.StatusCode)

	commentsBody, err := io.ReadAll(commentsResp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading comments response body: %w", err)
	}

	if commentsResp.StatusCode != http.StatusOK {
		snippet := string(commentsBody)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("JIRA comments API returned status %d: %s", commentsResp.StatusCode, snippet)
	}

	var commentsData jiraCommentsResponse
	if err := json.Unmarshal(commentsBody, &commentsData); err != nil {
		return nil, fmt.Errorf("parsing JIRA comments JSON: %w", err)
	}

	comments := make([]JIRAComment, 0, len(commentsData.Comments))
	for _, c := range commentsData.Comments {
		created, _ := time.Parse("2006-01-02T15:04:05.000-0700", c.Created)
		updated, _ := time.Parse("2006-01-02T15:04:05.000-0700", c.Updated)

		author := ""
		if c.Author != nil {
			author = c.Author.DisplayName
		}

		body := stripHTML(c.RenderedBody)
		if body == "" {
			body = extractADFText(c.Body)
		}

		comments = append(comments, JIRAComment{
			Author:  author,
			Body:    body,
			Created: created,
			Updated: updated,
		})
	}

	return comments, nil
}

// extractADFText recursively extracts plain text from JIRA's Atlassian Document Format (ADF) JSON.
func extractADFText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return string(raw)
	}

	var result strings.Builder

	// If this node has "text", extract it.
	if textRaw, ok := node["text"]; ok {
		var text string
		if err := json.Unmarshal(textRaw, &text); err == nil {
			result.WriteString(text)
		}
	}

	// Recurse into "content" array.
	if contentRaw, ok := node["content"]; ok {
		var children []json.RawMessage
		if err := json.Unmarshal(contentRaw, &children); err == nil {
			for _, child := range children {
				childText := extractADFText(child)
				if childText != "" {
					result.WriteString(childText)
					result.WriteString("\n")
				}
			}
		}
	}

	return strings.TrimSpace(result.String())
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)
var multiNewlineRe = regexp.MustCompile(`\n{3,}`)

// stripHTML removes HTML tags and collapses whitespace for cleaner text output.
func stripHTML(s string) string {
	text := htmlTagRe.ReplaceAllString(s, "")
	text = multiNewlineRe.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// setAuth sets Basic authentication headers on the request.
func (c *JIRAClient) setAuth(req *http.Request) {
	auth := base64.StdEncoding.EncodeToString([]byte(c.Email + ":" + c.Token))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
}
