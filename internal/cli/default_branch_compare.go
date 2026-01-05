package cli

import (
	"path/filepath"

	"github.com/brandonbloom/wt/internal/gitutil"
	"github.com/brandonbloom/wt/internal/project"
)

func defaultBranchComparisonRef(proj *project.Project) string {
	if proj == nil {
		return ""
	}
	workdir := proj.DefaultWorktreePath
	if workdir == "" && proj.Root != "" && proj.DefaultWorktree != "" {
		workdir = filepath.Join(proj.Root, proj.DefaultWorktree)
	}
	ref, _, err := gitutil.DefaultBranchComparisonRef(workdir, "origin", proj.Config.DefaultBranch)
	if err != nil {
		return proj.Config.DefaultBranch
	}
	return ref
}

