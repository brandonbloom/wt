package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestFriendlyWorktreeGitError(t *testing.T) {
	err := fmt.Errorf("git rev-parse --abbrev-ref HEAD: exit status 128; fatal: not a git repository: /Users/me/project/main/.git/worktrees/neon-thunder")
	msg, ok := friendlyWorktreeGitError("neon-thunder", err)
	if !ok {
		t.Fatalf("expected detection")
	}
	if !strings.Contains(msg, "broken git metadata") {
		t.Fatalf("unexpected message: %s", msg)
	}
	if !strings.Contains(msg, "neon-thunder") {
		t.Fatalf("worktree name missing: %s", msg)
	}
	if !strings.Contains(msg, ".git/worktrees/neon-thunder") {
		t.Fatalf("missing metadata path: %s", msg)
	}
}
