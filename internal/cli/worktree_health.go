package cli

import (
	"fmt"
	"strings"
)

func friendlyWorktreeGitError(worktreeName string, err error) (string, bool) {
	if err == nil {
		return "", false
	}
	msg := singleLineError(err)
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "not a git repository") {
		return "", false
	}
	if !strings.Contains(msg, ".git/worktrees/") {
		return "", false
	}
	missing := extractWorktreeMetadataPath(msg)
	detail := ""
	if missing != "" {
		detail = fmt.Sprintf(" (missing %s)", missing)
	}
	return fmt.Sprintf("broken git metadata for %s%s; run `git worktree prune` in your main worktree or delete the directory", worktreeName, detail), true
}

func extractWorktreeMetadataPath(msg string) string {
	for _, field := range strings.Fields(msg) {
		if strings.Contains(field, ".git/worktrees/") {
			return strings.Trim(field, ":")
		}
	}
	return ""
}
