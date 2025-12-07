package cli

import (
	"testing"

	"github.com/brandonbloom/wt/internal/processes"
)

func TestPruneProcessList(t *testing.T) {
	origSelf, origParent := currentProcessPID, parentProcessPID
	currentProcessPID = 1000
	parentProcessPID = 2000
	defer func() {
		currentProcessPID = origSelf
		parentProcessPID = origParent
	}()

	procs := []processes.Process{
		{PID: 1000, PPID: 2000, Command: "wt", CWD: "/tmp/a"},
		{PID: 2000, PPID: 0, Command: "zsh", CWD: "/tmp/a"},
		{PID: 3000, PPID: 2000, Command: "rails", CWD: "/tmp/a"},
		{PID: 4000, PPID: 5000, Command: "zsh", CWD: "/tmp/a"},
		{PID: 4500, PPID: 4000, Command: "zsh", CWD: "/tmp/a"},
	}

	filtered := pruneProcessList(procs)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 processes, got %d: %#v", len(filtered), filtered)
	}

	hasRails := false
	hasZsh := false
	for _, proc := range filtered {
		switch processCommandLabel(proc.Command) {
		case "rails":
			hasRails = true
		case "zsh":
			hasZsh = true
		case "wt":
			t.Fatalf("wt process should be excluded")
		}
	}
	if !hasRails || !hasZsh {
		t.Fatalf("expected rails and zsh to remain, got %#v", filtered)
	}
}
