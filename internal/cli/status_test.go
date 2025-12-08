package cli

import (
	"testing"
	"time"

	"github.com/brandonbloom/wt/internal/processes"
)

func TestBuildColumnLayoutUsesFullWidth(t *testing.T) {
	now := time.Date(2024, time.March, 14, 15, 9, 26, 0, time.UTC)
	statuses := []*worktreeStatus{{
		Name:      "whimsical-canoe",
		Branch:    "whimsical-canoe",
		Timestamp: now.Add(-30 * time.Minute),
		PRStatus:  "No PR",
		CIStatus:  "CI? gh api error",
		Processes: []processes.Process{
			{PID: 123, Command: "sleep 100"},
			{PID: 456, Command: "watch -n1 wt status"},
		},
	}}

	baseLayout := buildColumnLayout(statuses, now, 0)
	if baseLayout.totalWidth() <= 0 {
		t.Fatalf("expected base total width > 0, got %d", baseLayout.totalWidth())
	}

	maxWidth := baseLayout.totalWidth() + 50
	layout := buildColumnLayout(statuses, now, maxWidth)

	if got := layout.totalWidth(); got != maxWidth {
		t.Fatalf("layout total width = %d, want %d", got, maxWidth)
	}

	last := layout.widths[statusColumnCount-1]
	if last <= baseLayout.widths[statusColumnCount-1] {
		t.Fatalf("last column width did not expand: base=%d new=%d", baseLayout.widths[statusColumnCount-1], last)
	}
}
