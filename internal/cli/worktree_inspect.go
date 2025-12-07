package cli

import (
	"time"

	"github.com/brandonbloom/wt/internal/gitutil"
	"github.com/brandonbloom/wt/internal/project"
)

type worktreeGitData struct {
	Worktree           project.Worktree
	Branch             string
	Dirty              bool
	HasStash           bool
	Operation          string
	Ahead              int
	Behind             int
	BaseAhead          int
	BaseBehind         int
	Timestamp          time.Time
	UniqueAhead        int
	HeadHash           string
	HasRemoteBranch    bool
	RemoteMatchesHead  bool
	MergedIntoDefault  bool
	TreeMatchesDefault bool
}

func gatherWorktreeGitData(proj *project.Project, wt project.Worktree) (*worktreeGitData, error) {
	data := &worktreeGitData{Worktree: wt}

	branch, err := gitutil.CurrentBranch(wt.Path)
	if err != nil {
		return nil, err
	}
	data.Branch = branch

	dirty, err := gitutil.Dirty(wt.Path)
	if err != nil {
		return nil, err
	}
	data.Dirty = dirty

	if branch != "" {
		stash, err := gitutil.HasBranchStash(wt.Path, branch)
		if err != nil {
			return nil, err
		}
		data.HasStash = stash
	}

	operation, _ := gitutil.WorktreeOperation(wt.Path)
	data.Operation = operation

	ahead, behind, err := gitutil.AheadBehind(wt.Path, branch)
	if err != nil {
		if operation == "" && !isDetachedHeadError(err) {
			return nil, err
		}
		ahead, behind = 0, 0
	}
	data.Ahead = ahead
	data.Behind = behind

	ts, err := gitutil.HeadTimestamp(wt.Path)
	if err != nil {
		return nil, err
	}
	if dirty {
		if dirtyTS, derr := gitutil.LatestDirtyTimestamp(wt.Path); derr == nil {
			ts = dirtyTS
		}
	}
	data.Timestamp = ts

	baseAhead, baseBehind, err := gitutil.AheadBehindDefaultBranch(wt.Path, proj.Config.DefaultBranch)
	if err != nil {
		return nil, err
	}
	data.BaseAhead = baseAhead
	data.BaseBehind = baseBehind

	headHash, err := gitutil.Run(wt.Path, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	data.HeadHash = headHash

	merged, err := gitutil.HeadMergedInto(wt.Path, proj.Config.DefaultBranch)
	if err != nil {
		return nil, err
	}
	data.MergedIntoDefault = merged

	treeMatches, err := gitutil.HeadSameTree(wt.Path, proj.Config.DefaultBranch)
	if err != nil {
		return nil, err
	}
	data.TreeMatchesDefault = treeMatches

	uniqueAhead, err := gitutil.UniqueCommitsComparedTo(wt.Path, proj.Config.DefaultBranch)
	if err != nil {
		return nil, err
	}
	data.UniqueAhead = uniqueAhead

	if proj.DefaultWorktreePath != "" {
		remoteHash, exists, err := gitutil.RemoteBranchHead(proj.DefaultWorktreePath, "origin", branch)
		if err != nil {
			return nil, err
		}
		data.HasRemoteBranch = exists
		if exists {
			data.RemoteMatchesHead = remoteHash == headHash
		}
	}

	return data, nil
}
