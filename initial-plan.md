# MemGen - Technical Specification

## Build Prompt for Claude Code Agent

You are building **MemGen**, a networked MCP (Model Context Protocol) server written in Go that provides distributed knowledge management for AI coding agents. Multiple agents running on different machines connect to a single MemGen instance over HTTP to share and retrieve contextual knowledge tied to JIRA tickets.

---

## 1. Overview

### Problem

Multiple AI coding agents work interchangeably on tasks tied to JIRA tickets across different machines. Each agent lacks awareness of prior context — JIRA details, PR discussions, past decisions, outstanding review comments. There is no shared memory between sessions or machines.

### Solution

A single Go binary (`memgen`) that runs as an HTTP-based MCP server on a Linux machine in the local network. Agents connect via MCP-over-HTTP, pass their current git branch, and MemGen extracts the JIRA ticket ID, manages knowledge files, and provides tools to initialize, retrieve, update, and refresh knowledge.

Knowledge is assembled and maintained by invoking the `claude` CLI (Opus 4.6, high effort) to summarize, merge, and deduplicate data from JIRA, GitHub PRs, and agent-provided decisions.

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

1. **`claude` CLI**: Must be installed and authenticated. Invoke a test prompt (e.g., `claude --print "ping"`) to verify. If not authenticated, exit with error instructing the user to run `claude` and authenticate.

2. **`gh` CLI**: Must be installed and authenticated. Invoke `gh auth status` to verify. If not installed or not authenticated, exit with:
   - Installation instructions for Fedora 43 (primary), plus macOS, Windows, Ubuntu, and other Linux distros.
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

**Knowledge storage**: Organized by repository, then by ticket ID.

```
~/.config/memgen/
  config.toml
  knowledge/
    stitch-ai/
      stitch-mono/
        SV1-240.md
        SV1-241.md
        SBUX-111.md
    other-org/
      other-repo/
        PROJ-100.md
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

Extract the JIRA ticket ID from the branch name using the **first match** of the pattern `[A-Z]+-\d+`.

Examples:
- `SV1-240-mail-threading` -> `SV1-240`
- `SBUX-111-some-task` -> `SBUX-111`
- `SAI-342-some-task` -> `SAI-342`
- `SV1-240` -> `SV1-240` (bare ticket, no suffix)

### Validation

After extracting the ticket ID, validate it exists in JIRA by calling the JIRA REST API. If the API returns 404 or any error:

- Return an error message that includes:
  - The detected ticket ID
  - A clickable JIRA URL: `https://stitchai.atlassian.net/browse/<TICKET-ID>`
  - A message: "Could not find this ticket in JIRA. Check the link above — if the ticket doesn't exist, update/rename your branch and try again."

If no ticket pattern is found in the branch name, return an error: "No JIRA ticket detected in branch name `<branch>`. Branch must contain a JIRA ticket ID (e.g., SV1-240-description)."

---

## 4. MCP Tools

All tools require `branch` as a **required argument**. All tools extract the repo from the `x-mcp-repo` header. If either is missing, the tool MUST return an error.

### 4.1 `memgen__init`

**Purpose**: Gather knowledge from all external sources and create the initial knowledge file.

**Concurrency**: Acquire an in-process lock keyed by `<repo>/<ticket-id>`. If the lock is already held (another `init` or `set` is running for this ticket), return immediately: "Operation already in progress for `<TICKET-ID>`. Try again later."

**Steps**:

1. Extract ticket ID from `branch` argument.
2. Validate ticket exists in JIRA (see Section 3).
3. Gather data from all sources (see Section 5) into a temporary file.
4. Invoke `claude` CLI to process all gathered raw data into a structured knowledge summary (see Section 6 for knowledge file format).
5. Write the result to `~/.config/memgen/knowledge/<owner>/<repo>/<TICKET-ID>.md`.
6. Return success with a summary of what was gathered (e.g., "Initialized knowledge for SV1-240: JIRA ticket + 3 PRs + 12 commits on main").

**If the knowledge file already exists**: Overwrite it. `init` is a full re-initialization.

### 4.2 `memgen__get`

**Purpose**: Retrieve stored knowledge for the current branch's ticket.

**Steps**:

1. Extract ticket ID from `branch` argument.
2. Look up `~/.config/memgen/knowledge/<owner>/<repo>/<TICKET-ID>.md`.
3. If the file exists:
   - Read and return its contents.
   - Check the file's last-refresh timestamp. If older than 24 hours (configurable), append an informational note: "Knowledge was last refreshed on `<UTC timestamp>` (`<N>` days ago). Consider running `/kr` to refresh."
   - Never block or fail based on staleness — information only.
