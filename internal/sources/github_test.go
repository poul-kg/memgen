package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// MockExecutor records calls and returns canned responses.
type MockExecutor struct {
	// Calls records each invocation as "name arg1 arg2 ...".
	Calls []string
	// Responses maps a command prefix to its stdout, stderr, and error.
	// The key is matched against the beginning of the recorded call string.
	Responses map[string]MockResponse
	// DefaultResponse is returned if no matching response is found.
	DefaultResponse MockResponse
}

// MockResponse holds canned stdout, stderr, and error for a mock command.
type MockResponse struct {
	Stdout string
	Stderr string
	Err    error
}

// Execute records the call and returns the matching canned response.
func (m *MockExecutor) Execute(name string, args ...string) (string, string, error) {
	call := name + " " + strings.Join(args, " ")
	m.Calls = append(m.Calls, call)

	// Find the longest matching prefix.
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
	return m.DefaultResponse.Stdout, m.DefaultResponse.Stderr, m.DefaultResponse.Err
}

func TestFilterPREntriesByTicket(t *testing.T) {
	t.Parallel()

	entries := []ghPRListEntry{
		{Number: 1, Title: "SBUX-404: Fix login flow", Body: "Some body", HeadRefName: "feature/login"},
		{Number: 2, Title: "Fix 404 page not found error", Body: "Unrelated PR about HTTP 404", HeadRefName: "fix/404-page"},
		{Number: 3, Title: "Update configs", Body: "This relates to sbux-404 work", HeadRefName: "chore/configs"},
		{Number: 4, Title: "Branch match", Body: "No mention in title or body", HeadRefName: "feature/SBUX-404-checkout"},
		{Number: 5, Title: "Completely unrelated", Body: "Nothing relevant here", HeadRefName: "feature/unrelated"},
	}

	filtered := filterPREntriesByTicket(entries, "SBUX-404")

	if len(filtered) != 3 {
		t.Fatalf("expected 3 entries after filter, got %d", len(filtered))
	}

	// Verify which PRs survived.
	numbers := make([]int, len(filtered))
	for i, e := range filtered {
		numbers[i] = e.Number
	}

	expected := []int{1, 3, 4}
	for i, want := range expected {
		if numbers[i] != want {
			t.Errorf("filtered[%d]: expected PR #%d, got #%d", i, want, numbers[i])
		}
	}
}

