package cli

import (
	"context"
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

func gatherWorktreeGitData(ctx context.Context, proj *project.Project, wt project.Worktree, defaultCompareRef string) (*worktreeGitData, error) {
	data := &worktreeGitData{Worktree: wt}

	branch, err := withTraceRegion(ctx, "git current branch", func() (string, error) {
		return gitutil.CurrentBranch(wt.Path)
	})
	if err != nil {
		return nil, err
	}
	data.Branch = branch

	dirty, err := withTraceRegion(ctx, "git dirty", func() (bool, error) {
		return gitutil.Dirty(wt.Path)
	})
	if err != nil {
		return nil, err
	}
	data.Dirty = dirty

	if branch != "" {
		stash, err := withTraceRegion(ctx, "git stash", func() (bool, error) {
			return gitutil.HasBranchStash(wt.Path, branch)
		})
		if err != nil {
			return nil, err
		}
		data.HasStash = stash
	}

	operation, _ := withTraceRegion(ctx, "git operation", func() (string, error) {
		return gitutil.WorktreeOperation(wt.Path)
	})
	data.Operation = operation

	ahead, behind, err := func() (int, int, error) {
		type aheadBehind struct {
			ahead  int
			behind int
		}
		out, err := withTraceRegion(ctx, "git ahead/behind upstream", func() (aheadBehind, error) {
			ahead, behind, err := gitutil.AheadBehind(wt.Path, branch)
			return aheadBehind{ahead: ahead, behind: behind}, err
		})
		return out.ahead, out.behind, err
	}()
	if err != nil {
		if operation == "" && !isDetachedHeadError(err) {
			return nil, err
		}
		ahead, behind = 0, 0
	}
	data.Ahead = ahead
	data.Behind = behind

	ts, err := withTraceRegion(ctx, "git head timestamp", func() (time.Time, error) {
		return gitutil.HeadTimestamp(wt.Path)
	})
	if err != nil {
		return nil, err
	}
	if dirty {
		dirtyTS, derr := withTraceRegion(ctx, "git latest dirty timestamp", func() (time.Time, error) {
			return gitutil.LatestDirtyTimestamp(wt.Path)
		})
		if derr == nil {
			ts = dirtyTS
		}
	}
	data.Timestamp = ts

	baseAhead, baseBehind, err := func() (int, int, error) {
		type aheadBehind struct {
			ahead  int
			behind int
		}
		out, err := withTraceRegion(ctx, "git ahead/behind default", func() (aheadBehind, error) {
			ahead, behind, err := gitutil.AheadBehindDefaultBranch(wt.Path, proj.Config.DefaultBranch)
			return aheadBehind{ahead: ahead, behind: behind}, err
		})
		return out.ahead, out.behind, err
	}()
	if err != nil {
		return nil, err
	}
	data.BaseAhead = baseAhead
	data.BaseBehind = baseBehind

	headHash, err := withTraceRegion(ctx, "git head hash", func() (string, error) {
		return gitutil.Run(wt.Path, "rev-parse", "HEAD")
	})
	if err != nil {
		return nil, err
	}
	data.HeadHash = headHash

	compareRef := defaultCompareRef
	if compareRef == "" {
		compareRef = proj.Config.DefaultBranch
	}

	merged, err := withTraceRegion(ctx, "git head merged into default", func() (bool, error) {
		return gitutil.HeadMergedInto(wt.Path, compareRef)
	})
	if err != nil {
		return nil, err
	}
	data.MergedIntoDefault = merged

	treeMatches, err := withTraceRegion(ctx, "git head tree matches default", func() (bool, error) {
		return gitutil.HeadSameTree(wt.Path, compareRef)
	})
	if err != nil {
		return nil, err
	}
	data.TreeMatchesDefault = treeMatches

	uniqueAhead, err := withTraceRegion(ctx, "git unique commits", func() (int, error) {
		return gitutil.UniqueCommitsComparedTo(wt.Path, compareRef)
	})
	if err != nil {
		return nil, err
	}
	data.UniqueAhead = uniqueAhead

	if proj.DefaultWorktreePath != "" {
		remoteHash, exists, err := func() (string, bool, error) {
			type remoteBranch struct {
				hash   string
				exists bool
			}
			out, err := withTraceRegion(ctx, "git remote branch head", func() (remoteBranch, error) {
				hash, exists, err := gitutil.RemoteBranchHead(proj.DefaultWorktreePath, "origin", branch)
				return remoteBranch{hash: hash, exists: exists}, err
			})
			return out.hash, out.exists, err
		}()
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