4. If the file does not exist:
   - Return HTTP 200 with message: "No knowledge found for `<TICKET-ID>`. Run `memgen__init` to initialize knowledge for this ticket."
   - The calling agent is expected to follow this recommendation.

### 4.3 `memgen__set`

**Purpose**: Store key decisions and context from the current agent session into the knowledge file.

**Concurrency**: Same lock mechanism as `init`. If locked, return "try again later."

**Required**: Knowledge file must already exist. If it does not exist, return an error: "No knowledge file found for `<TICKET-ID>`. Run `memgen__init` first."

**Steps**:

1. Extract ticket ID from `branch` argument.
2. Verify knowledge file exists.
3. Acquire lock for `<repo>/<ticket-id>`.
4. Read existing knowledge file.
5. Invoke `claude` CLI with:
   - The existing knowledge file content (old knowledge)
   - The new decisions/context provided by the agent (new knowledge)
   - Instructions to merge: new knowledge/decisions overwrite older conflicting decisions, deduplicate, and preserve everything that is still relevant.
   - If the new knowledge is identical or already covered, make no changes.
6. Timestamp every new decision entry with the current UTC time.
7. Write the merged result back to the knowledge file.
8. Return success with a summary of what changed (or "no changes needed" if deduplicated).

### 4.4 `memgen__refresh`

**Purpose**: Refresh knowledge by fetching only new data since the last refresh.

**Concurrency**: Same lock mechanism. If locked, return "try again later."

**Required**: Knowledge file must already exist. If not, return error telling agent to run `init`.

**Steps**:

1. Extract ticket ID from `branch` argument.
2. Read existing knowledge file, extract the last-refresh timestamp.
3. Fetch only new data from sources **after** the last-refresh timestamp:
   - JIRA: comments created/updated after timestamp.
   - GitHub: PR comments, reviews, and commits after timestamp.
   - Main branch commits referencing the ticket after timestamp.
4. If no new data found, update the refresh timestamp and return "Knowledge is up to date."
5. If new data found, invoke `claude` CLI to merge new data into existing knowledge (same merge logic as `set`).
6. Update the refresh timestamp.
7. Return summary of what was added.

---

## 5. Data Sources

All external data fetching uses the `gh` CLI for GitHub and the JIRA REST API (v3, Cloud) for JIRA.

### 5.1 JIRA

- **API**: `https://<jira-url>/rest/api/3/issue/<TICKET-ID>?expand=renderedFields`
- **Auth**: Basic auth with email + API token from `config.toml`.
- **Fetch**:
  - Ticket summary, description, status, assignee, reporter, priority, labels, sprint.
  - All comments (with authors and timestamps).

### 5.2 GitHub PRs

- **Tool**: `gh pr list --repo <repo> --search "<TICKET-ID>" --state all --json number,title,body,state,author,createdAt,updatedAt,headRefName,url`
- **For each PR found**:
  - PR summary (title, body, state, author).
  - All review comments and their replies: `gh api repos/<owner>/<repo>/pulls/<number>/comments`
  - All review threads: resolved vs unresolved status.
  - All PR reviews (approved, changes requested, commented): `gh api repos/<owner>/<repo>/pulls/<number>/reviews`
  - Change request details — who requested what, and whether it was resolved.
  - Commit messages on the PR.
- **Include all PRs**: open, closed, merged. No filtering.

### 5.3 GitHub Commits on Main

- **Tool**: `gh api repos/<owner>/<repo>/commits --paginate -q '.[] | select(.commit.message | test("<TICKET-ID>"))'` or equivalent search.
- **Purpose**: Understand what has already been merged to main for this ticket.
- **Fetch**: Commit hash, message, author, date.

### 5.4 Existing Knowledge File

- If a `.md` file already exists for this ticket during `init`, it is read but **overwritten** (init is a full reset).
- During `refresh` and `set`, the existing file is read and merged with new data.

---

## 6. Knowledge File Format

Each knowledge file is a Markdown document assembled and maintained by the `claude` CLI. The file MUST include these sections. Claude CLI is instructed to maintain this structure during all summarization and merge operations.

