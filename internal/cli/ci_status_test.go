package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestMarkCIInterrupted(t *testing.T) {
	statuses := []*worktreeStatus{
		{Name: "foo"},
		{Name: "bar", CIStatus: "CI✓"},
	}
	var updated []string
	markCIInterrupted(statuses, func(ws *worktreeStatus) {
		updated = append(updated, ws.Name)
	})

	if got := statuses[0].CIStatus; got != "CI: interrupted" {
		t.Fatalf("status 0 CIStatus = %q, want %q", got, "CI: interrupted")
	}
	if len(updated) != 1 || updated[0] != "foo" {
		t.Fatalf("onUpdate called for %v, want [foo]", updated)
	}
	if got := statuses[1].CIStatus; got != "CI✓" {
		t.Fatalf("status 1 CIStatus changed to %q, want CI✓", got)
	}
}

func TestFormatCIErrorLabelCommitMissing(t *testing.T) {
	err := fmt.Errorf("wrap: %w", errCommitNotOnGitHub)
	got := formatCIErrorLabel(err)
	want := fmt.Sprintf("CI: ? %s", ciMissingCommitMsg)
	if got != want {
		t.Fatalf("formatCIErrorLabel() = %q, want %q", got, want)
	}
}

func TestClassifyGhErrorDetectsMissingCommit(t *testing.T) {
	err := classifyGhError("gh: No commit found for SHA: 123", errors.New("fail"))
	if !errors.Is(err, errCommitNotOnGitHub) {
		t.Fatalf("expected errCommitNotOnGitHub, got %v", err)
	}
}
