Refresh knowledge with latest data from JIRA and GitHub. Notes are preserved.

1. Run `git branch --show-current` to get the current branch name.
2. Call the `memgen__refresh` MCP tool with the branch name as the `branch` argument, using a sub-agent. Wait for the sub-agent to complete.
3. After refresh completes, call `memgen__get` with the same branch name.
4. Present the updated knowledge, highlighting what changed since last refresh.
