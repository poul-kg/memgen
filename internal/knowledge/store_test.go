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
	s := NewStore("/base")
	got := s.FilePath("owner/repo", "TICKET-123")
	want := filepath.Join("/base", "owner", "repo", "TICKET-123.md")
	if got != want {
		t.Fatalf("FilePath = %q, want %q", got, want)
	}
}

func TestWrite_CreatesDirectoriesAndFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	err := s.Write("owner/repo", "TICKET-1", "hello")
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	p := filepath.Join(dir, "owner", "repo", "TICKET-1.md")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file to exist at %s: %v", p, err)
	}
}

func TestRead_ReturnsContent(t *testing.T) {
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
	dir := t.TempDir()
	s := NewStore(dir)
	_, err := s.Read("owner/repo", "MISSING")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestExists_ReturnsTrueAndFalse(t *testing.T) {
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

func TestLastRefreshed_ParsesValidTimestamp(t *testing.T) {
	content := "# Knowledge\n**Last Refreshed**: 2026-04-08T12:00:00Z\nSome details."
	s := NewStore("")
	got := s.LastRefreshed(content)
	want := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("LastRefreshed = %v, want %v", got, want)
	}
}

func TestLastRefreshed_ReturnsZeroForMissing(t *testing.T) {
	s := NewStore("")
	got := s.LastRefreshed("no timestamp here")
	if !got.IsZero() {
		t.Fatalf("expected zero time, got %v", got)
	}
}

func TestLastRefreshed_ReturnsZeroForMalformed(t *testing.T) {
	content := "**Last Refreshed**: not-a-date"
	s := NewStore("")
	got := s.LastRefreshed(content)
	if !got.IsZero() {
		t.Fatalf("expected zero time for malformed timestamp, got %v", got)
	}
}

func TestStalenessNote_ReturnsWarningForOldContent(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	content := "**Last Refreshed**: " + old
	s := NewStore("")
	note := s.StalenessNote(content, 1*time.Hour)
	if note == "" {
		t.Fatal("expected a staleness warning, got empty string")
	}
	if !strings.Contains(note, "Warning") {
		t.Fatalf("expected warning in note, got %q", note)
	}
}

func TestStalenessNote_ReturnsEmptyForFreshContent(t *testing.T) {
	fresh := time.Now().UTC().Format(time.RFC3339)
	content := "**Last Refreshed**: " + fresh
	s := NewStore("")
	note := s.StalenessNote(content, 1*time.Hour)
	if note != "" {
		t.Fatalf("expected empty string for fresh content, got %q", note)
	}
}
