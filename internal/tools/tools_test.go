package tools

import (
	"testing"
)

func TestLockKey(t *testing.T) {
	t.Parallel()
	key := lockKey("org/repo", "SV1-240")
	if key != "org/repo/SV1-240" {
		t.Errorf("lockKey = %q, want %q", key, "org/repo/SV1-240")
	}
}

func TestExtractTicket(t *testing.T) {
	t.Parallel()
	id, err := extractTicket("feature/SV1-240-work")
	if err != nil {
		t.Fatalf("extractTicket returned error: %v", err)
	}
	if id != "SV1-240" {
		t.Errorf("extractTicket = %q, want %q", id, "SV1-240")
	}

	_, err = extractTicket("main")
	if err == nil {
		t.Error("expected error for branch without ticket")
	}
}
