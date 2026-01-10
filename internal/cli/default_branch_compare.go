package cli

import (
	"path/filepath"

	"github.com/brandonbloom/wt/internal/gitutil"
	"github.com/brandonbloom/wt/internal/project"
)

type defaultBranchCompareContext struct {
	CompareRef   string
	PRsExpected  bool
	SyncMode     gitutil.DefaultBranchSyncMode
	DefaultBranch string
}

func defaultBranchComparisonContext(proj *project.Project) defaultBranchCompareContext {
	if proj == nil {
		return defaultBranchCompareContext{}
	}
	workdir := proj.DefaultWorktreePath
	if workdir == "" && proj.Root != "" && proj.DefaultWorktree != "" {
		workdir = filepath.Join(proj.Root, proj.DefaultWorktree)
	}
	ref, mode, err := gitutil.DefaultBranchComparisonRef(workdir, "origin", proj.Config.DefaultBranch)
	if err != nil {
		return defaultBranchCompareContext{
			CompareRef:    proj.Config.DefaultBranch,
			PRsExpected:   false,
			SyncMode:      gitutil.DefaultBranchLocalFirst,
			DefaultBranch: proj.Config.DefaultBranch,
		}
	}
	return defaultBranchCompareContext{
		CompareRef:    ref,
		PRsExpected:   mode == gitutil.DefaultBranchRemoteFirst,
		SyncMode:      mode,
		DefaultBranch: proj.Config.DefaultBranch,
	}
}
