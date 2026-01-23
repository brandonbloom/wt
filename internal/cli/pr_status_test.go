package cli

import (
	"context"
	"testing"
)

func TestMarkPRInterrupted(t *testing.T) {
	statuses := []*worktreeStatus{
		{Name: "pending", PRStatus: prLoadingLabel},
		{Name: "custom", PRStatus: "PR #42 open"},
		{Name: "empty"},
	}

	var updated []string
	markPRInterrupted(statuses, func(ws *worktreeStatus) {
		updated = append(updated, ws.Name)
	})

	wantUpdates := []string{"pending", "empty"}
	if len(updated) != len(wantUpdates) {
		t.Fatalf("onUpdate called for %v, want %v", updated, wantUpdates)
	}
	for i, name := range wantUpdates {
		if updated[i] != name {
			t.Fatalf("onUpdate[%d] = %q, want %q", i, updated[i], name)
		}
	}
	if statuses[0].PRStatus != prInterruptedLabel {
		t.Fatalf("status 0 PRStatus = %q, want %q", statuses[0].PRStatus, prInterruptedLabel)
	}
	if statuses[1].PRStatus != "PR #42 open" {
		t.Fatalf("status 1 changed to %q", statuses[1].PRStatus)
	}
	if statuses[2].PRStatus != prInterruptedLabel {
		t.Fatalf("status 2 PRStatus = %q, want %q", statuses[2].PRStatus, prInterruptedLabel)
	}
}

func TestFetchPullRequestStatuses_SkipsStatusesWithErrors(t *testing.T) {
	statuses := []*worktreeStatus{
		{
			Name:      "broken",
			HasError:  true,
			Error:     "git failed",
			PRStatus:  "error: git failed",
			Branch:    "",
			Path:      "/does/not/matter",
			HeadHash:  "",
			Timestamp: currentTimeOverride(),
		},
	}

	if err := fetchPullRequestStatuses(context.Background(), nil, nil, statuses, workflowExpectations{PRsExpected: true}, nil); err != nil {
		t.Fatalf("fetchPullRequestStatuses returned error: %v", err)
	}
	if got := statuses[0].PRStatus; got != "error: git failed" {
		t.Fatalf("PRStatus = %q, want %q", got, "error: git failed")
	}
}
