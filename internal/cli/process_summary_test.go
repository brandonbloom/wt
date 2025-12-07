package cli

import (
	"testing"

	"github.com/brandonbloom/wt/internal/processes"
)

func TestSummarizeProcessesBasics(t *testing.T) {
	cases := []struct {
		name     string
		procs    []processes.Process
		limit    int
		expected string
	}{
		{
			name:     "none",
			expected: "-",
		},
		{
			name: "single",
			procs: []processes.Process{
				{PID: 42, Command: "vim"},
			},
			expected: "vim (42)",
		},
		{
			name: "minimumVisibleExceedsLimit",
			procs: []processes.Process{
				{PID: 1, Command: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				{PID: 2, Command: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
				{PID: 3, Command: "cccccccccccccccccccccccccccccccc"},
				{PID: 4, Command: "dddddddddddddddddddddddddddddddd"},
			},
			limit:    10,
			expected: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa (1), bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb (2), cccccccccccccccccccccccccccccccc (3), + 1 more",
		},
		{
			name: "respectsLimitWithOverflow",
			procs: []processes.Process{
				{PID: 10, Command: "codex"},
				{PID: 20, Command: "emacs"},
				{PID: 30, Command: "node"},
				{PID: 40, Command: "vim"},
			},
			limit:    40,
			expected: "codex (10), emacs (20), node (30), + 1 more",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeProcesses(tc.procs, tc.limit)
			if got != tc.expected {
				t.Fatalf("summarizeProcesses() = %q, want %q", got, tc.expected)
			}
		})
	}
}
