Retrieve knowledge for the current ticket.

1. Run `git branch --show-current` to get the current branch name.
2. Call the `memgen__get` MCP tool with the branch name as the `branch` argument.
3. If the response says no knowledge exists, immediately call `memgen__init` with the same branch name using a sub-agent. Wait for the sub-agent to complete.
4. After init completes, call `memgen__get` again and present the knowledge.
5. If knowledge exists, present it to the user. If there is a staleness warning, mention it.
