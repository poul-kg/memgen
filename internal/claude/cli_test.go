package claude

import (
	"errors"
	"strings"
	"testing"
)

// MockCall records a single invocation of ExecuteWithStdin.
type MockCall struct {
	Stdin string
	Name  string
	Args  []string
}

// MockExecutor captures calls and returns canned responses.
type MockExecutor struct {
	Calls    []MockCall
	Response string
	Stderr   string
	Err      error
	// CallResponses allows per-call responses when set (indexed by call order).
	CallResponses []MockResponse
}

// MockResponse holds a per-call canned response.
type MockResponse struct {
	Response string
	Stderr   string
	Err      error
}

func (m *MockExecutor) ExecuteWithStdin(stdin string, name string, args ...string) (string, string, error) {
	idx := len(m.Calls)
	m.Calls = append(m.Calls, MockCall{Stdin: stdin, Name: name, Args: args})
	if idx < len(m.CallResponses) {
		r := m.CallResponses[idx]
		return r.Response, r.Stderr, r.Err
	}
	return m.Response, m.Stderr, m.Err
}

func TestInitKnowledge_CommandArgs(t *testing.T) {
	mock := &MockExecutor{Response: "# TICKET-123: Summary"}
	cli := &CLI{Executor: mock}

	result, err := cli.InitKnowledge("raw data here", "feature/branch", "TICKET-123")
	if err != nil {
		t.Fatalf("InitKnowledge returned error: %v", err)
	}
	if result != "# TICKET-123: Summary" {
		t.Errorf("InitKnowledge result = %q, want %q", result, "# TICKET-123: Summary")
	}

	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}

	call := mock.Calls[0]
	if call.Name != "claude" {
		t.Errorf("command name = %q, want %q", call.Name, "claude")
	}

	argsStr := strings.Join(call.Args, " ")
	for _, want := range []string{"--model", "claude-opus-4-6", "--print", "--output-format", "text"} {
		if !strings.Contains(argsStr, want) {
			t.Errorf("args %q missing expected flag %q", argsStr, want)
		}
	}
}

func TestInitKnowledge_StdinContent(t *testing.T) {
	mock := &MockExecutor{Response: "output"}
	cli := &CLI{Executor: mock}

	_, err := cli.InitKnowledge("raw data payload", "feat/my-branch", "PROJ-456")
	if err != nil {
		t.Fatalf("InitKnowledge returned error: %v", err)
	}

	stdin := mock.Calls[0].Stdin

	// Verify stdin contains raw data, branch, and ticket ID.
	for _, want := range []string{"raw data payload", "feat/my-branch", "PROJ-456"} {
		if !strings.Contains(stdin, want) {
			t.Errorf("stdin missing %q", want)
		}
	}

	// Verify required structural elements from the template.
	requiredPhrases := []string{
		"Maintain chronological order",
		"Highlight unresolved PR review comments and outstanding change requests",
		"Convert all timestamps to UTC",
		"## JIRA Ticket",
		"### JIRA Comments",
		"## Pull Requests",
		"#### Review Comments & Change Requests",
		"#### Commits",
		"## Commits on Main",
		"## Decisions",
		"RAW DATA:",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(stdin, phrase) {
			t.Errorf("stdin missing required phrase %q", phrase)
		}
	}
}

func TestInitKnowledge_Error(t *testing.T) {
	mock := &MockExecutor{
		Err:    errors.New("exit status 1"),
		Stderr: "something went wrong",
	}
	cli := &CLI{Executor: mock}

	_, err := cli.InitKnowledge("data", "branch", "TICKET-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "claude init-knowledge failed") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "claude init-knowledge failed")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error = %q, want it to contain stderr", err.Error())
	}
}

func TestMergeDecisions_StdinContent(t *testing.T) {
	mock := &MockExecutor{Response: "merged output"}
	cli := &CLI{Executor: mock}

	result, err := cli.MergeDecisions("existing knowledge", "new decisions", "2026-04-08T12:00:00Z")
	if err != nil {
		t.Fatalf("MergeDecisions returned error: %v", err)
	}
	if result != "merged output" {
		t.Errorf("MergeDecisions result = %q, want %q", result, "merged output")
	}

	stdin := mock.Calls[0].Stdin

	// Verify stdin contains existing knowledge and new decisions.
	for _, want := range []string{"existing knowledge", "new decisions", "2026-04-08T12:00:00Z"} {
		if !strings.Contains(stdin, want) {
			t.Errorf("stdin missing %q", want)
		}
	}

	// Verify required instructions.
	requiredPhrases := []string{
		"Merge new decisions into the Decisions section",
		"conflict with or supersede older ones should replace them",
		`output EXACTLY "no changes needed"`,
		"Preserve all other sections UNCHANGED",
		"Output the complete updated knowledge file",
		"EXISTING KNOWLEDGE:",
		"NEW DECISIONS:",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(stdin, phrase) {
			t.Errorf("stdin missing required phrase %q", phrase)
		}
	}
}

func TestMergeDecisions_NoChangesNeeded(t *testing.T) {
	mock := &MockExecutor{Response: "no changes needed"}
	cli := &CLI{Executor: mock}

	result, err := cli.MergeDecisions("existing", "duplicate stuff", "2026-04-08T12:00:00Z")
	if err != nil {
		t.Fatalf("MergeDecisions returned error: %v", err)
	}
	if result != "no changes needed" {
		t.Errorf("MergeDecisions result = %q, want %q", result, "no changes needed")
	}
}