func TestFetchPRs_FiltersFalsePositives(t *testing.T) {
	// gh pr list returns 3 PRs: one real match, one with just "404", one unrelated.
	prListData := `[
		{
			"number": 10,
			"title": "SBUX-404: Implement checkout flow",
			"body": "Implements SBUX-404.",
			"state": "OPEN",
			"author": {"login": "dev1"},
			"createdAt": "2026-04-01T10:00:00Z",
			"updatedAt": "2026-04-02T10:00:00Z",
			"headRefName": "feature/SBUX-404-checkout",
			"url": "https://github.com/myorg/myrepo/pull/10"
		},
		{
			"number": 20,
			"title": "Fix 404 page rendering",
			"body": "The 404 page was broken.",
			"state": "MERGED",
			"author": {"login": "dev2"},
			"createdAt": "2026-03-15T10:00:00Z",
			"updatedAt": "2026-03-16T10:00:00Z",
			"headRefName": "fix/404-page",
			"url": "https://github.com/myorg/myrepo/pull/20"
		},
		{
			"number": 30,
			"title": "Add logging middleware",
			"body": "General logging improvements.",
			"state": "OPEN",
			"author": {"login": "dev3"},
			"createdAt": "2026-04-05T10:00:00Z",
			"updatedAt": "2026-04-06T10:00:00Z",
			"headRefName": "feature/logging",
			"url": "https://github.com/myorg/myrepo/pull/30"
		}
	]`

	emptyGraphql := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}`

	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh pr list":                                      {Stdout: prListData},
			"gh api repos/myorg/myrepo/pulls/10/reviews":      {Stdout: "[]"},
			"gh api repos/myorg/myrepo/pulls/10/comments":     {Stdout: "[]"},
			"gh api repos/myorg/myrepo/pulls/10/commits":      {Stdout: "[]"},
			"gh api graphql -f query=query {\n  repository(owner: \"myorg\", name: \"myrepo\") {\n    pullRequest(number: 10)": {Stdout: emptyGraphql},
		},
	}

	client := &GitHubClient{
		Repo:     "myorg/myrepo",
		Executor: mock,
	}

	prs, err := client.FetchPRs("SBUX-404")
	if err != nil {
		t.Fatalf("FetchPRs returned error: %v", err)
	}

	// Only PR #10 should survive (SBUX-404 in title/body/branch).
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR after filtering, got %d", len(prs))
	}
	if prs[0].Number != 10 {
		t.Errorf("expected PR #10, got #%d", prs[0].Number)
	}

	// Verify no API calls were made for filtered-out PRs #20 and #30.
	for _, call := range mock.Calls {
		if strings.Contains(call, "pulls/20") || strings.Contains(call, "pulls/30") {
			t.Errorf("unexpected API call for filtered-out PR: %s", call)
		}
	}
}

func TestFetchPRs_Success(t *testing.T) {
	prListData := string(loadTestdata(t, "gh_pr_list.json"))
	reviewsData := string(loadTestdata(t, "gh_pr_reviews.json"))
	commentsData := string(loadTestdata(t, "gh_pr_comments.json"))
	// Use a simple commits response for PR commits.
	prCommitsData := `[
		{
			"sha": "abc123",
			"commit": {
				"message": "STITCH-1234: initial implementation",
				"author": {"name": "Alice Chen", "email": "alice@stitchai.com", "date": "2026-04-03T15:00:00Z"}
			},
			"author": {"login": "alicec"}
		},
		{
			"sha": "def456",
			"commit": {
				"message": "STITCH-1234: address review feedback",
				"author": {"name": "Alice Chen", "email": "alice@stitchai.com", "date": "2026-04-04T16:00:00Z"}
			},
			"author": {"login": "alicec"}
		}
	]`

	// GraphQL response for review thread resolved status.
	graphqlResponse142 := `{
		"data": {
			"repository": {
				"pullRequest": {
					"reviewThreads": {
						"nodes": [
							{
								"isResolved": true,
								"comments": { "nodes": [{ "databaseId": 90001 }] }
							},
							{
								"isResolved": false,
								"comments": { "nodes": [{ "databaseId": 90003 }] }
							},
							{
								"isResolved": true,
								"comments": { "nodes": [{ "databaseId": 90004 }] }
							}
						]
					}
				}
			}
		}
	}`
	emptyGraphql := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}`

	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh pr list": {Stdout: prListData},
			"gh api repos/stitchai/platform/pulls/142/reviews":  {Stdout: reviewsData},
			"gh api repos/stitchai/platform/pulls/147/reviews":  {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/145/reviews":  {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/142/comments": {Stdout: commentsData},
			"gh api repos/stitchai/platform/pulls/147/comments": {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/145/comments": {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/142/commits":  {Stdout: prCommitsData},
			"gh api repos/stitchai/platform/pulls/147/commits":  {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/145/commits":  {Stdout: "[]"},
			"gh api graphql -f query=query {\n  repository(owner: \"stitchai\", name: \"platform\") {\n    pullRequest(number: 142)": {Stdout: graphqlResponse142},
			"gh api graphql -f query=query {\n  repository(owner: \"stitchai\", name: \"platform\") {\n    pullRequest(number: 147)": {Stdout: emptyGraphql},
			"gh api graphql -f query=query {\n  repository(owner: \"stitchai\", name: \"platform\") {\n    pullRequest(number: 145)": {Stdout: emptyGraphql},
		},
	}

	client := &GitHubClient{
		Repo:     "stitchai/platform",
		Executor: mock,
	}

	prs, err := client.FetchPRs("STITCH-1234")
	if err != nil {
		t.Fatalf("FetchPRs returned error: %v", err)
	}

	if len(prs) != 3 {
		t.Fatalf("expected 3 PRs, got %d", len(prs))
	}

	// Check first PR (should be #142 based on JSON order).
	pr := prs[0]
	if pr.Number != 142 {
		t.Errorf("expected PR #142, got #%d", pr.Number)
	}
	if pr.Title != "STITCH-1234: Implement password reset email flow" {
		t.Errorf("unexpected PR title: %s", pr.Title)
	}
	if pr.State != "MERGED" {
		t.Errorf("expected state MERGED, got %s", pr.State)
	}
	if pr.Author != "alicec" {
		t.Errorf("expected author alicec, got %s", pr.Author)
	}
	if pr.Branch != "feature/STITCH-1234-password-reset" {
		t.Errorf("unexpected branch: %s", pr.Branch)
	}

	// Check reviews for first PR.
	if len(pr.Reviews) != 3 {
		t.Fatalf("expected 3 reviews for PR #142, got %d", len(pr.Reviews))
	}
	if pr.Reviews[0].Author != "bobm" {
		t.Errorf("expected first review from bobm, got %s", pr.Reviews[0].Author)
	}
	if pr.Reviews[0].State != "CHANGES_REQUESTED" {
		t.Errorf("expected CHANGES_REQUESTED, got %s", pr.Reviews[0].State)
	}
	if pr.Reviews[1].State != "APPROVED" {
		t.Errorf("expected second review APPROVED, got %s", pr.Reviews[1].State)
	}

	// Check review comments for first PR.
	if len(pr.Comments) != 5 {
		t.Fatalf("expected 5 comments for PR #142, got %d", len(pr.Comments))
	}
	if pr.Comments[0].Path != "internal/auth/token.go" {
		t.Errorf("unexpected comment path: %s", pr.Comments[0].Path)
	}
	if pr.Comments[1].InReplyTo != 90001 {
		t.Errorf("expected InReplyTo 90001 for reply comment, got %d", pr.Comments[1].InReplyTo)
	}

	// Check ID field on comments.
	if pr.Comments[0].ID != 90001 {
		t.Errorf("expected comment ID 90001, got %d", pr.Comments[0].ID)
	}
	if pr.Comments[1].ID != 90002 {
		t.Errorf("expected comment ID 90002, got %d", pr.Comments[1].ID)
	}

	// Check Resolved field: comment 90001 is thread-start and should be resolved (true).
	if !pr.Comments[0].Resolved {
		t.Error("expected comment 90001 to be resolved")
	}
	// Comment 90002 is a reply (not a thread-start), so it won't be in the resolved map — should be false.
	if pr.Comments[1].Resolved {
		t.Error("expected comment 90002 (reply) to not be resolved")
	}
	// Comment 90003 is a thread-start, should be unresolved (false).
	if pr.Comments[2].Resolved {
		t.Error("expected comment 90003 to be unresolved")
	}
	// Comment 90004 is a thread-start, should be resolved (true).
	if !pr.Comments[3].Resolved {
		t.Error("expected comment 90004 to be resolved")
	}

	// Check commits for first PR.
	if len(pr.Commits) != 2 {
		t.Fatalf("expected 2 commits for PR #142, got %d", len(pr.Commits))
	}
	if pr.Commits[0].SHA != "abc123" {
		t.Errorf("unexpected commit SHA: %s", pr.Commits[0].SHA)
	}
}

