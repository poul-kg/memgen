Store a note for the current ticket.

1. Run `git branch --show-current` to get the current branch name.
2. If `$ARGUMENTS` is not empty, use it directly as the note body. Call the `memgen__set` MCP tool with the branch name as the `branch` argument and `$ARGUMENTS` as the `note` argument. Report what was stored.
3. If `$ARGUMENTS` is empty, review the current conversation and summarize:
   - Key decisions made
   - Implementation choices and their rationale
   - Problems encountered and how they were resolved
   - Any open questions or TODOs
   Then call the `memgen__set` MCP tool with the branch name as the `branch` argument and the summary as the `note` argument, using a sub-agent. Wait for the sub-agent to complete.
4. Report what was stored.
