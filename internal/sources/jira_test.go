package sources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func loadTestdata(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/" + filename)
	if err != nil {
		t.Fatalf("failed to load testdata/%s: %v", filename, err)
	}
	return data
}

func TestFetchTicket_Success(t *testing.T) {
	ticketData := loadTestdata(t, "jira_ticket.json")
	commentsData := loadTestdata(t, "jira_comments.json")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header is present.
		auth := r.Header.Get("Authorization")
		if auth == "" {
			t.Error("expected Authorization header")
		}

		switch {
		case r.URL.Path == "/rest/api/3/issue/STITCH-1234" && r.URL.Query().Get("expand") == "renderedFields":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(ticketData)
		case r.URL.Path == "/rest/api/3/issue/STITCH-1234/comment":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(commentsData)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "test@example.com",
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	ticket, err := client.FetchTicket("STITCH-1234")
	if err != nil {
		t.Fatalf("FetchTicket returned error: %v", err)
	}

	if ticket.Key != "STITCH-1234" {
		t.Errorf("expected key STITCH-1234, got %s", ticket.Key)
	}
	if ticket.Summary != "Implement password reset flow via email" {
		t.Errorf("unexpected summary: %s", ticket.Summary)
	}
	if ticket.Status != "In Progress" {
		t.Errorf("expected status 'In Progress', got %s", ticket.Status)
	}
	if ticket.Priority != "High" {
		t.Errorf("expected priority 'High', got %s", ticket.Priority)
	}
	if ticket.Assignee != "Alice Chen" {
		t.Errorf("expected assignee 'Alice Chen', got %s", ticket.Assignee)
	}
	if ticket.Reporter != "Bob Martinez" {
		t.Errorf("expected reporter 'Bob Martinez', got %s", ticket.Reporter)
	}
	if len(ticket.Labels) != 3 {
		t.Errorf("expected 3 labels, got %d", len(ticket.Labels))
	}
	if ticket.Description == "" {
		t.Error("expected non-empty description")
	}
	// Verify rendered HTML description is used.
	if ticket.Description == ticket.Summary {
		t.Error("expected rendered HTML description, not summary fallback")
	}

	// Verify comments.
	if len(ticket.Comments) != 4 {
		t.Fatalf("expected 4 comments, got %d", len(ticket.Comments))
	}
	if ticket.Comments[0].Author != "Bob Martinez" {
		t.Errorf("expected first comment author 'Bob Martinez', got %s", ticket.Comments[0].Author)
	}
	if ticket.Comments[0].Body == "" {
		t.Error("expected non-empty comment body")
	}
	if ticket.Comments[0].Created.IsZero() {
		t.Error("expected non-zero comment created time")
	}
}

func TestFetchTicket_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorMessages":["Issue does not exist or you do not have permission to see it."]}`))
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "test@example.com",
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	_, err := client.FetchTicket("NONEXIST-999")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	expected := `could not find ticket "NONEXIST-999" in JIRA`
	if got := err.Error(); got[:len(expected)] != expected {
		t.Errorf("expected error to start with %q, got %q", expected, got)
	}
}

func TestFetchTicket_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Client must be authenticated to access this resource."}`))
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "bad@example.com",
		Token:      "bad-token",
		HTTPClient: server.Client(),
	}

	_, err := client.FetchTicket("STITCH-1234")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if got := err.Error(); got != "JIRA authentication failed (401): check email and API token" {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestFetchTicket_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{this is not valid json`))
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "test@example.com",
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	_, err := client.FetchTicket("STITCH-1234")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestFetchTicket_EmptyComments(t *testing.T) {
	ticketData := loadTestdata(t, "jira_ticket.json")
	emptyComments := `{"startAt":0,"maxResults":50,"total":0,"comments":[]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/issue/STITCH-1234" && r.URL.Query().Get("expand") == "renderedFields":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(ticketData)
		case r.URL.Path == "/rest/api/3/issue/STITCH-1234/comment":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(emptyComments))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "test@example.com",
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	ticket, err := client.FetchTicket("STITCH-1234")
	if err != nil {
		t.Fatalf("FetchTicket returned error: %v", err)
	}
	if len(ticket.Comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(ticket.Comments))
	}
}

