package knowledge

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilePath_ReturnsCorrectPath(t *testing.T) {
	t.Parallel()
	s := NewStore("/base")
	got := s.FilePath("owner/repo", "TICKET-123")
	want := filepath.Join("/base", "owner", "repo", "TICKET-123.yaml")
	if got != want {
		t.Fatalf("FilePath = %q, want %q", got, want)
	}
}

func TestWrite_CreatesDirectoriesAndFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)
	err := s.Write("owner/repo", "TICKET-1", "hello")
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	p := filepath.Join(dir, "owner", "repo", "TICKET-1.yaml")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file to exist at %s: %v", p, err)
	}
}

func TestRead_ReturnsContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)
	content := "some knowledge content"
	if err := s.Write("owner/repo", "TICKET-1", content); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got, err := s.Read("owner/repo", "TICKET-1")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if got != content {
		t.Fatalf("Read = %q, want %q", got, content)
	}
}

func TestRead_MissingFileReturnsErrNotExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)
	_, err := s.Read("owner/repo", "MISSING")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestExists_ReturnsTrueAndFalse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)

	if s.Exists("owner/repo", "TICKET-1") {
		t.Fatal("expected Exists to return false before Write")
	}

	if err := s.Write("owner/repo", "TICKET-1", "data"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if !s.Exists("owner/repo", "TICKET-1") {
		t.Fatal("expected Exists to return true after Write")
	}
}

func TestStalenessNoteFromTime_ReturnsWarningForOldTime(t *testing.T) {
	t.Parallel()
	old := time.Now().Add(-48 * time.Hour).UTC()
	note := StalenessNoteFromTime(old, 1*time.Hour)
	if note == "" {
		t.Fatal("expected a staleness warning, got empty string")
	}
	if !strings.Contains(note, "Warning") {
		t.Fatalf("expected warning in note, got %q", note)
	}
}

func TestStalenessNoteFromTime_ReturnsEmptyForFreshTime(t *testing.T) {
	t.Parallel()
	fresh := time.Now().UTC()
	note := StalenessNoteFromTime(fresh, 1*time.Hour)
	if note != "" {
		t.Fatalf("expected empty string for fresh time, got %q", note)
	}
}

func TestStalenessNoteFromTime_ReturnsEmptyForZeroTime(t *testing.T) {
	t.Parallel()
	note := StalenessNoteFromTime(time.Time{}, 1*time.Hour)
	if note != "" {
		t.Fatalf("expected empty string for zero time, got %q", note)
	}
}

func TestWriteKnowledge_And_ReadKnowledge(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)

	kf := &KnowledgeFile{
		TicketID:      "SV1-100",
		Branch:        "feature/SV1-100-work",
		LastRefreshed: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		JIRA: JIRASection{
			Summary:  "Test ticket",
			Status:   "Open",
			Labels:   []string{"backend"},
			Comments: []JIRAComment{},
		},
		PullRequests: []PullRequest{},
		MainCommits:  []CommitEntry{},
		Notes:        []Note{{Date: time.Date(2026, 4, 8, 13, 0, 0, 0, time.UTC), Body: "A note"}},
	}

	if err := s.WriteKnowledge("org/repo", "SV1-100", kf); err != nil {
		t.Fatalf("WriteKnowledge failed: %v", err)
	}

	got, err := s.ReadKnowledge("org/repo", "SV1-100")
	if err != nil {
		t.Fatalf("ReadKnowledge failed: %v", err)
	}
	if got.TicketID != "SV1-100" {
		t.Errorf("TicketID = %q, want %q", got.TicketID, "SV1-100")
	}
	if got.JIRA.Summary != "Test ticket" {
		t.Errorf("JIRA.Summary = %q, want %q", got.JIRA.Summary, "Test ticket")
	}
	if len(got.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(got.Notes))
	}
	if got.Notes[0].Body != "A note" {
		t.Errorf("Note.Body = %q, want %q", got.Notes[0].Body, "A note")
	}
	// Verify slices are never nil.
	if got.PullRequests == nil {
		t.Error("PullRequests should not be nil")
	}
	if got.MainCommits == nil {
		t.Error("MainCommits should not be nil")
	}
	if got.JIRA.Labels == nil {
		t.Error("JIRA.Labels should not be nil")
	}
}

