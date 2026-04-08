package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store manages knowledge files on disk.
type Store struct {
	baseDir string // e.g. ~/.config/memgen/knowledge
}

// NewStore creates a new Store rooted at baseDir.
func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// FilePath returns the full path for a knowledge file: baseDir/owner/repo/TICKET-ID.md
// The repo parameter is expected in "owner/repo" format.
func (s *Store) FilePath(repo, ticketID string) string {
	return filepath.Join(s.baseDir, repo, ticketID+".md")
}

// Exists checks if the knowledge file exists.
func (s *Store) Exists(repo, ticketID string) bool {
	_, err := os.Stat(s.FilePath(repo, ticketID))
	return err == nil
}

// Read reads the knowledge file contents. Returns os.ErrNotExist if missing.
func (s *Store) Read(repo, ticketID string) (string, error) {
	data, err := os.ReadFile(s.FilePath(repo, ticketID))
	if err != nil {
		if os.IsNotExist(err) {
			return "", os.ErrNotExist
		}
		return "", err
	}
	return string(data), nil
}

// Write writes content to the knowledge file, creating directories as needed.
func (s *Store) Write(repo, ticketID string, content string) error {
	p := s.FilePath(repo, ticketID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

// LastRefreshed extracts the "Last Refreshed" timestamp from knowledge file content.
// It looks for a line matching "**Last Refreshed**: <RFC3339 timestamp>" and parses it.
// Returns zero time if the marker is not found or the timestamp is malformed.
func (s *Store) LastRefreshed(content string) time.Time {
	const prefix = "**Last Refreshed**: "
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			raw := strings.TrimSpace(line[len(prefix):])
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return time.Time{}
			}
			return t
		}
	}
	return time.Time{}
}

// StalenessNote returns a staleness warning string if the knowledge is older than the threshold.
// Returns an empty string if the content is fresh enough or if no timestamp is found.
func (s *Store) StalenessNote(content string, threshold time.Duration) string {
	t := s.LastRefreshed(content)
	if t.IsZero() {
		return ""
	}
	age := time.Since(t)
	if age <= threshold {
		return ""
	}
	hours := int(age.Hours())
	if hours < 24 {
		return fmt.Sprintf("Warning: knowledge is %d hours old (threshold: %s)", hours, threshold)
	}
	days := hours / 24
	return fmt.Sprintf("Warning: knowledge is %d days old (threshold: %s)", days, threshold)
}