```markdown
# <TICKET-ID>: <Ticket Summary>

**Last Refreshed**: <UTC timestamp>
**Branch**: <branch name>
**Status**: <JIRA status>

## JIRA Ticket

<Summarized ticket description, key details, acceptance criteria, etc.>

### JIRA Comments

<Chronological summary of JIRA comments with authors and timestamps (UTC).>

## Pull Requests

### PR #<number>: <title>
- **State**: <open/closed/merged>
- **Author**: <author>
- **URL**: <url>

<PR description summary>

#### Review Comments & Change Requests

<Structured summary of review threads:
- Who requested what change
- Whether it was resolved or still outstanding
- Key discussion points and decisions
- Replies and follow-ups>

#### Commits

<List of commits with messages>

(Repeat for each PR)

## Commits on Main

<Commits already merged to main that reference this ticket.>

## Decisions

<Agent-provided decisions and context from `memgen__set` calls.
Each entry timestamped in UTC.>

### <UTC timestamp>
<Decision content>

### <UTC timestamp>
<Decision content>
```

The exact formatting within sections is delegated to the `claude` CLI — it should produce clean, readable Markdown that agents can easily parse and act on. The section structure above is the required skeleton.

---

## 7. Claude CLI Integration

### Invocation

All Claude CLI calls use:

```bash
claude --model claude-opus-4-6 --verbose --print --output-format text
```

Input is provided via stdin (pipe the raw data or existing knowledge + new data). The prompt instructs Claude what to produce.

### Use Cases

**A. Init — Full Knowledge Assembly**

Pipe all raw gathered data (JIRA JSON, PR data, commit logs) to Claude with a system prompt instructing it to:
- Parse and summarize all data into the knowledge file format (Section 6).
- Maintain chronological order within sections.
- Highlight unresolved PR review comments and outstanding change requests.
- Convert all timestamps to UTC.

**B. Set — Merge Decisions**

Pipe the existing knowledge file + new decisions to Claude with a system prompt instructing it to:
- Merge new decisions into the Decisions section.
- If new decisions conflict with or supersede older ones, keep the new decision and mark or remove the old one.
- If the new content is already present (duplicate), make no changes and report "no changes needed."
- Timestamp each new decision with current UTC time.
- Preserve all other sections unchanged.

**C. Refresh — Incremental Merge**

Pipe the existing knowledge file + new raw data (only data after last refresh) to Claude with a system prompt instructing it to:
- Update relevant sections with new data (new JIRA comments, new PR comments, new commits).
- Highlight newly unresolved review comments.
- Preserve existing Decisions section untouched.
- Update the Last Refreshed timestamp.

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

Ship the following Claude Code command files in the `commands/` directory of the memgen repository. Users copy these to their project's `.claude/commands/` directory.

### `/kg` — Knowledge Get

**File**: `commands/kg.md`

```markdown
Retrieve knowledge for the current ticket.

1. Run `git branch --show-current` to get the current branch name.
2. Call the `memgen__get` MCP tool with the branch name.
3. If the response says no knowledge exists, immediately call `memgen__init` with the same branch name using a sub-agent.
4. After init completes, call `memgen__get` again and present the knowledge.
5. If knowledge exists, present it to the user.
```

### `/ks` — Knowledge Set

**File**: `commands/ks.md`

```markdown
Summarize and store key decisions from this session.

1. Run `git branch --show-current` to get the current branch name.
2. Review the current conversation and summarize:
   - Key decisions made
   - Implementation choices and their rationale
   - Problems encountered and how they were resolved
   - Any open questions or TODOs
3. Call the `memgen__set` MCP tool with the branch name and the summary, using a sub-agent.
4. Report what was stored.
```

### `/kr` — Knowledge Refresh

**File**: `commands/kr.md`

```markdown
Refresh knowledge with latest data from JIRA and GitHub.

1. Run `git branch --show-current` to get the current branch name.
2. Call the `memgen__refresh` MCP tool with the branch name, using a sub-agent.
3. After refresh completes, call `memgen__get` with the same branch name.
4. Present the updated knowledge, highlighting what changed since last refresh.
```

---

## 10. Error Handling

### Startup Errors (exit immediately with message)

| Condition | Message |
|-----------|---------|
| Config dir missing | "Created `~/.config/memgen/`. Edit `config.toml` and restart." |
| Config has placeholders | "Edit `~/.config/memgen/config.toml` with your JIRA credentials and restart." |
| `claude` CLI not found | "Claude CLI is not installed. Install it and run `claude` to authenticate." |
| `claude` CLI not authenticated | "Claude CLI is not authenticated. Run `claude` and complete authentication." |
| `gh` CLI not found | "GitHub CLI is not installed." + install instructions for Fedora 43, Ubuntu, macOS, Windows. |
| `gh` CLI not authenticated | "GitHub CLI is not authenticated. Run `gh auth login` to authenticate." |
| Invalid TOML config | "Failed to parse `config.toml`: `<error>`" |