func TestReadKnowledge_MissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)

	_, err := s.ReadKnowledge("org/repo", "MISSING")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestReadKnowledge_LegacyMDFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)

	// Create a legacy .md file but no .yaml file.
	mdPath := s.oldFilePath("org/repo", "SV1-100")
	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(mdPath, []byte("# old content"), 0o644); err != nil {
		t.Fatalf("failed to write .md file: %v", err)
	}

	_, err := s.ReadKnowledge("org/repo", "SV1-100")
	if err == nil {
		t.Fatal("expected error for legacy .md file")
	}
	if !strings.Contains(err.Error(), "legacy .md format") {
		t.Errorf("error should mention legacy format, got: %s", err.Error())
	}
}

func TestReadSection_JIRA(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)

	kf := &KnowledgeFile{
		TicketID:      "SV1-100",
		Branch:        "feature/SV1-100-work",
		LastRefreshed: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		JIRA: JIRASection{
			Summary:  "Feature X",
			Status:   "Open",
			Labels:   []string{},
			Comments: []JIRAComment{},
		},
		PullRequests: []PullRequest{},
		MainCommits:  []CommitEntry{},
		Notes:        []Note{{Date: time.Now().UTC(), Body: "My note"}},
	}
	if err := s.WriteKnowledge("org/repo", "SV1-100", kf); err != nil {
		t.Fatalf("WriteKnowledge failed: %v", err)
	}

	result, err := s.ReadSection("org/repo", "SV1-100", "jira")
	if err != nil {
		t.Fatalf("ReadSection(jira) failed: %v", err)
	}
	if !strings.Contains(result, "Feature X") {
		t.Errorf("jira section should contain summary, got: %s", result)
	}
	if strings.Contains(result, "My note") {
		t.Errorf("jira section should not contain notes, got: %s", result)
	}
}

func TestReadSection_Notes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)

	kf := &KnowledgeFile{
		TicketID:      "SV1-100",
		Branch:        "feature/SV1-100-work",
		LastRefreshed: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		JIRA: JIRASection{
			Summary:  "Feature X",
			Labels:   []string{},
			Comments: []JIRAComment{},
		},
		PullRequests: []PullRequest{},
		MainCommits:  []CommitEntry{},
		Notes:        []Note{{Date: time.Now().UTC(), Body: "Important decision"}},
	}
	if err := s.WriteKnowledge("org/repo", "SV1-100", kf); err != nil {
		t.Fatalf("WriteKnowledge failed: %v", err)
	}

	result, err := s.ReadSection("org/repo", "SV1-100", "notes")
	if err != nil {
		t.Fatalf("ReadSection(notes) failed: %v", err)
	}
	if !strings.Contains(result, "Important decision") {
		t.Errorf("notes section should contain note body, got: %s", result)
	}
}

func TestReadSection_EmptyScope_ReturnsFullFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)

	kf := &KnowledgeFile{
		TicketID:      "SV1-100",
		Branch:        "feature/SV1-100-work",
		LastRefreshed: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		JIRA: JIRASection{
			Summary:  "Feature X",
			Labels:   []string{},
			Comments: []JIRAComment{},
		},
		PullRequests: []PullRequest{},
		MainCommits:  []CommitEntry{},
		Notes:        []Note{},
	}
	if err := s.WriteKnowledge("org/repo", "SV1-100", kf); err != nil {
		t.Fatalf("WriteKnowledge failed: %v", err)
	}

	result, err := s.ReadSection("org/repo", "SV1-100", "")
	if err != nil {
		t.Fatalf("ReadSection('') failed: %v", err)
	}
	if !strings.Contains(result, "SV1-100") {
		t.Errorf("full content should contain ticket ID, got: %s", result)
	}
}

func TestReadSection_InvalidScope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewStore(dir)

	kf := &KnowledgeFile{
		TicketID:      "SV1-100",
		Branch:        "feature/SV1-100-work",
		LastRefreshed: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		JIRA: JIRASection{
			Labels:   []string{},
			Comments: []JIRAComment{},
		},
		PullRequests: []PullRequest{},
		MainCommits:  []CommitEntry{},
		Notes:        []Note{},
	}
	if err := s.WriteKnowledge("org/repo", "SV1-100", kf); err != nil {
		t.Fatalf("WriteKnowledge failed: %v", err)
	}

	_, err := s.ReadSection("org/repo", "SV1-100", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
	if !strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("error should mention unknown scope, got: %s", err.Error())
	}
}