func TestFetchPRs_NoPRsFound(t *testing.T) {
	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh pr list": {Stdout: "[]"},
		},
	}

	client := &GitHubClient{
		Repo:     "stitchai/platform",
		Executor: mock,
	}

	prs, err := client.FetchPRs("NONEXIST-999")
	if err != nil {
		t.Fatalf("FetchPRs returned error: %v", err)
	}

	if len(prs) != 0 {
		t.Errorf("expected 0 PRs, got %d", len(prs))
	}
}

func TestFetchPRs_CommandFails(t *testing.T) {
	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh pr list": {
				Stderr: "gh: Not logged in. Run `gh auth login`.",
				Err:    fmt.Errorf("exit status 1"),
			},
		},
	}

	client := &GitHubClient{
		Repo:     "stitchai/platform",
		Executor: mock,
	}

	_, err := client.FetchPRs("STITCH-1234")
	if err == nil {
		t.Fatal("expected error when gh command fails")
	}
	if !strings.Contains(err.Error(), "gh pr list failed") {
		t.Errorf("expected 'gh pr list failed' in error, got: %s", err.Error())
	}
}

func TestFetchMainCommits_Success(t *testing.T) {
	commitsData := string(loadTestdata(t, "gh_commits.json"))

	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh api repos/stitchai/platform/commits": {Stdout: commitsData},
		},
	}

	client := &GitHubClient{
		Repo:     "stitchai/platform",
		Executor: mock,
	}

	commits, err := client.FetchMainCommits("STITCH-1234")
	if err != nil {
		t.Fatalf("FetchMainCommits returned error: %v", err)
	}

	// The testdata has 4 commits total, but only 2 contain "STITCH-1234".
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits matching STITCH-1234, got %d", len(commits))
	}
	if commits[0].SHA != "b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1" {
		t.Errorf("unexpected first commit SHA: %s", commits[0].SHA)
	}
	if commits[0].Author != "alicec" {
		t.Errorf("expected commit author alicec, got %s", commits[0].Author)
	}
	if commits[1].SHA != "d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3" {
		t.Errorf("unexpected second commit SHA: %s", commits[1].SHA)
	}
}

func TestFetchMainCommits_NoMatching(t *testing.T) {
	commitsData := string(loadTestdata(t, "gh_commits.json"))

	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh api repos/stitchai/platform/commits": {Stdout: commitsData},
		},
	}

	client := &GitHubClient{
		Repo:     "stitchai/platform",
		Executor: mock,
	}

	commits, err := client.FetchMainCommits("NONEXIST-999")
	if err != nil {
		t.Fatalf("FetchMainCommits returned error: %v", err)
	}

	if len(commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(commits))
	}
}

