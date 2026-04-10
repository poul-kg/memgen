# MemGen

Networked MCP server in Go that provides distributed knowledge management for AI coding agents, tied to JIRA tickets.

## Build & Test

```bash
# Build
go build -o memgen ./cmd/memgen

# Run all unit tests
go test ./...

# Run tests verbose
go test -v ./...

# Run integration tests
go test -tags=integration ./...

# Run a single package's tests
go test ./internal/tools/...
```

## Project Structure

```
cmd/memgen/main.go          -- Entry point, startup validation, config loading
internal/
  config/                    -- TOML config parsing and validation
  knowledge/                 -- File store (store.go), per-ticket locks (lock.go), YAML types (types.go), source conversion (convert.go)
  ticket/                    -- Branch-to-JIRA-ticket extraction
  sources/                   -- JIRA REST API client + GitHub gh CLI wrapper
  tools/                     -- MCP tool implementations (init, get, set, refresh)
  server/                    -- HTTP/MCP server setup and tool registration
commands/                    -- Claude Code slash commands (/kg, /ks, /kr)
testdata/                    -- JSON fixtures for JIRA and GitHub API mocks
```

## Architecture

- **Transport**: HTTP-based MCP server using `mcp-go` library (streamable HTTP). Not stdio.
- **Storage**: YAML files (.yaml) in `~/.config/memgen/knowledge/<org>/<repo>/<TICKET>.yaml`
- **Config**: TOML at `~/.config/memgen/config.toml`
- **External deps**: `gh` CLI for GitHub data, JIRA REST API v3 for ticket data.
- **Concurrency**: In-process non-blocking `TryLock` per ticket. Returns "try again later" if locked.
- **Repo**: Passed via `x-mcp-repo` HTTP header. Branch passed as tool argument.

## Code Conventions

- Go standard layout: `cmd/` for binaries, `internal/` for private packages.
- Table-driven tests with `t.Parallel()` where possible.
- External commands (`gh`) are abstracted behind `CommandExecutor` interfaces for testability.
- JIRA API is tested via `httptest.NewServer` mocks.
- All timestamps are UTC (RFC3339 format).
- No authentication on the MCP server — local network only.

## Key Interfaces for Mocking

- `sources.CommandExecutor` — for `gh` CLI calls
- JIRA client uses real HTTP, mocked via `httptest.NewServer` in tests

## MCP Tools

| Tool | Args | Description |
|------|------|-------------|
| `memgen__init` | `branch` | Gathers JIRA + GitHub data, creates structured YAML knowledge file |
| `memgen__get` | `branch`, `scope` (optional) | Reads knowledge file, warns if stale. Scope filters: jira, pr, git, comments, notes |
| `memgen__set` | `branch`, `note` | Appends a timestamped note to the knowledge file |
| `memgen__refresh` | `branch` | Re-fetches all data from JIRA and GitHub, preserves notes |

## Code Quality

Before committing, use the JetBrains MCP `get_file_problems` tool (with `errorsOnly: false`) on every changed `.go` file to check for errors AND warnings. Fix all reported issues before committing. Common issues:
- Unhandled errors: use `_, _ = w.Write(...)` for `http.ResponseWriter.Write` in test handlers, or handle errors explicitly in production code.
- Unused variables/imports.
- Unresolved references.

## Spec

See `initial-plan.md` for the full technical specification.
