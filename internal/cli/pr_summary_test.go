package cli

import "testing"

func TestSummarizePullRequestState_ElidesNoPRWhenNoPendingWork(t *testing.T) {
	summary := summarizePullRequestState(
		prContext{
			HasPendingWork:   false,
			HasUniqueCommits: false,
		},
		nil,
		workflowExpectations{PRsExpected: true},
	)

	if summary.Column != "" {
		t.Fatalf("Column = %q, want empty", summary.Column)
	}
	if summary.Reason != "" {
		t.Fatalf("Reason = %q, want empty", summary.Reason)
	}
}

func TestSummarizePullRequestState_ShowsNoPRWhenPendingWorkAndPRsExpected(t *testing.T) {
	summary := summarizePullRequestState(
		prContext{
			HasPendingWork:   true,
			HasUniqueCommits: true,
		},
		nil,
		workflowExpectations{PRsExpected: true},
	)

	if summary.Column != "No PR" {
		t.Fatalf("Column = %q, want %q", summary.Column, "No PR")
	}
	if summary.Reason != "No PR" {
		t.Fatalf("Reason = %q, want %q", summary.Reason, "No PR")
	}
}
