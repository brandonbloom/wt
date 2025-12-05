package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brandonbloom/wt/internal/gitutil"
	"github.com/brandonbloom/wt/internal/project"
	"github.com/brandonbloom/wt/internal/timefmt"
	"github.com/spf13/cobra"
)

func runStatus(cmd *cobra.Command, args []string) error {
	proj, err := loadProjectFromWD()
	if err != nil {
		return err
	}

	worktrees, err := project.ListWorktrees(proj.Root)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	current := ""
	for _, wt := range worktrees {
		if isWithin(wd, wt.Path) {
			current = wt.Name
			break
		}
	}

	now := time.Now()
	for _, wt := range worktrees {
		status, err := collectWorktreeStatus(wt)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %v\n", wt.Name, err)
			continue
		}
		status.Current = wt.Name == current
		printWorktreeStatus(cmd, status, now)
	}

	return nil
}

type worktreeStatus struct {
	Name      string
	Branch    string
	Dirty     bool
	Ahead     int
	Behind    int
	Timestamp time.Time
	Current   bool
}

func collectWorktreeStatus(wt project.Worktree) (*worktreeStatus, error) {
	branch, err := gitutil.CurrentBranch(wt.Path)
	if err != nil {
		return nil, err
	}
	dirty, err := gitutil.Dirty(wt.Path)
	if err != nil {
		return nil, err
	}
	ahead, behind, err := gitutil.AheadBehind(wt.Path)
	if err != nil {
		return nil, err
	}
	ts, err := gitutil.HeadTimestamp(wt.Path)
	if err != nil {
		return nil, err
	}
	if dirty {
		if dirtyTS, derr := gitutil.LatestDirtyTimestamp(wt.Path); derr == nil {
			ts = dirtyTS
		}
	}
	return &worktreeStatus{
		Name:      wt.Name,
		Branch:    branch,
		Dirty:     dirty,
		Ahead:     ahead,
		Behind:    behind,
		Timestamp: ts,
	}, nil
}

func printWorktreeStatus(cmd *cobra.Command, status *worktreeStatus, now time.Time) {
	prefix := "  "
	if status.Current {
		prefix = "* "
	}
	dirtyMarker := ""
	if status.Dirty {
		dirtyMarker = "!"
	}
	relative := timefmt.Relative(status.Timestamp, now)
	fmt.Fprintf(
		cmd.OutOrStdout(),
		"%s%-12s %-20s %s %s\n",
		prefix,
		status.Name,
		fmt.Sprintf("%s%s ↑%d ↓%d", status.Branch, dirtyMarker, status.Ahead, status.Behind),
		relative,
		"(PR: pending)",
	)
}

func isWithin(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}
