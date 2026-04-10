# MemGen - Technical Specification

## Build Prompt for Claude Code Agent

You are building **MemGen**, a networked MCP (Model Context Protocol) server written in Go that provides distributed knowledge management for AI coding agents. Multiple agents running on different machines connect to a single MemGen instance over HTTP to share and retrieve contextual knowledge tied to JIRA tickets.

---

## 1. Overview

### Problem

Multiple AI coding agents work interchangeably on tasks tied to JIRA tickets across different machines. Each agent lacks awareness of prior context — JIRA details, PR discussions, past decisions, outstanding review comments. There is no shared memory between sessions or machines.

### Solution

A single Go binary (`memgen`) that runs as an HTTP-based MCP server on a Linux machine in the local network. Agents connect via MCP-over-HTTP, pass their current git branch, and MemGen extracts the JIRA ticket ID, manages knowledge files, and provides tools to initialize, retrieve, update, and refresh knowledge.

Knowledge is assembled directly by Go code that converts JIRA and GitHub data into structured YAML files. No LLM is involved in knowledge assembly.

### Key Design Principles

- **Network-first**: HTTP server, not stdio. Multiple machines, one server.
- **No authentication**: Local network only. Auth is a future concern.
- **Fail-fast startup**: Validate all external dependencies before accepting connections.
- **Explicit over implicit**: Tools fail if required arguments are missing. No silent fallbacks.
- **Non-blocking locks**: If an operation is already in progress for a ticket, return immediately with "try again later."
- **UTC everywhere**: All timestamps in UTC. No timezone ambiguity between MCP server, JIRA, and GitHub.

---

## 2. Architecture

### Runtime Dependencies (validated at startup)

The MCP server MUST validate the following before accepting any connections. If any check fails, exit with a clear, actionable error message.

1. **`gh` CLI**: Must be installed and authenticated. Invoke `gh auth status` to verify. If not installed or not authenticated, exit with:
   - Installation instructions for Fedora (primary), plus macOS, Windows, Ubuntu, and other Linux distros.
   - Instructions to run `gh auth login`.

### Config & Storage

**Config directory**: `~/.config/memgen/` (Linux only, hardcoded path).

**Config file**: `~/.config/memgen/config.toml`

```toml
[server]
port = 3040

[jira]
url = "https://stitchai.atlassian.net"
email = "user@example.com"
token = "your-jira-api-token"
```

**First-run behavior**:
1. If `~/.config/memgen/` does not exist, create it.
2. If `config.toml` does not exist, create a sample file with placeholder values and exit with a message telling the user to edit it.
3. If `config.toml` exists but contains placeholder/default values, exit with a message telling the user to fill in real values.
4. If `config.toml` is properly configured, proceed to dependency validation and startup.

**Knowledge storage**: YAML files organized by repository, then by ticket ID.

```
~/.config/memgen/
  config.toml
  knowledge/
    stitch-ai/
      stitch-mono/
        SV1-240.yaml
        SV1-241.yaml
        SBUX-111.yaml
    other-org/
      other-repo/
        PROJ-100.yaml
```

### MCP HTTP Server

- Listens on the port specified in `config.toml` (default: `3040`).
- HTTP only (no TLS). TLS termination will be handled by a reverse proxy if needed in the future.
- MCP endpoint: serves MCP protocol over HTTP per the MCP specification.

### Required Headers

Every MCP request must include:

| Header | Description | Example |
|--------|-------------|---------|
| `x-mcp-repo` | GitHub repository in `owner/repo` format | `stitch-ai/stitch-mono` |

The repo header determines which subdirectory under `knowledge/` is used for storage.

Example `.mcp.json` for Claude Code:

```json
{
  "mcpServers": {
    "memgen": {
      "type": "http",
      "url": "http://memgen-host:3040/mcp",
      "headers": {
        "x-mcp-repo": "stitch-ai/stitch-mono"
      }
    }
  }
}
```

---

## 3. Ticket Detection

### Branch Pattern

Extract the JIRA ticket ID from the branch name using the **first match** of the pattern `[A-Z][A-Z0-9]+-\d+`.

