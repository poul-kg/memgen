package claude

import (
	"bytes"
	"fmt"
	"os/exec"
)

// CommandExecutor abstracts command execution for testing.
type CommandExecutor interface {
	ExecuteWithStdin(stdin string, name string, args ...string) (stdout string, stderr string, err error)
}

// DefaultExecutor runs real commands.
type DefaultExecutor struct{}

func (e *DefaultExecutor) ExecuteWithStdin(stdin string, name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewBufferString(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// CLI wraps the claude command-line tool.
type CLI struct {
	Executor CommandExecutor
}

func NewCLI() *CLI {
	return &CLI{Executor: &DefaultExecutor{}}
}

// baseArgs returns the base arguments for all claude invocations.
func (c *CLI) baseArgs() []string {
	return []string{"--model", "claude-opus-4-6", "--print", "--output-format", "text"}
}

// InitKnowledge takes raw gathered data and produces a structured knowledge file.
// rawData contains all JIRA, PR, commit data concatenated.
// branch is the git branch name.
// ticketID is the extracted JIRA ticket ID.
func (c *CLI) InitKnowledge(rawData, branch, ticketID string) (string, error) {
	prompt := fmt.Sprintf(`You are assembling a knowledge file for JIRA ticket %s on branch %s.

Below is all the raw data gathered from JIRA, GitHub PRs, and commits. Parse and summarize it into the following Markdown structure. Maintain chronological order within sections. Highlight unresolved PR review comments and outstanding change requests. Convert all timestamps to UTC.

Required structure:
# {TICKET-ID}: {Ticket Summary}

**Last Refreshed**: {current UTC timestamp in RFC3339}
**Branch**: {branch}
**Status**: {JIRA status}

## JIRA Ticket
{summarized description, acceptance criteria}

### JIRA Comments
{chronological comments with authors and UTC timestamps}

## Pull Requests
### PR #{number}: {title}
- **State**: {state}
- **Author**: {author}
- **URL**: {url}

{PR description summary}

#### Review Comments & Change Requests
{structured summary: who requested what, resolved vs outstanding, key discussions}

#### Commits
{list of commits}

## Commits on Main
{commits merged to main referencing this ticket}

## Decisions
{empty for init}

---
RAW DATA:
%s`, ticketID, branch, rawData)

	args := c.baseArgs()
	stdout, stderr, err := c.Executor.ExecuteWithStdin(prompt, "claude", args...)
	if err != nil {
		return "", fmt.Errorf("claude init-knowledge failed: %w\nstderr: %s", err, stderr)
	}
	return stdout, nil
}

// MergeDecisions takes existing knowledge and new decisions, returns merged result.
// Returns the exact string "no changes needed" if the new content is already covered.
func (c *CLI) MergeDecisions(existingKnowledge, newDecisions, currentUTCTime string) (string, error) {
	prompt := fmt.Sprintf(`You are updating a knowledge file. Below is the existing knowledge file and new decisions from an agent session.

Rules:
- Merge new decisions into the Decisions section
- New decisions that conflict with or supersede older ones should replace them
- If the new content is already covered (duplicate), output EXACTLY "no changes needed" and nothing else
- Timestamp each new decision with: %s
- Preserve all other sections UNCHANGED
- Output the complete updated knowledge file

EXISTING KNOWLEDGE:
%s

NEW DECISIONS:
%s`, currentUTCTime, existingKnowledge, newDecisions)

	args := c.baseArgs()
	stdout, stderr, err := c.Executor.ExecuteWithStdin(prompt, "claude", args...)
	if err != nil {
		return "", fmt.Errorf("claude merge-decisions failed: %w\nstderr: %s", err, stderr)
	}
	return stdout, nil
}

// RefreshKnowledge takes existing knowledge and new raw data (since last refresh), returns updated knowledge.
func (c *CLI) RefreshKnowledge(existingKnowledge, newRawData, currentUTCTime string) (string, error) {
	prompt := fmt.Sprintf(`You are refreshing a knowledge file with new data. Below is the existing knowledge and new data fetched since the last refresh.

Rules:
- Update relevant sections with new data (new JIRA comments, PR comments, commits)
- Highlight newly unresolved review comments
- Preserve the Decisions section UNTOUCHED
- Update "Last Refreshed" to: %s
- Output the complete updated knowledge file

EXISTING KNOWLEDGE:
%s

NEW DATA:
%s`, currentUTCTime, existingKnowledge, newRawData)

	args := c.baseArgs()
	stdout, stderr, err := c.Executor.ExecuteWithStdin(prompt, "claude", args...)
	if err != nil {
		return "", fmt.Errorf("claude refresh-knowledge failed: %w\nstderr: %s", err, stderr)
	}
	return stdout, nil
}

// CheckAvailable verifies the claude CLI is installed and authenticated.
// Returns nil if OK, descriptive error otherwise.
func (c *CLI) CheckAvailable() error {
	// Check that claude is installed by running --version.
	_, stderr, err := c.Executor.ExecuteWithStdin("", "claude", "--version")
	if err != nil {
		return fmt.Errorf("claude CLI not found or not executable: %w\nstderr: %s", err, stderr)
	}

	// Check that claude is authenticated and working.
	args := append(c.baseArgs(), "--print", "respond with OK")
	// baseArgs already includes --print, but the second --print is harmless;
	// however, the prompt should go via stdin. Let's use stdin instead.
	args = c.baseArgs()
	stdout, stderr, err := c.Executor.ExecuteWithStdin("respond with OK", "claude", args...)
	if err != nil {
		return fmt.Errorf("claude CLI not authenticated or not working: %w\nstderr: %s", err, stderr)
	}
	_ = stdout
	return nil
}
