package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Store manages knowledge files on disk.
type Store struct {
	baseDir string // e.g. ~/.config/memgen/knowledge
}

// NewStore creates a new Store rooted at baseDir.
func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// FilePath returns the full path for a knowledge file: baseDir/owner/repo/TICKET-ID.yaml
// The repo parameter is expected in "owner/repo" format.
func (s *Store) FilePath(repo, ticketID string) string {
	return filepath.Join(s.baseDir, repo, ticketID+".yaml")
}

// oldFilePath returns the legacy .md file path for backward compatibility checks.
func (s *Store) oldFilePath(repo, ticketID string) string {
	return filepath.Join(s.baseDir, repo, ticketID+".md")
}

// Exists checks if the YAML knowledge file exists.
// Returns false even if a legacy .md file exists — users should re-init.
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

// StalenessNoteFromTime returns a staleness warning if the given time is older than the threshold.
// Returns an empty string if the time is zero or within the threshold.
func StalenessNoteFromTime(t time.Time, threshold time.Duration) string {
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

// ReadKnowledge reads and unmarshals the YAML knowledge file.
// Returns a helpful error if the legacy .md file exists but .yaml does not.
func (s *Store) ReadKnowledge(repo, ticketID string) (*KnowledgeFile, error) {
	data, err := os.ReadFile(s.FilePath(repo, ticketID))
	if err != nil {
		if os.IsNotExist(err) {
			// Check if legacy .md exists.
			if _, mdErr := os.Stat(s.oldFilePath(repo, ticketID)); mdErr == nil {
				return nil, fmt.Errorf("knowledge file %s has legacy .md format; please re-run init to convert to YAML", ticketID)
			}
			return nil, os.ErrNotExist
		}
		return nil, err
	}

	var kf KnowledgeFile
	if err := yaml.Unmarshal(data, &kf); err != nil {
		return nil, fmt.Errorf("parsing knowledge YAML for %s: %w", ticketID, err)
	}

	// Ensure slices are never nil after unmarshaling.
	if kf.JIRA.Labels == nil {
		kf.JIRA.Labels = []string{}
	}
	if kf.JIRA.Comments == nil {
		kf.JIRA.Comments = []JIRAComment{}
	}
	if kf.PullRequests == nil {
		kf.PullRequests = []PullRequest{}
	}
	for i := range kf.PullRequests {
		if kf.PullRequests[i].Reviews == nil {
			kf.PullRequests[i].Reviews = []PRReview{}
		}
		if kf.PullRequests[i].Comments == nil {
			kf.PullRequests[i].Comments = []PRComment{}
		}
		if kf.PullRequests[i].Commits == nil {
			kf.PullRequests[i].Commits = []CommitEntry{}
		}
	}
	if kf.MainCommits == nil {
		kf.MainCommits = []CommitEntry{}
	}
	if kf.Notes == nil {
		kf.Notes = []Note{}
	}

	return &kf, nil
}

// WriteKnowledge marshals and writes the knowledge file as YAML.
func (s *Store) WriteKnowledge(repo, ticketID string, kf *KnowledgeFile) error {
	data, err := yaml.Marshal(kf)
	if err != nil {
		return fmt.Errorf("marshaling knowledge YAML for %s: %w", ticketID, err)
	}
	return s.Write(repo, ticketID, string(data))
}

// slimPullRequest is a reduced PR view for the "pr" scope.
type slimPullRequest struct {
	Number int    `yaml:"number"`
	Title  string `yaml:"title"`
	State  string `yaml:"state"`
	Author string `yaml:"author"`
	URL    string `yaml:"url"`
	Body   string `yaml:"body"`
}

// ReadSection reads a scoped portion of the knowledge file.
// Supported scopes: "" (full file), "jira", "pr", "git", "comments", "notes".
func (s *Store) ReadSection(repo, ticketID, scope string) (string, error) {
	if scope == "" {
		// Return full file content as string.
		return s.Read(repo, ticketID)
	}

	kf, err := s.ReadKnowledge(repo, ticketID)
	if err != nil {
		return "", err
	}

	var section any
	switch scope {
	case "jira":
		section = kf.JIRA
	case "pr":
		slim := make([]slimPullRequest, 0, len(kf.PullRequests))
		for _, pr := range kf.PullRequests {
			slim = append(slim, slimPullRequest{
				Number: pr.Number,
				Title:  pr.Title,
				State:  pr.State,
				Author: pr.Author,
				URL:    pr.URL,
				Body:   pr.Body,
			})
		}
		section = slim
	case "git":
		section = struct {
			PullRequests []PullRequest `yaml:"pull_requests"`
			MainCommits  []CommitEntry `yaml:"main_commits"`
		}{
			PullRequests: kf.PullRequests,
			MainCommits:  kf.MainCommits,
		}
	case "comments":
		type commentSection struct {
			PRComments []PRComment `yaml:"pr_comments"`
			PRReviews  []PRReview  `yaml:"pr_reviews"`
		}
		allComments := make([]PRComment, 0)
		allReviews := make([]PRReview, 0)
		for _, pr := range kf.PullRequests {
			allComments = append(allComments, pr.Comments...)
			allReviews = append(allReviews, pr.Reviews...)
		}
		section = commentSection{
			PRComments: allComments,
			PRReviews:  allReviews,
		}
	case "notes":
		section = kf.Notes
	default:
		return "", fmt.Errorf("unknown scope %q: valid scopes are jira, pr, git, comments, notes", scope)
	}

	data, err := yaml.Marshal(section)
	if err != nil {
		return "", fmt.Errorf("marshaling %s section: %w", scope, err)
	}
	return string(data), nil
}