Examples:
- `SV1-240-mail-threading` -> `SV1-240`
- `SBUX-111-some-task` -> `SBUX-111`
- `SAI-342-some-task` -> `SAI-342`
- `SV1-240` -> `SV1-240` (bare ticket, no suffix)

### Validation

If no ticket pattern is found in the branch name, return an error: "No JIRA ticket detected in branch name `<branch>`. Branch must contain a ticket ID like SV1-240."

JIRA ticket existence is validated during `init` and `refresh` when `FetchTicket()` is called. If the JIRA API returns 404, the error includes:
- The detected ticket ID.
- A browse URL generated from the configured JIRA base URL: `<jira-url>/browse/<TICKET-ID>`.
- The error message from the JIRA client.

The `ticket.BrowseURL()` helper generates browse URLs using the JIRA base URL from config (not hardcoded).

---

## 4. MCP Tools

All tools require `branch` as a **required argument**. All tools extract the repo from the `x-mcp-repo` header. If either is missing, the tool MUST return an error.

### 4.1 `memgen__init`

**Purpose**: Gather knowledge from all external sources and create the initial knowledge file.

**Concurrency**: Acquire an in-process lock keyed by `<repo>/<ticket-id>`. If the lock is already held (another `init` or `set` is running for this ticket), return immediately: "Operation already in progress for `<TICKET-ID>`. Try again later."

**Steps**:

1. Extract ticket ID from `branch` argument.
2. Create per-request GitHub client and validate the repo exists.
3. Fetch JIRA ticket data (validates ticket exists in JIRA, see Section 3).
4. Fetch GitHub PRs matching the ticket ID (with post-filtering for exact matches).
5. Fetch main branch commits referencing the ticket ID.
6. Build a `KnowledgeFile` struct directly from source data via `knowledge.FromSources()`.
7. Write the result as YAML to `~/.config/memgen/knowledge/<owner>/<repo>/<TICKET-ID>.yaml`.
8. Return success with a summary of what was gathered (e.g., "Initialized knowledge for SV1-240: JIRA ticket + 3 PRs + 12 commits on main").

**If the knowledge file already exists**: Overwrite it. `init` is a full re-initialization.

### 4.2 `memgen__get`

**Purpose**: Retrieve stored knowledge for the current branch's ticket.

**Arguments**: `branch` (required), `scope` (optional).

**Scope filtering**: The `scope` parameter allows returning only a specific section of the knowledge file. Valid scopes are:
- `""` (empty/omitted) -- full YAML knowledge file
- `"jira"` -- JIRA ticket section only
- `"pr"` -- slim PR summaries (number, title, state, author, URL, body -- no reviews/comments/commits)
- `"git"` -- full PR data + main branch commits
- `"comments"` -- all PR review comments and reviews aggregated across all PRs
- `"notes"` -- notes section only

**Steps**:

1. Extract ticket ID from `branch` argument.
2. If the file does not exist:
   - Return HTTP 200 with message: "No knowledge found for `<TICKET-ID>`. Run `memgen__init` to initialize knowledge for this ticket."
   - The calling agent is expected to follow this recommendation.
3. If scope is provided, use `ReadSection()` to return only that YAML section.
4. If no scope, read and unmarshal the full YAML, re-marshal and return.
5. Check the `last_refreshed` timestamp. If older than 24 hours, append a staleness warning: "Warning: knowledge is N days old" with a suggestion to run `memgen__refresh`.
6. Never block or fail based on staleness -- information only.

### 4.3 `memgen__set`

**Purpose**: Append a timestamped note to the knowledge file for the current branch's ticket.

**Arguments**: `branch` (required), `note` (required). The `decisions` argument is accepted as a deprecated alias for `note`.

**Concurrency**: Same lock mechanism as `init`. If locked, return "try again later."

**Required**: Knowledge file must already exist. If it does not exist, return an error: "No knowledge file found for `<TICKET-ID>`. Run `memgen__init` first."

**Steps**:

