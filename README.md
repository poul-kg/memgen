# MemGen

MemGen is a networked MCP (Model Context Protocol) server written in Go that provides distributed knowledge management for AI coding agents. Multiple agents running on different machines connect to a single MemGen instance over HTTP to share and retrieve contextual knowledge tied to JIRA tickets -- ticket details, PR discussions, review comments, and team decisions.

Knowledge files are structured YAML assembled directly by Go code from JIRA and GitHub data. There is no LLM in the loop -- output is deterministic and fast. Agents pass their git branch, MemGen extracts the JIRA ticket ID, and the corresponding knowledge file is created, read, updated, or refreshed.

## Prerequisites

- **Go 1.25+** (for building from source)
- **GitHub CLI (`gh`)** -- installed and authenticated
- **JIRA Cloud instance** with an API token

### Installing the GitHub CLI

**Fedora:**

```bash
sudo dnf install gh
```

**Ubuntu / Debian:**

```bash
sudo apt install gh
```

**macOS:**

```bash
brew install gh
```

**Windows:**

```bash
winget install GitHub.cli
```

For other platforms, see the [official installation guide](https://github.com/cli/cli#installation).

After installing, authenticate:

```bash
gh auth login
```

### JIRA API Token

Generate a JIRA API token at [https://id.atlassian.com/manage-profile/security/api-tokens](https://id.atlassian.com/manage-profile/security/api-tokens).

## Build & Run

### Native

```bash
go build -o memgen ./cmd/memgen
./memgen
```

### Docker

```bash
docker compose build
docker compose up -d
```

### First-Run Behavior

On the first run, MemGen creates `~/.config/memgen/` and writes a sample `config.toml` with placeholder values, then exits with a message telling you to edit it. Once configured, it validates the `gh` CLI and starts the HTTP server.

## Configuration

MemGen reads its configuration from `~/.config/memgen/config.toml`. Here is the full format:

```toml
[server]
port = 3040

[jira]
url = "https://your-company.atlassian.net"
email = "user@example.com"
token = "your-jira-api-token"
```

| Field | Description |
|-------|-------------|
| `server.port` | HTTP port for the MCP server (default: `3040`) |
| `jira.url` | Your JIRA Cloud instance URL |
| `jira.email` | Your JIRA account email |
| `jira.token` | JIRA API token (see Prerequisites) |

## Connecting from Claude Code

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

The `x-mcp-repo` header is required on every request. It determines which subdirectory under `~/.config/memgen/knowledge/` is used for storage. Replace `your-memgen-host` with the machine running MemGen and `your-org/your-repo` with your GitHub repository in `owner/repo` format.

## Slash Commands

Copy the command files to your project's `.claude/commands/` directory:

```bash
mkdir -p /path/to/your/project/.claude/commands
cp commands/*.md /path/to/your/project/.claude/commands/
```

### `/kg` -- Knowledge Get

Retrieves knowledge for the current branch's ticket. If no knowledge exists, automatically runs `memgen__init` first.

Supports an optional scope argument:

| Scope | What it returns |
|-------|-----------------|
| _(no scope)_ | Full knowledge file -- JIRA ticket, all PRs with reviews/comments/commits, main branch commits, and notes |
| `jira` | JIRA ticket only -- summary, description, status, priority, assignee, reporter, labels, and all JIRA comments |
| `pr` | PR summaries only -- number, title, state, author, URL, and body (no reviews/comments/commits) |
| `git` | Full PR data + main commits -- all PRs with reviews, comments, commits, plus commits on main branch |
| `comments` | PR review comments and reviews only -- all review comments (with resolved status) and review decisions across all PRs |
| `notes` | Notes only -- custom notes added via `/ks` |

Usage examples: `/kg`, `/kg jira`, `/kg comments`, `/kg notes`.

### `/ks` -- Knowledge Set

Stores a note for the current ticket. If `$ARGUMENTS` is provided, it is used directly as the note body. If omitted, the agent reviews the current conversation and summarizes key decisions, implementation choices, problems, and TODOs.

Usage examples: `/ks`, `/ks Decided to use bcrypt with cost factor 12`.

### `/kr` -- Knowledge Refresh

Re-fetches all data from JIRA and GitHub and rebuilds the knowledge file. Notes are preserved during refresh. After refreshing, presents the updated knowledge.

## MCP Tools Reference

All tools require the `branch` argument and the `x-mcp-repo` HTTP header.

| Tool | Arguments | Description |
|------|-----------|-------------|
| `memgen__init` | `branch` (required) | Gathers JIRA ticket data, GitHub PRs, and main branch commits. Creates a structured YAML knowledge file. Overwrites any existing file. |
| `memgen__get` | `branch` (required), `scope` (optional: `jira`, `pr`, `git`, `comments`, `notes`) | Reads the knowledge file. Returns a recommendation to run `init` if none exists. Warns if knowledge is older than 24 hours. |
| `memgen__set` | `branch` (required), `note` (required) | Appends a timestamped note to the knowledge file. Knowledge file must already exist. The `decisions` argument is accepted as a deprecated alias for `note`. |
| `memgen__refresh` | `branch` (required) | Re-fetches all data from JIRA and GitHub, rebuilds the knowledge file. Notes are preserved. Knowledge file must already exist. |

## Knowledge File Format

Knowledge is stored as YAML at:

```
~/.config/memgen/knowledge/<org>/<repo>/<TICKET-ID>.yaml
```

Top-level structure:

| Key | Description |
|-----|-------------|
| `ticket_id` | JIRA ticket ID (e.g., `SV1-240`) |
| `branch` | Git branch name used during init |
| `last_refreshed` | UTC timestamp of last init or refresh |
| `jira` | JIRA ticket details: summary, description, status, priority, assignee, reporter, labels, comments |
| `pull_requests` | All PRs referencing the ticket (open, closed, merged) with reviews, review comments (including resolved status), and commits |
| `main_commits` | Commits on the main branch whose message references the ticket |
| `notes` | Timestamped notes added via `memgen__set` or `/ks` |

Example:

```yaml
ticket_id: STITCH-1234
branch: feature/STITCH-1234-password-reset
last_refreshed: 2026-04-10T12:00:00Z
jira:
  summary: "Implement password reset flow"
  description: "Full description..."
  status: "In Progress"
  priority: "High"
  assignee: "Alice Chen"
  reporter: "Bob Miller"
  labels: [auth, security]
  comments:
    - author: "Carol N"
      created: 2026-04-03T10:00:00Z
      updated: 2026-04-03T10:00:00Z
      body: "Comment text"
pull_requests:
  - number: 142
    title: "STITCH-1234: Implement password reset"
    state: MERGED
    author: alicec
    url: https://github.com/org/repo/pull/142
    created_at: 2026-04-03T10:00:00Z
    updated_at: 2026-04-06T10:00:00Z
    branch: feature/STITCH-1234-password-reset
    body: "PR description"
    reviews:
      - author: bobm
        state: APPROVED
        body: "Looks good"
        created_at: 2026-04-05T10:00:00Z
    comments:
      - id: 90001
        author: bobm
        body: "Token expiry should be configurable"
        path: internal/auth/token.go
        created_at: 2026-04-04T10:05:00Z
        updated_at: 2026-04-04T10:05:00Z
        in_reply_to: 0
        resolved: true
    commits:
      - sha: abc123
        message: "STITCH-1234: initial implementation"
        author: alicec
        date: 2026-04-03T15:00:00Z
main_commits:
  - sha: def456
    message: "STITCH-1234: merged to main"
    author: alicec
    date: 2026-04-07T10:00:00Z
notes:
  - date: 2026-04-08T14:00:00Z
    body: "Decided to use bcrypt cost factor 12 in next security review"
```

## How It Works

1. An agent passes a git branch name (e.g., `SV1-240-mail-threading`).
2. MemGen extracts the JIRA ticket ID using the first match of `[A-Z][A-Z0-9]+-\d+` (yields `SV1-240`).
3. JIRA ticket details and comments are fetched via the JIRA REST API v3.
4. GitHub PRs referencing the ticket are found via `gh pr list --search`. Results are post-filtered for exact ticket ID matches to eliminate false positives from GitHub's fuzzy search.
5. For each PR: reviews, review comments (with resolved/unresolved status via GraphQL), and commits are fetched.
6. Commits on main referencing the ticket are fetched separately.
7. All data is assembled into a Go struct and written as YAML. No LLM processing is involved.

Mutating operations (`init`, `set`, `refresh`) use non-blocking per-ticket locks. If a lock is already held, the tool returns immediately with "try again later." The `get` tool is read-only and never locks.

## Docker Setup

The `Dockerfile` uses a multi-stage build:
1. **Builder stage** (`golang:latest`): builds the `memgen` binary with `CGO_ENABLED=0`.
2. **Runtime stage** (`debian:bookworm-slim`): installs the `gh` CLI and runs the binary.

The `docker-compose.yml` mounts the following volumes:

| Volume | Container Path | Mode | Purpose |
|--------|---------------|------|---------|
| `~/.config/gh` | `/home/memgen/.config/gh` | read-only | GitHub CLI authentication |
| `~/.config/memgen` | `/home/memgen/.config/memgen` | read-write | MemGen config and knowledge storage |

The container runs as the host user to avoid file permission issues:

```yaml
user: "${UID:-1000}:${GID:-1000}"
```

Set `UID` and `GID` environment variables if your user ID is not 1000. Port 3040 is mapped from the container to the host.

## Testing

```bash
# Run all unit tests
go test ./...

# Run tests verbose
go test -v ./...

# Run integration tests
go test -tags=integration ./...

# Run a single package
go test ./internal/tools/...
```
