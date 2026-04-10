Retrieve knowledge for the current ticket.

1. Run `git branch --show-current` to get the current branch name.
2. Call the `memgen__get` MCP tool with the branch name as the `branch` argument. If `$ARGUMENTS` is not empty, pass it as the `scope` argument.
3. If the response says no knowledge exists, immediately call `memgen__init` with the same branch name using a sub-agent. Wait for the sub-agent to complete.
4. After init completes, call `memgen__get` again (with the same scope if provided) and present the knowledge.
5. If knowledge exists, present it to the user. If there is a staleness warning, mention it.

## Available Scopes

| Scope | What it returns |
|-------|-----------------|
| (no scope) | Full knowledge file — JIRA ticket, all PRs with reviews/comments/commits, main branch commits, and notes |
| `jira` | JIRA ticket only — summary, description, status, priority, assignee, reporter, labels, and all JIRA comments |
| `pr` | PR summaries only — number, title, state, author, URL, and description body for each PR (no reviews/comments/commits) |
| `git` | Full PR data + main commits — all PRs with reviews, comments, commits, plus commits on main branch |
| `comments` | PR review comments and reviews only — all review comments (with resolved status) and review decisions across all PRs |
| `notes` | Notes only — custom notes added via `/ks` |

## Usage Examples

- `/kg` — get everything
- `/kg jira` — just the JIRA ticket details
- `/kg pr` — quick overview of related PRs
- `/kg git` — full git context (PRs + main commits)
- `/kg comments` — see what reviewers said and what's resolved
- `/kg notes` — see team decisions and notes