1. Extract ticket ID from `branch` argument.
2. Verify knowledge file exists.
3. Acquire lock for `<repo>/<ticket-id>`.
4. Read existing knowledge file as a `KnowledgeFile` struct.
5. Append a new `Note` entry with the current UTC timestamp and the provided note body.
6. Write the updated struct back as YAML.
7. Return success: "Added note for `<TICKET-ID>`."

### 4.4 `memgen__refresh`

**Purpose**: Refresh knowledge by re-fetching all data from JIRA and GitHub. Notes are preserved.

**Concurrency**: Same lock mechanism. If locked, return "try again later."

**Required**: Knowledge file must already exist. If not, return error telling agent to run `init`.

**Steps**:

1. Extract ticket ID from `branch` argument.
2. Verify knowledge file exists.
3. Create per-request GitHub client and validate repo.
4. Acquire lock for `<repo>/<ticket-id>`.
5. Read existing knowledge file, save the Notes section.
6. Full re-fetch from all sources: JIRA ticket, GitHub PRs (with filtering), main branch commits.
7. Build a new `KnowledgeFile` struct from fresh data via `knowledge.FromSources()`.
8. Restore the saved Notes section into the new struct.
9. Write as YAML.
10. Return summary: "Refreshed knowledge for `<TICKET-ID>`: JIRA ticket + N PRs + N commits on main."

---

## 5. Data Sources

All external data fetching uses the `gh` CLI for GitHub and the JIRA REST API (v3, Cloud) for JIRA. Source data is converted directly into Go structs (`knowledge.KnowledgeFile`) via `knowledge.FromSources()` -- no LLM processing is involved.

### 5.1 JIRA

- **API**: `https://<jira-url>/rest/api/3/issue/<TICKET-ID>?expand=renderedFields`
- **Auth**: Basic auth with email + API token from `config.toml`.
- **Fetch**:
  - Ticket summary, description (HTML stripped from rendered fields), status, assignee, reporter, priority, labels.
  - All comments via separate API call: `GET /rest/api/3/issue/<TICKET-ID>/comment?orderBy=created`
  - Comment body: prefers `renderedBody` (HTML stripped), falls back to recursive ADF text extraction.
  - Timestamps parsed from JIRA format (`2006-01-02T15:04:05.000-0700`).

### 5.2 GitHub PRs

- **Search**: `gh pr list --repo <repo> --search "<TICKET-ID>" --state all --json number,title,body,state,author,createdAt,updatedAt,headRefName,url --limit 100`
- **Post-filtering**: Results are filtered by `filterPREntriesByTicket()` which checks for an exact case-insensitive match of the ticket ID in PR title, body, or branch name. This eliminates false positives from GitHub's fuzzy search (e.g., "404" matching "SBUX-404").
- **For each PR found**:
  - PR metadata (title, body, state, author, URLs, timestamps, branch name).
  - Reviews: `gh api repos/<owner>/<repo>/pulls/<number>/reviews?per_page=100`
  - Review comments: `gh api repos/<owner>/<repo>/pulls/<number>/comments?per_page=100`
  - Review thread resolved status: fetched via GraphQL API (`repository.pullRequest.reviewThreads`). Maps each thread's first comment database ID to its `isResolved` status. Graceful fallback to `resolved: false` if GraphQL is unavailable.
  - Commits: `gh api repos/<owner>/<repo>/pulls/<number>/commits?per_page=100`
- **Include all PRs**: open, closed, merged.

### 5.3 GitHub Commits on Main

- **API**: `gh api repos/<owner>/<repo>/commits?sha=<branch>&per_page=100`
- **Branch detection**: Tries `main` first, then `master`.
- **Filtering**: Client-side filtering of commits whose message contains the ticket ID (exact string match).
- **Fetch**: Commit SHA, message, author (prefers GitHub login, falls back to git author name), date.

### 5.4 Existing Knowledge File

- If a `.yaml` file already exists for this ticket during `init`, it is **overwritten** (init is a full reset).
- During `refresh`, the existing file is read, notes are preserved, and all other data is replaced with fresh fetches.
- During `set`, the existing file is read and a new note is appended.
- Legacy `.md` files are detected and a helpful error is returned suggesting re-init.