func TestFetchMainCommits_CommandFails(t *testing.T) {
	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh api": {
				Stderr: "gh: HTTP 403",
				Err:    fmt.Errorf("exit status 1"),
			},
		},
	}

	client := &GitHubClient{
		Repo:     "stitchai/platform",
		Executor: mock,
	}

	_, err := client.FetchMainCommits("STITCH-1234")
	if err == nil {
		t.Fatal("expected error when gh api fails")
	}
}

func TestFetchPRs_VerifiesCommandArguments(t *testing.T) {
	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh pr list": {Stdout: "[]"},
		},
	}

	client := &GitHubClient{
		Repo:     "myorg/myrepo",
		Executor: mock,
	}

	_, err := client.FetchPRs("TICKET-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}

	call := mock.Calls[0]
	expectedParts := []string{
		"gh pr list",
		"--repo myorg/myrepo",
		"--search TICKET-42",
		"--state all",
		"--limit 100",
	}
	for _, part := range expectedParts {
		if !strings.Contains(call, part) {
			t.Errorf("expected call to contain %q, got: %s", part, call)
		}
	}
}

func TestFetchMainCommits_VerifiesAPICall(t *testing.T) {
	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh api": {Stdout: "[]"},
		},
	}

	client := &GitHubClient{
		Repo:     "myorg/myrepo",
		Executor: mock,
	}

	_, err := client.FetchMainCommits("TICKET-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}

	call := mock.Calls[0]
	if !strings.Contains(call, "repos/myorg/myrepo/commits") {
		t.Errorf("expected call to contain repo path, got: %s", call)
	}
	if !strings.Contains(call, "sha=main") {
		t.Errorf("expected call to contain sha=main, got: %s", call)
	}
}

func TestFetchPRsSince_FiltersCorrectly(t *testing.T) {
	prListData := string(loadTestdata(t, "gh_pr_list.json"))
	emptyGraphql := `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}`

	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh pr list": {Stdout: prListData},
			"gh api repos/stitchai/platform/pulls/142/reviews":  {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/147/reviews":  {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/145/reviews":  {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/142/comments": {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/147/comments": {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/145/comments": {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/142/commits":  {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/147/commits":  {Stdout: "[]"},
			"gh api repos/stitchai/platform/pulls/145/commits":  {Stdout: "[]"},
			"gh api graphql": {Stdout: emptyGraphql},
		},
	}

	client := &GitHubClient{
		Repo:     "stitchai/platform",
		Executor: mock,
	}

	// Filter for PRs updated after April 7, 2026 — should get #147 (updated April 8)
	// and #142 is excluded (updated April 6).
	since := time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC)
	prs, err := client.FetchPRsSince("STITCH-1234", since)
	if err != nil {
		t.Fatalf("FetchPRsSince returned error: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR after April 7, got %d", len(prs))
	}
	if prs[0].Number != 147 {
		t.Errorf("expected PR #147, got #%d", prs[0].Number)
	}
}

func TestFetchMainCommitsSince_VerifiesSinceParam(t *testing.T) {
	mock := &MockExecutor{
		Responses: map[string]MockResponse{
			"gh api": {Stdout: "[]"},
		},
	}

	client := &GitHubClient{
		Repo:     "stitchai/platform",
		Executor: mock,
	}

	since := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	_, err := client.FetchMainCommitsSince("STITCH-1234", since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}

	call := mock.Calls[0]
	if !strings.Contains(call, "since=2026-04-05T00:00:00Z") {
		t.Errorf("expected call to contain since parameter, got: %s", call)
	}
}

func TestDefaultExecutor_Interface(t *testing.T) {
	// Verify DefaultExecutor implements CommandExecutor.
	var _ CommandExecutor = &DefaultExecutor{}
}

func TestGitHubTestdataFixtures(t *testing.T) {
	// Verify all GitHub testdata fixtures are valid JSON.
	fixtures := []string{
		"gh_pr_list.json",
		"gh_pr_reviews.json",
		"gh_pr_comments.json",
		"gh_commits.json",
	}
	for _, f := range fixtures {
		data, err := os.ReadFile("../../testdata/" + f)
		if err != nil {
			t.Fatalf("failed to read %s: %v", f, err)
		}
		var v interface{}
		if err := json.Unmarshal(data, &v); err != nil {
			t.Errorf("%s is not valid JSON: %v", f, err)
		}
	}
}