func TestMergeDecisions_CommandArgs(t *testing.T) {
	mock := &MockExecutor{Response: "output"}
	cli := &CLI{Executor: mock}

	_, err := cli.MergeDecisions("existing", "new", "2026-04-08T12:00:00Z")
	if err != nil {
		t.Fatalf("MergeDecisions returned error: %v", err)
	}

	call := mock.Calls[0]
	argsStr := strings.Join(call.Args, " ")
	for _, want := range []string{"--model", "claude-opus-4-6", "--print", "--output-format", "text"} {
		if !strings.Contains(argsStr, want) {
			t.Errorf("args %q missing expected flag %q", argsStr, want)
		}
	}
}

func TestRefreshKnowledge_StdinContent(t *testing.T) {
	mock := &MockExecutor{Response: "refreshed output"}
	cli := &CLI{Executor: mock}

	result, err := cli.RefreshKnowledge("existing knowledge", "new raw data", "2026-04-08T14:00:00Z")
	if err != nil {
		t.Fatalf("RefreshKnowledge returned error: %v", err)
	}
	if result != "refreshed output" {
		t.Errorf("RefreshKnowledge result = %q, want %q", result, "refreshed output")
	}

	stdin := mock.Calls[0].Stdin

	// Verify stdin contains existing knowledge, new data, and current time.
	for _, want := range []string{"existing knowledge", "new raw data", "2026-04-08T14:00:00Z"} {
		if !strings.Contains(stdin, want) {
			t.Errorf("stdin missing %q", want)
		}
	}

	// Verify required instructions.
	requiredPhrases := []string{
		"Update relevant sections with new data",
		"Highlight newly unresolved review comments",
		"Preserve the Decisions section UNTOUCHED",
		`Update "Last Refreshed"`,
		"Output the complete updated knowledge file",
		"EXISTING KNOWLEDGE:",
		"NEW DATA:",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(stdin, phrase) {
			t.Errorf("stdin missing required phrase %q", phrase)
		}
	}
}

func TestRefreshKnowledge_CommandArgs(t *testing.T) {
	mock := &MockExecutor{Response: "output"}
	cli := &CLI{Executor: mock}

	_, err := cli.RefreshKnowledge("existing", "new", "2026-04-08T14:00:00Z")
	if err != nil {
		t.Fatalf("RefreshKnowledge returned error: %v", err)
	}

	call := mock.Calls[0]
	argsStr := strings.Join(call.Args, " ")
	for _, want := range []string{"--model", "claude-opus-4-6", "--print", "--output-format", "text"} {
		if !strings.Contains(argsStr, want) {
			t.Errorf("args %q missing expected flag %q", argsStr, want)
		}
	}
}

func TestRefreshKnowledge_Error(t *testing.T) {
	mock := &MockExecutor{
		Err:    errors.New("exit status 1"),
		Stderr: "refresh error details",
	}
	cli := &CLI{Executor: mock}

	_, err := cli.RefreshKnowledge("existing", "new", "2026-04-08T14:00:00Z")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "claude refresh-knowledge failed") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "claude refresh-knowledge failed")
	}
}

func TestCheckAvailable_Success(t *testing.T) {
	mock := &MockExecutor{
		CallResponses: []MockResponse{
			{Response: "claude 1.0.0", Stderr: "", Err: nil}, // --version
			{Response: "OK", Stderr: "", Err: nil},           // auth check
		},
	}
	cli := &CLI{Executor: mock}

	err := cli.CheckAvailable()
	if err != nil {
		t.Fatalf("CheckAvailable returned error: %v", err)
	}

	if len(mock.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(mock.Calls))
	}

	// First call: --version
	versionCall := mock.Calls[0]
	if versionCall.Name != "claude" {
		t.Errorf("first call name = %q, want %q", versionCall.Name, "claude")
	}
	versionArgs := strings.Join(versionCall.Args, " ")
	if !strings.Contains(versionArgs, "--version") {
		t.Errorf("first call args = %q, want --version", versionArgs)
	}

	// Second call: auth check via --print with stdin
	authCall := mock.Calls[1]
	if authCall.Name != "claude" {
		t.Errorf("second call name = %q, want %q", authCall.Name, "claude")
	}
}

func TestCheckAvailable_CLINotFound(t *testing.T) {
	mock := &MockExecutor{
		CallResponses: []MockResponse{
			{Response: "", Stderr: "command not found", Err: errors.New("exec: not found")},
		},
	}
	cli := &CLI{Executor: mock}

	err := cli.CheckAvailable()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "not executable") {
		t.Errorf("error = %q, expected it to mention CLI not found", err.Error())
	}
}

func TestCheckAvailable_NotAuthenticated(t *testing.T) {
	mock := &MockExecutor{
		CallResponses: []MockResponse{
			{Response: "claude 1.0.0", Stderr: "", Err: nil},                              // --version succeeds
			{Response: "", Stderr: "not authenticated", Err: errors.New("exit status 1")}, // auth fails
		},
	}
	cli := &CLI{Executor: mock}

	err := cli.CheckAvailable()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not authenticated") && !strings.Contains(err.Error(), "not working") {
		t.Errorf("error = %q, expected it to mention authentication", err.Error())
	}
}

func TestNewCLI_DefaultExecutor(t *testing.T) {
	cli := NewCLI()
	if cli.Executor == nil {
		t.Fatal("NewCLI() Executor is nil")
	}
	if _, ok := cli.Executor.(*DefaultExecutor); !ok {
		t.Errorf("NewCLI() Executor type = %T, want *DefaultExecutor", cli.Executor)
	}
}

func TestBaseArgs(t *testing.T) {
	cli := NewCLI()
	args := cli.baseArgs()
	want := []string{"--model", "claude-opus-4-6", "--print", "--output-format", "text"}
	if len(args) != len(want) {
		t.Fatalf("baseArgs() length = %d, want %d", len(args), len(want))
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("baseArgs()[%d] = %q, want %q", i, args[i], w)
		}
	}
}