---

## 6. Knowledge File Format

Each knowledge file is a structured YAML document (`.yaml`) assembled directly by Go code. The file maps to the `knowledge.KnowledgeFile` Go struct defined in `internal/knowledge/types.go`.

### Go Types

```go
type KnowledgeFile struct {
    TicketID      string        `yaml:"ticket_id"`
    Branch        string        `yaml:"branch"`
    LastRefreshed time.Time     `yaml:"last_refreshed"`
    JIRA          JIRASection   `yaml:"jira"`
    PullRequests  []PullRequest `yaml:"pull_requests"`
    MainCommits   []CommitEntry `yaml:"main_commits"`
    Notes         []Note        `yaml:"notes"`
}

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

type JIRAComment struct {
    Author  string    `yaml:"author"`
    Created time.Time `yaml:"created"`
    Updated time.Time `yaml:"updated"`
    Body    string    `yaml:"body"`
}

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

type PRReview struct {
    Author    string    `yaml:"author"`
    State     string    `yaml:"state"`
    Body      string    `yaml:"body"`
    CreatedAt time.Time `yaml:"created_at"`
}

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

type CommitEntry struct {
    SHA     string    `yaml:"sha"`
    Message string    `yaml:"message"`
    Author  string    `yaml:"author"`
    Date    time.Time `yaml:"date"`
}

type Note struct {
    Date time.Time `yaml:"date"`
    Body string    `yaml:"body"`
}
```

### Example YAML

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

### Conversion

Source data is converted to the YAML struct via `knowledge.FromSources(ticketID, branch, jiraTicket, prs, mainCommits)` in `internal/knowledge/convert.go`. This function maps source types (`sources.JIRATicket`, `sources.PR`, `sources.Commit`) to knowledge types (`JIRASection`, `PullRequest`, `CommitEntry`). All slices are initialized (never nil) to produce clean YAML output.

---

## 7. Data Processing

**No LLM dependency.** Knowledge files are assembled directly by Go code. The `claude` CLI is not used at runtime.

- **Init**: Source data (JIRA, GitHub) is fetched and converted to Go structs via `knowledge.FromSources()`, then marshaled to YAML via `gopkg.in/yaml.v3`.
- **Set**: The existing YAML is unmarshaled, a new `Note` is appended, and the struct is re-marshaled.
- **Refresh**: All source data is re-fetched fresh, converted to a new struct, existing notes are preserved, and the result is written as YAML.

This deterministic approach ensures reproducible output and eliminates the `claude` CLI as a runtime dependency.

---

## 8. Concurrency Control

### In-Process Lock Map

Maintain a `sync.Mutex`-based lock map keyed by `<owner>/<repo>/<ticket-id>`.

- Use `sync.Map` or a guarded `map[string]*sync.Mutex` to store per-ticket locks.
- Before any mutating operation (`init`, `set`, `refresh`), attempt to acquire the lock using a **non-blocking tryLock**.
- If the lock is already held, return immediately with HTTP 200 and a message: "An operation is already in progress for `<TICKET-ID>`. Try again later."
- Release the lock when the operation completes (success or failure). Use `defer`.
- `get` is read-only and does NOT acquire the lock.

---

## 9. Slash Commands

Ship the following Claude Code command files in the `commands/` directory of the memgen repository. Users copy these to their project's `.claude/commands/` directory. Commands support `$ARGUMENTS` for passing parameters directly.

### `/kg` -- Knowledge Get

**File**: `commands/kg.md`

Supports an optional scope argument via `$ARGUMENTS` (e.g., `/kg jira`, `/kg comments`).

Steps:
1. Run `git branch --show-current` to get the current branch name.
2. Call `memgen__get` with the branch name. If `$ARGUMENTS` is not empty, pass it as the `scope` argument.
3. If no knowledge exists, call `memgen__init` using a sub-agent, then call `memgen__get` again (with the same scope).
4. Present the knowledge. If there is a staleness warning, mention it.

Available scopes: (no scope) = full file, `jira`, `pr`, `git`, `comments`, `notes`.

### `/ks` -- Knowledge Set

