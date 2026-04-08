package ticket

import (
	"testing"
)

func TestExtract(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		want    string
		wantErr bool
	}{
		{
			name:   "ticket prefix with description",
			branch: "SV1-240-mail-threading",
			want:   "SV1-240",
		},
		{
			name:   "four-letter project key",
			branch: "SBUX-111-some-task",
			want:   "SBUX-111",
		},
		{
			name:   "three-letter project key",
			branch: "SAI-342-some-task",
			want:   "SAI-342",
		},
		{
			name:   "bare ticket ID",
			branch: "SV1-240",
			want:   "SV1-240",
		},
		{
			name:    "no ticket in branch name",
			branch:  "feature-no-ticket",
			wantErr: true,
		},
		{
			name:    "lowercase does not match",
			branch:  "lowercase-sv1-240",
			wantErr: true,
		},
		{
			name:   "first match wins when multiple tickets present",
			branch: "SV1-240-SV1-241-merge",
			want:   "SV1-240",
		},
		{
			name:    "empty string",
			branch:  "",
			wantErr: true,
		},
		{
			name:    "single letter project key does not match",
			branch:  "A-1-minimal",
			wantErr: true,
		},
		{
			name:   "digits in project key are valid",
			branch: "ABC123-456",
			want:   "ABC123-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Extract(tt.branch)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Extract(%q) expected error, got %q", tt.branch, got)
				}
				return
			}
			if err != nil {
				t.Errorf("Extract(%q) unexpected error: %v", tt.branch, err)
				return
			}
			if got != tt.want {
				t.Errorf("Extract(%q) = %q, want %q", tt.branch, got, tt.want)
			}
		})
	}
}

func TestBrowseURL(t *testing.T) {
	tests := []struct {
		name        string
		jiraBaseURL string
		ticketID    string
		want        string
	}{
		{
			name:        "standard base URL without trailing slash",
			jiraBaseURL: "https://stitchai.atlassian.net",
			ticketID:    "SV1-240",
			want:        "https://stitchai.atlassian.net/browse/SV1-240",
		},
		{
			name:        "base URL with trailing slash is trimmed",
			jiraBaseURL: "https://stitchai.atlassian.net/",
			ticketID:    "SV1-240",
			want:        "https://stitchai.atlassian.net/browse/SV1-240",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BrowseURL(tt.jiraBaseURL, tt.ticketID)
			if got != tt.want {
				t.Errorf("BrowseURL(%q, %q) = %q, want %q", tt.jiraBaseURL, tt.ticketID, got, tt.want)
			}
		})
	}
}
