package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/poul-kg/memgen/internal/tools"
)

type contextKey struct{ name string }

var repoContextKey = contextKey{"repo"}

// New creates and configures the MCP server with all tools registered.
func New(deps *tools.Deps) *mcpserver.MCPServer {
	s := mcpserver.NewMCPServer("memgen", "1.0.0")

	registerInit(s, deps)
	registerGet(s, deps)
	registerSet(s, deps)
	registerRefresh(s, deps)

	return s
}

// NewHTTP wraps the MCP server in an HTTP transport.
func NewHTTP(mcpSrv *mcpserver.MCPServer) *mcpserver.StreamableHTTPServer {
	return mcpserver.NewStreamableHTTPServer(mcpSrv,
		mcpserver.WithEndpointPath("/mcp"),
		mcpserver.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			repo := r.Header.Get("x-mcp-repo")
			return context.WithValue(ctx, repoContextKey, repo)
		}),
	)
}

// repoFromContext extracts the repo header value from context.
func repoFromContext(ctx context.Context) (string, error) {
	repo, ok := ctx.Value(repoContextKey).(string)
	if !ok || repo == "" {
		return "", fmt.Errorf("missing required header x-mcp-repo")
	}
	return repo, nil
}

// textResult creates a simple text MCP result.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: text},
		},
	}
}

// errorResult creates an MCP error result.
func errorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: text},
		},
		IsError: true,
	}
}

func registerInit(s *mcpserver.MCPServer, deps *tools.Deps) {
	s.AddTool(mcp.Tool{
		Name:        "memgen__init",
		Description: "Initialize knowledge for a JIRA ticket by gathering data from JIRA, GitHub PRs, and commits. Requires branch argument. Use a sub-agent to run this tool.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Git branch name containing the JIRA ticket ID",
				},
			},
			Required: []string{"branch"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := repoFromContext(ctx)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		branch, err := request.RequireString("branch")
		if err != nil {
			return errorResult(err.Error()), nil
		}
		result, err := tools.Init(deps, repo, branch)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		return textResult(result), nil
	})
}

func registerGet(s *mcpserver.MCPServer, deps *tools.Deps) {
	s.AddTool(mcp.Tool{
		Name:        "memgen__get",
		Description: "Retrieve stored knowledge for the current branch's JIRA ticket. Requires branch argument.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Git branch name containing the JIRA ticket ID",
				},
				"scope": map[string]any{
					"type":        "string",
					"description": "Filter which section to return: jira, pr, git, comments, notes. Omit for full knowledge.",
					"enum":        []string{"jira", "pr", "git", "comments", "notes"},
				},
			},
			Required: []string{"branch"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := repoFromContext(ctx)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		branch, err := request.RequireString("branch")
		if err != nil {
			return errorResult(err.Error()), nil
		}
		scope, _ := request.GetArguments()["scope"].(string)
		result, err := tools.Get(deps, repo, branch, scope)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		return textResult(result), nil
	})
}

func registerSet(s *mcpserver.MCPServer, deps *tools.Deps) {
	s.AddTool(mcp.Tool{
		Name:        "memgen__set",
		Description: "Append a note to the knowledge file for the current branch's ticket. Requires branch and note arguments.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Git branch name containing the JIRA ticket ID",
				},
				"note": map[string]any{
					"type":        "string",
					"description": "A note to append to the knowledge file with a UTC timestamp",
				},
				"decisions": map[string]any{
					"type":        "string",
					"description": "Deprecated: use 'note' instead. Key decisions to store.",
				},
			},
			Required: []string{"branch"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := repoFromContext(ctx)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		branch, err := request.RequireString("branch")
		if err != nil {
			return errorResult(err.Error()), nil
		}
		note, _ := request.GetArguments()["note"].(string)
		if note == "" {
			note, _ = request.GetArguments()["decisions"].(string)
		}
		if note == "" {
			return errorResult("Missing required argument: 'note' (or 'decisions')"), nil
		}
		result, err := tools.Set(deps, repo, branch, note)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		return textResult(result), nil
	})
}

func registerRefresh(s *mcpserver.MCPServer, deps *tools.Deps) {
	s.AddTool(mcp.Tool{
		Name:        "memgen__refresh",
		Description: "Refresh knowledge with latest data from JIRA and GitHub. Notes are preserved. Requires branch argument. Use a sub-agent to run this tool.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Git branch name containing the JIRA ticket ID",
				},
			},
			Required: []string{"branch"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := repoFromContext(ctx)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		branch, err := request.RequireString("branch")
		if err != nil {
			return errorResult(err.Error()), nil
		}
		result, err := tools.Refresh(deps, repo, branch)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		return textResult(result), nil
	})
}