**File**: `commands/ks.md`

Supports an optional note body via `$ARGUMENTS` (e.g., `/ks Decided to use approach B`).

Steps:
1. Run `git branch --show-current` to get the current branch name.
2. If `$ARGUMENTS` is not empty, use it directly as the note body. Call `memgen__set` with the branch name and note.
3. If `$ARGUMENTS` is empty, review the current conversation, summarize key decisions/choices/problems/TODOs, and call `memgen__set` with the summary as the note, using a sub-agent.
4. Report what was stored.

### `/kr` -- Knowledge Refresh

**File**: `commands/kr.md`

Steps:
1. Run `git branch --show-current` to get the current branch name.
2. Call `memgen__refresh` with the branch name, using a sub-agent. Wait for completion.
3. Call `memgen__get` with the same branch name.
4. Present the updated knowledge, highlighting what changed. Notes are preserved during refresh.

---

## 10. Error Handling

### Startup Errors (exit immediately with message)

| Condition | Message |
|-----------|---------|
| Config dir missing | "Created `~/.config/memgen/`. Edit `config.toml` and restart." |
| Config has placeholders | "Edit `~/.config/memgen/config.toml` with your JIRA credentials and restart." |
| `gh` CLI not found | "GitHub CLI is not installed." + install instructions for Fedora, Ubuntu, macOS, Windows. |
| `gh` CLI not authenticated | "GitHub CLI is not authenticated. Run `gh auth login` to authenticate." |
| Invalid TOML config | "Failed to parse `config.toml`: `<error>`" |

### Runtime Errors (return in MCP tool response)

| Condition | Message |
|-----------|---------|
| Missing `x-mcp-repo` header | "Missing required header `x-mcp-repo`." |
| Missing `branch` argument | "Missing required argument `branch`." |
| No ticket ID in branch | "No JIRA ticket detected in branch `<branch>`. Branch must contain a ticket ID like SV1-240." |
| JIRA ticket not found | "Could not find ticket `<ID>` in JIRA. Check: `<browse URL>` — if the ticket doesn't exist, rename your branch and try again." |
| JIRA API error | "JIRA API error: `<details>`. Check your credentials in `config.toml`." |
| JIRA auth failure (401) | "JIRA authentication failed (401): check email and API token." |
| GitHub repo not found | "GitHub repository `<repo>` not found. Check the x-mcp-repo header in your .mcp.json configuration." |
| GitHub API error | "GitHub CLI error: `<details>`. Check `gh auth status`." |
| Lock held | "An operation is already in progress for `<TICKET-ID>`. Try again later." |
| `set`/`refresh` without `init` | "No knowledge file found for `<TICKET-ID>`. Run `memgen__init` first." |
| Legacy `.md` file exists | "Knowledge file `<TICKET-ID>` has legacy .md format; please re-run init to convert to YAML." |
| Unknown scope | "Unknown scope `<scope>`: valid scopes are jira, pr, git, comments, notes." |

---

## 11. Project Structure

```
memgen/
  cmd/
    memgen/
      main.go              # Entry point, startup validation (gh CLI), config loading
  internal/
    config/
      config.go            # TOML config parsing, validation, defaults
    server/
      server.go            # HTTP/MCP server setup, tool registration with schemas, header extraction
    tools/
      tools.go             # Deps struct, shared helpers (lockKey, extractTicket, githubClient)
      init.go              # memgen__init implementation
      get.go               # memgen__get implementation (with scope filtering)
      set.go               # memgen__set implementation (note append)
      refresh.go           # memgen__refresh implementation (full re-fetch, preserve notes)
    ticket/
      detect.go            # Branch -> ticket ID extraction, JIRA browse URL helper
    sources/
      jira.go              # JIRA REST API client (ticket + comments, HTML stripping, ADF extraction)
      github.go            # gh CLI wrappers (PRs with filtering, reviews, comments, commits, GraphQL resolved status)
    knowledge/
      types.go             # YAML struct definitions (KnowledgeFile, JIRASection, PullRequest, PRComment, Note, etc.)
      convert.go           # Source-to-YAML conversion (FromSources, convertJIRA, convertPRs, convertCommits)
      store.go             # Knowledge file read/write, path resolution, scoped reads, staleness check, legacy .md detection
      lock.go              # Per-ticket non-blocking TryLock concurrency
  commands/
    kg.md                  # /kg slash command (supports scope via $ARGUMENTS)
    ks.md                  # /ks slash command (supports note body via $ARGUMENTS)
    kr.md                  # /kr slash command
  spec/
    v1-initial.md          # This specification
    v2-yaml-storage.md     # YAML storage migration spec
  testdata/                # JSON fixtures for JIRA and GitHub API mocks
  config.sample.toml       # Sample config file
  Dockerfile               # Multi-stage Docker build (Go builder + debian-slim with gh CLI)
  docker-compose.yml       # Docker Compose with volume mounts for gh/memgen config
  go.mod
  go.sum
  README.md
  CLAUDE.md                # Developer guide for AI agents
  .mcp.json                # MCP config for Claude Code
```

