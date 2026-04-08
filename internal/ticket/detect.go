package ticket

import (
	"fmt"
	"regexp"
	"strings"
)

var ticketPattern = regexp.MustCompile(`[A-Z][A-Z0-9]+-\d+`)

// Extract returns the first JIRA ticket ID found in the branch name.
// Returns empty string and an error if no ticket found.
func Extract(branch string) (string, error) {
	match := ticketPattern.FindString(branch)
	if match == "" {
		return "", fmt.Errorf("no JIRA ticket detected in branch name %q; branch must contain a ticket ID like SV1-240", branch)
	}
	return match, nil
}

// BrowseURL returns the JIRA browse URL for a ticket.
func BrowseURL(jiraBaseURL, ticketID string) string {
	return fmt.Sprintf("%s/browse/%s", strings.TrimRight(jiraBaseURL, "/"), ticketID)
}