func TestFetchCommentsSince_FiltersCorrectly(t *testing.T) {
	commentsData := loadTestdata(t, "jira_comments.json")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(commentsData)
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "test@example.com",
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	// Filter for comments after April 4, 2026 — should get the last 2 comments
	// (Carol's from April 5 and Alice's from April 5).
	since := time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)
	comments, err := client.FetchCommentsSince("STITCH-1234", since)
	if err != nil {
		t.Fatalf("FetchCommentsSince returned error: %v", err)
	}

	if len(comments) != 2 {
		t.Fatalf("expected 2 comments after April 4, got %d", len(comments))
	}
	if comments[0].Author != "Carol Nguyen" {
		t.Errorf("expected first filtered comment from Carol Nguyen, got %s", comments[0].Author)
	}
	if comments[1].Author != "Alice Chen" {
		t.Errorf("expected second filtered comment from Alice Chen, got %s", comments[1].Author)
	}
}

func TestFetchCommentsSince_NoMatchingComments(t *testing.T) {
	commentsData := loadTestdata(t, "jira_comments.json")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(commentsData)
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "test@example.com",
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	// Filter for comments after a far-future date — should get 0.
	since := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	comments, err := client.FetchCommentsSince("STITCH-1234", since)
	if err != nil {
		t.Fatalf("FetchCommentsSince returned error: %v", err)
	}

	if len(comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(comments))
	}
}

func TestFetchTicket_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errorMessages":["Internal server error"]}`))
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "test@example.com",
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	_, err := client.FetchTicket("STITCH-1234")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if got := err.Error(); !contains(got, "500") {
		t.Errorf("expected error to contain status code 500, got: %s", got)
	}
}

func TestFetchTicket_VerifiesAuthHeader(t *testing.T) {
	var capturedAuthHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/issue/TEST-1" {
			capturedAuthHeader = r.Header.Get("Authorization")
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "user@example.com",
		Token:      "mytoken123",
		HTTPClient: server.Client(),
	}

	_, _ = client.FetchTicket("TEST-1")

	if capturedAuthHeader == "" {
		t.Fatal("expected Authorization header to be set")
	}
	// Basic auth should be base64("user@example.com:mytoken123")
	expected := "Basic dXNlckBleGFtcGxlLmNvbTpteXRva2VuMTIz"
	if capturedAuthHeader != expected {
		t.Errorf("expected auth header %q, got %q", expected, capturedAuthHeader)
	}
}

func TestFetchTicket_NilAssignee(t *testing.T) {
	// Test with a ticket where assignee is null.
	ticketJSON := `{
		"key": "TEST-1",
		"renderedFields": {"description": "<p>Test</p>"},
		"fields": {
			"summary": "Unassigned ticket",
			"status": {"name": "To Do"},
			"priority": {"name": "Medium"},
			"assignee": null,
			"reporter": {"displayName": "Someone"},
			"labels": []
		}
	}`
	emptyComments := `{"startAt":0,"maxResults":50,"total":0,"comments":[]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/issue/TEST-1":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(ticketJSON))
		case r.URL.Path == "/rest/api/3/issue/TEST-1/comment":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(emptyComments))
		}
	}))
	defer server.Close()

	client := &JIRAClient{
		BaseURL:    server.URL,
		Email:      "test@example.com",
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	ticket, err := client.FetchTicket("TEST-1")
	if err != nil {
		t.Fatalf("FetchTicket returned error: %v", err)
	}
	if ticket.Assignee != "" {
		t.Errorf("expected empty assignee, got %s", ticket.Assignee)
	}
}

func TestJIRACommentParsing(t *testing.T) {
	// Verify that the test fixture parses correctly.
	data := loadTestdata(t, "jira_comments.json")
	var resp jiraCommentsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("failed to parse jira_comments.json: %v", err)
	}
	if resp.Total != 4 {
		t.Errorf("expected 4 total comments, got %d", resp.Total)
	}
	if len(resp.Comments) != 4 {
		t.Errorf("expected 4 comments, got %d", len(resp.Comments))
	}
	for i, c := range resp.Comments {
		if c.RenderedBody == "" {
			t.Errorf("comment %d has empty renderedBody", i)
		}
		if c.Author == nil {
			t.Errorf("comment %d has nil author", i)
		}
	}
}

// contains is a test helper that checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