---

## 12. Build & Run

### Build (native)

```bash
go build -o memgen ./cmd/memgen
```

### Run (native)

```bash
./memgen
```

On first run, creates `~/.config/memgen/` and `config.toml` with sample values, then exits prompting the user to configure.

On subsequent runs with valid config, validates `gh` CLI and starts the HTTP server.

### Docker Build & Run

```bash
# Build and start
docker compose up -d --build

# View logs
docker compose logs -f memgen
```

The `docker-compose.yml` mounts:
- Host `gh` CLI auth (`~/.config/gh`) as read-only.
- Host memgen config and knowledge storage (`~/.config/memgen`).
- Runs as the host user (`UID`/`GID` environment variables).

The `Dockerfile` uses a multi-stage build:
1. **Builder stage**: `golang:latest`, builds the `memgen` binary with `CGO_ENABLED=0`.
2. **Runtime stage**: `debian:bookworm-slim` with `gh` CLI installed.

### Test

```bash
# Run all unit tests
go test ./...

# Run tests verbose
go test -v ./...

# Run integration tests
go test -tags=integration ./...

# Run a single package's tests
go test ./internal/tools/...
```

### Prerequisites

- Go 1.25+ (for building from source).
- `gh` CLI (installed and authenticated via `gh auth login`).
- JIRA Cloud account with API token.

---

## 13. Implementation Constraints

- **Language**: Go. Single binary. No external runtime dependencies beyond the `gh` CLI.
- **Database**: Filesystem `.yaml` files only. No SQLite, no Postgres, no Redis.
- **Storage format**: YAML via `gopkg.in/yaml.v3`. Structured Go types marshaled/unmarshaled directly.
- **OS**: Linux only. Hardcode `~/.config/memgen/` path.
- **Auth**: None. Do not implement authentication.
- **TLS**: None. HTTP only.
- **No LLM dependency**: Knowledge files are assembled by Go code, not by an LLM. The `claude` CLI is not used at runtime.
- **Timestamps**: UTC only, everywhere. When consuming JIRA/GitHub timestamps, convert to UTC. When storing, store UTC. When displaying, display UTC.
- **Config format**: TOML only (`github.com/BurntSushi/toml`).
- **Go MCP library**: `github.com/mark3labs/mcp-go` for MCP protocol handling (streamable HTTP transport). Do not hand-roll the MCP protocol.
- **Container support**: Dockerfile and docker-compose.yml for containerized deployment.

---

## 14. Testing Requirements

**Test coverage is mandatory.** Every package must have meaningful test coverage. Tests are not optional or deferred -- they are part of the implementation. A feature is not complete until its tests pass.

### Testing Strategy

#### Unit Tests

Every package in `internal/` must have corresponding `_test.go` files.

