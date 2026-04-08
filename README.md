# MemGen

Networked MCP server that provides distributed knowledge management for AI coding agents. Multiple agents on different machines share context about JIRA tickets — ticket details, PR discussions, review comments, and key decisions — through a single MemGen instance.

Agents connect over HTTP, pass their git branch, and MemGen extracts the JIRA ticket ID to manage knowledge files. Knowledge is assembled and maintained by Claude (Opus 4.6) which summarizes, merges, and deduplicates data from JIRA, GitHub PRs, and agent-provided decisions.

## Prerequisites

- Go 1.22+
- [Claude CLI](https://docs.anthropic.com/en/docs/claude-cli) — installed and authenticated
- [GitHub CLI (`gh`)](https://cli.github.com/) — installed and authenticated

## Build

```bash
go build -o memgen ./cmd/memgen
```

## Configuration

On first run, MemGen creates `~/.config/memgen/config.toml` with sample values:

```toml
[server]
port = 3040

[jira]
url = "https://your-company.atlassian.net"
email = "user@example.com"
token = "your-jira-api-token"
```

Edit the file with your JIRA Cloud credentials:

| Field | Description |
|-------|-------------|
| `server.port` | HTTP port for the MCP server (default: 3040) |
| `jira.url` | Your JIRA Cloud instance URL |
| `jira.email` | Your JIRA account email |
| `jira.token` | [JIRA API token](https://id.atlassian.com/manage-profile/security/api-tokens) |

## Run

```bash
./memgen
```

MemGen validates that `claude` and `gh` CLIs are installed and authenticated before accepting connections.

## Connect from Claude Code

Add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "memgen": {
      "type": "http",
      "url": "http://your-memgen-host:3040/mcp",
      "headers": {
        "x-mcp-repo": "your-org/your-repo"
      }
    }
  }
}
```

Replace `your-memgen-host` with the machine running MemGen, and `your-org/your-repo` with your GitHub repository.

## Slash Commands

Copy the command files from `commands/` to your project's `.claude/commands/` directory:

```bash
cp commands/*.md /path/to/your/project/.claude/commands/
```

| Command | Description |
|---------|-------------|
| `/kg` | **Knowledge Get** — Retrieve knowledge for the current branch's ticket. Auto-initializes if no knowledge exists. |
| `/ks` | **Knowledge Set** — Summarize key decisions from the current session and store them. |
| `/kr` | **Knowledge Refresh** — Fetch latest data from JIRA and GitHub since last refresh. |

## MCP Tools

All tools require a `branch` argument (the current git branch name) and the `x-mcp-repo` header.

### `memgen__init`

Gathers knowledge from JIRA, GitHub PRs, and commits. Creates a structured knowledge file summarized by Claude.

### `memgen__get`

Retrieves stored knowledge. Returns a recommendation to run `memgen__init` if no knowledge exists. Includes a staleness warning if knowledge is older than 24 hours.

### `memgen__set`

Stores key decisions into the knowledge file. Claude merges new decisions with existing content, deduplicating as needed. Requires `decisions` argument.

### `memgen__refresh`

Refreshes knowledge by fetching only new data since the last refresh timestamp.

## How It Works

1. Agent calls `/kg` (or `memgen__get` directly) with the current branch.
2. MemGen extracts the JIRA ticket ID from the branch name (e.g., `SV1-240` from `SV1-240-mail-threading`).
3. If no knowledge exists, the agent calls `memgen__init` which:
   - Fetches the JIRA ticket details and comments
   - Finds all GitHub PRs referencing the ticket (open, closed, merged)
   - Fetches PR review comments, change requests, and commits
   - Finds commits on main referencing the ticket
   - Pipes all data to Claude for structured summarization
4. Knowledge is stored as a Markdown file in `~/.config/memgen/knowledge/<org>/<repo>/<TICKET-ID>.md`.
5. Any agent on any machine can retrieve this knowledge via `memgen__get`.
6. Agents store session decisions via `memgen__set`, which Claude merges into the knowledge file.
7. Agents refresh with latest JIRA/GitHub data via `memgen__refresh`.

## Knowledge Storage

```
~/.config/memgen/
  config.toml
  knowledge/
    your-org/
      your-repo/
        SV1-240.md
        SV1-241.md
```

## Testing

```bash
# Unit tests
go test ./...

# Integration tests
go test -tags=integration ./...
```