### Runtime Errors (return in MCP tool response)

| Condition | Message |
|-----------|---------|
| Missing `x-mcp-repo` header | "Missing required header `x-mcp-repo`." |
| Missing `branch` argument | "Missing required argument `branch`." |
| No ticket ID in branch | "No JIRA ticket detected in branch `<branch>`. Branch must contain a ticket ID like SV1-240." |
| JIRA ticket not found | "Could not find ticket `<ID>` in JIRA. Check: `https://stitchai.atlassian.net/browse/<ID>` — if the ticket doesn't exist, rename your branch and try again." |
| JIRA API error | "JIRA API error: `<details>`. Check your credentials in `config.toml`." |
| GitHub API error | "GitHub CLI error: `<details>`. Check `gh auth status`." |
| Lock held | "An operation is already in progress for `<TICKET-ID>`. Try again later." |
| `set`/`refresh` without `init` | "No knowledge file found for `<TICKET-ID>`. Run `memgen__init` first." |
| Claude CLI failure | "Claude CLI failed during summarization: `<stderr output>`. Check that `claude` is working." |

---

## 11. Project Structure

```
memgen/
  cmd/
    memgen/
      main.go              # Entry point, startup validation, config loading
  internal/
    config/
      config.go            # TOML config parsing, validation, defaults
    server/
      server.go            # HTTP server, MCP protocol handling
      middleware.go         # Header extraction (x-mcp-repo)
    tools/
      init.go              # memgen__init implementation
      get.go               # memgen__get implementation
      set.go               # memgen__set implementation
      refresh.go           # memgen__refresh implementation
    ticket/
      detect.go            # Branch -> ticket ID extraction & JIRA validation
    sources/
      jira.go              # JIRA REST API client
      github.go            # gh CLI wrappers for PRs, commits, reviews
    knowledge/
      store.go             # Knowledge file read/write, path resolution
      lock.go              # Per-ticket concurrency lock
    claude/
      cli.go               # Claude CLI invocation (init, merge, refresh prompts)
  commands/
    kg.md                  # /kg slash command
    ks.md                  # /ks slash command
    kr.md                  # /kr slash command
  config.sample.toml       # Sample config file
  go.mod
  go.sum
  README.md
  .mcp.json.example        # Example MCP config for Claude Code
```

---

## 12. Build & Run

### Build

```bash
go build -o memgen ./cmd/memgen
```

### Run

```bash
./memgen
```

On first run, creates `~/.config/memgen/` and `config.toml` with sample values, then exits prompting the user to configure.

On subsequent runs with valid config, validates dependencies and starts the HTTP server.

### README

The README must include:
- What MemGen is and why it exists (1-2 paragraphs).
- Prerequisites: Go 1.22+, `claude` CLI (authenticated), `gh` CLI (authenticated).
- Build instructions.
- Configuration guide (TOML fields explained).
- How to connect from Claude Code (`.mcp.json` example).
- How to install slash commands (copy `commands/*.md` to `.claude/commands/`).
- Usage examples for each tool.

---

## 13. Implementation Constraints

- **Language**: Go. Single binary. No external runtime dependencies beyond `claude` and `gh` CLIs.
- **Database**: Filesystem `.md` files only. No SQLite, no Postgres, no Redis.
- **OS**: Linux only. Hardcode `~/.config/memgen/` path.
- **Auth**: None. Do not implement authentication.
- **TLS**: None. HTTP only.
- **Model**: All Claude CLI calls use `claude-opus-4-6` with high effort.
- **Timestamps**: UTC only, everywhere. When consuming JIRA/GitHub timestamps, convert to UTC. When storing, store UTC. When displaying, display UTC.
- **Config format**: TOML only.
- **Go MCP library**: Use the official `github.com/mark3labs/mcp-go` library (or equivalent well-maintained Go MCP library) for MCP protocol handling. Do not hand-roll the MCP protocol.

---

## 14. Non-Goals (Explicitly Out of Scope)

- Authentication / authorization
- TLS termination
- Multi-user support
- Web UI / dashboard
- Persistent process management (systemd, Docker) — user runs the binary directly
- Windows / macOS server support
- Rate limiting
- Metrics / observability
- Data encryption at rest