| Package | What to Test |
|---------|-------------|
| `config` | TOML parsing: valid config, missing fields, placeholder detection, malformed TOML, missing file, directory creation logic. |
| `ticket` | Branch-to-ticket extraction: standard patterns, bare ticket, no match, multiple matches (first wins), edge cases (lowercase, special chars). Browse URL generation. |
| `knowledge/store` | YAML file read/write, path resolution from repo+ticket, missing directory creation, file-not-found behavior, scoped section reads (jira, pr, git, comments, notes), staleness check, legacy `.md` detection, nil slice initialization after unmarshal. |
| `knowledge/lock` | Lock acquisition, non-blocking tryLock returns immediately when held, lock release, concurrent access from multiple goroutines. |
| `knowledge/types` | YAML marshal/unmarshal round-trip for all struct types. Verify field tags produce expected YAML keys. Empty/nil slice handling. |
| `knowledge/convert` | `FromSources()` with full data, nil JIRA, empty PR slices, nil commit slices. Verify all fields map correctly from source types to knowledge types. |
| `sources/jira` | JIRA API response parsing: ticket details, comments, empty comments, HTML stripping, ADF text extraction. Error cases: auth failure (401), not found (404), network error, malformed JSON. Use `httptest.Server` to mock JIRA API responses. |
| `sources/github` | `gh` CLI command construction: correct arguments for PR search, commit search, review comments. Parse `gh` JSON output. PR filtering by ticket ID (`filterPREntriesByTicket`). Commit filtering by ticket ID. GraphQL resolved status fetch with fallback. Test with mock command execution (inject a test executor). |
| `tools/init` | Full init flow with mocked sources. Verify: lock acquired, sources fetched, YAML file written with correct struct fields, lock released. Error propagation when JIRA fails, when GitHub fails. |
| `tools/get` | File exists: returns YAML content + staleness info. File missing: returns init recommendation. Scope filtering: each scope returns correct section. |
| `tools/set` | File exists: reads YAML, appends note, writes back. File missing: returns error. Lock contention: returns "try again later." Verify note has UTC timestamp. |
| `tools/refresh` | Full re-fetch with mocked sources. Notes preserved after refresh. File missing case returns error. |
| `server` | MCP tool registration: verify all 4 tools registered with correct schemas. Tool dispatch: valid calls, missing headers, missing arguments. Scope parameter handling for get. Note/decisions backward compatibility for set. |

#### Integration Tests

Place in `internal/integration/` or as build-tagged files (`//go:build integration`).

| Test | What It Covers |
|------|---------------|
| Config round-trip | Write TOML, load it, verify all fields populated. |
| Knowledge file lifecycle | `init` -> `get` -> `set` -> `get` -> `refresh` -> `get` full cycle using mocked external sources but real file I/O. Verify YAML structure at each step. |
| Concurrent lock behavior | Multiple goroutines attempting `init` and `set` on the same ticket simultaneously -- verify only one proceeds, others get "try again later." |
| HTTP server end-to-end | Start real HTTP server, send MCP requests with headers and arguments, verify correct tool dispatch and response format. |

#### What to Mock

- **JIRA API**: Use `httptest.NewServer` to return canned JSON responses. Never call real JIRA in tests.
- **`gh` CLI**: Inject a `sources.CommandExecutor` interface. In tests, provide a mock that returns canned stdout/stderr. Never call real `gh` in tests.
- **Filesystem**: Unit tests for knowledge store use `os.MkdirTemp` for isolated temporary directories. Clean up with `t.Cleanup`.

#### Test Conventions

- Use table-driven tests where multiple inputs map to expected outputs.
- Use `t.Parallel()` where tests are independent.
- Use `testdata/` directories for fixture files (sample JIRA responses, GitHub API responses, etc.).
- Test error messages -- verify they contain the expected actionable information (ticket ID, URLs, instructions).
- Verify YAML struct fields directly (not string matching) where possible.
- No test should depend on network access, installed CLIs, or external state.
- Run all tests with `go test ./...`. Integration tests gated behind `//go:build integration` tag, run with `go test -tags=integration ./...`.

---

## 15. Non-Goals (Explicitly Out of Scope)

- Authentication / authorization
- TLS termination
- Multi-user support
- Web UI / dashboard
- Systemd service management
- Windows / macOS server support
- Rate limiting
- Metrics / observability
- Data encryption at rest
- LLM-based summarization (knowledge is structured data, not LLM output)
