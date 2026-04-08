package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/poul-kg/memgen/internal/claude"
	"github.com/poul-kg/memgen/internal/knowledge"
	"github.com/poul-kg/memgen/internal/sources"
	"github.com/poul-kg/memgen/internal/tools"
)

func testDeps() *tools.Deps {
	return &tools.Deps{
		Store:       knowledge.NewStore("/tmp/memgen-test-knowledge"),
		Locks:       knowledge.NewLockManager(),
		JIRA:        &sources.JIRAClient{BaseURL: "https://test.atlassian.net", Email: "test@test.com", Token: "tok"},
		GitHub:      &sources.GitHubClient{Repo: "owner/repo", Executor: &sources.DefaultExecutor{}},
		Claude:      claude.NewCLI(),
		JIRABaseURL: "https://test.atlassian.net",
	}
}

func TestNew_RegistersFourTools(t *testing.T) {
	deps := testDeps()
	s := New(deps)
	if s == nil {
		t.Fatal("New returned nil")
	}

	// Use ListTools to verify 4 tools are registered.
	// First initialize the server session.
	s.HandleMessage(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2025-03-26",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0.0"}
		}
	}`))

	listResult := s.HandleMessage(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "tools/list",
		"params": {}
	}`))

	// Marshal the result to JSON string for inspection.
	resultBytes, err := json.Marshal(listResult)
	if err != nil {
		t.Fatalf("failed to marshal tools/list result: %v", err)
	}
	resultStr := string(resultBytes)
	expectedTools := []string{"memgen__init", "memgen__get", "memgen__set", "memgen__refresh"}
	for _, name := range expectedTools {
		if !contains(resultStr, name) {
			t.Errorf("expected tool %q in tools/list response, got: %s", name, resultStr)
		}
	}
}

func TestRepoFromContext_WithValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), repoContextKey, "owner/repo")
	repo, err := repoFromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "owner/repo" {
		t.Errorf("expected %q, got %q", "owner/repo", repo)
	}
}

func TestRepoFromContext_WithoutValue(t *testing.T) {
	ctx := context.Background()
	_, err := repoFromContext(ctx)
	if err == nil {
		t.Fatal("expected error for missing repo context")
	}
}

func TestRepoFromContext_EmptyString(t *testing.T) {
	ctx := context.WithValue(context.Background(), repoContextKey, "")
	_, err := repoFromContext(ctx)
	if err == nil {
		t.Fatal("expected error for empty repo context")
	}
}

func TestTextResult(t *testing.T) {
	result := textResult("hello world")
	if result.IsError {
		t.Error("expected IsError to be false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Type != "text" {
		t.Errorf("expected type %q, got %q", "text", tc.Type)
	}
	if tc.Text != "hello world" {
		t.Errorf("expected text %q, got %q", "hello world", tc.Text)
	}
}

func TestErrorResult(t *testing.T) {
	result := errorResult("something failed")
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Type != "text" {
		t.Errorf("expected type %q, got %q", "text", tc.Type)
	}
	if tc.Text != "something failed" {
		t.Errorf("expected text %q, got %q", "something failed", tc.Text)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
