package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

type gatherWorktreeGitDataOptions struct {
	IncludeUniqueCommits bool
	IncludeMergeState    bool
	IncludeTreeMatch     bool
	IncludeRemoteInfo    bool
	StashBranches        map[string]bool
}

var gatherWorktreeGitDataOptionsStatus = gatherWorktreeGitDataOptions{
	IncludeUniqueCommits: true,
	IncludeMergeState:    false,
	IncludeTreeMatch:     false,
	IncludeRemoteInfo:    false,
}

var gatherWorktreeGitDataOptionsFull = gatherWorktreeGitDataOptions{
	IncludeUniqueCommits: true,
	IncludeMergeState:    true,
	IncludeTreeMatch:     true,
	IncludeRemoteInfo:    true,
}

func gatherWorktreeGitData(ctx context.Context, proj *project.Project, wt project.Worktree, defaultCompareRef string, opts gatherWorktreeGitDataOptions) (*worktreeGitData, error) {
	data := &worktreeGitData{Worktree: wt}

	status, err := withTraceRegion(ctx, "git status", func() (gitutil.StatusSummary, error) {
		return gitutil.Status(wt.Path)
	})
	if err != nil {
		return nil, err
	}
	data.Branch = status.Head
	data.HeadHash = status.HeadOID

	data.Dirty = status.HasChanges

	if data.Branch != "" {
		stash, err := withTraceRegion(ctx, "git stash", func() (bool, error) {
			if opts.StashBranches != nil {
				return opts.StashBranches[data.Branch], nil
			}
			return gitutil.HasBranchStash(wt.Path, data.Branch)
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

	data.Ahead = status.Ahead
	data.Behind = status.Behind

	ts, err := withTraceRegion(ctx, "git head timestamp", func() (time.Time, error) {
		return gitutil.HeadTimestamp(wt.Path)
	})
	if err != nil {
		return nil, err
	}
	if data.Dirty {
		dirtyTS, derr := withTraceRegion(ctx, "dirty mtime", func() (time.Time, error) {
			return latestMTime(wt.Path, status.Paths)
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

	compareRef := defaultCompareRef
	if compareRef == "" {
		compareRef = proj.Config.DefaultBranch
	}

	if opts.IncludeMergeState {
		merged, err := withTraceRegion(ctx, "git head merged into default", func() (bool, error) {
			return gitutil.HeadMergedInto(wt.Path, compareRef)
		})
		if err != nil {
			return nil, err
		}
		data.MergedIntoDefault = merged
	}

	if opts.IncludeTreeMatch {
		treeMatches, err := withTraceRegion(ctx, "git head tree matches default", func() (bool, error) {
			return gitutil.HeadSameTree(wt.Path, compareRef)
		})
		if err != nil {
			return nil, err
		}
		data.TreeMatchesDefault = treeMatches
	}

	if opts.IncludeUniqueCommits {
		uniqueAhead, err := withTraceRegion(ctx, "git unique commits", func() (int, error) {
			return gitutil.UniqueCommitsComparedTo(wt.Path, compareRef)
		})
		if err != nil {
			return nil, err
		}
		data.UniqueAhead = uniqueAhead
	}

	if opts.IncludeRemoteInfo && proj.DefaultWorktreePath != "" {
		remoteHash, exists, err := func() (string, bool, error) {
			type remoteBranch struct {
				hash   string
				exists bool
			}
			out, err := withTraceRegion(ctx, "git remote branch head", func() (remoteBranch, error) {
				hash, exists, err := gitutil.RemoteBranchHead(proj.DefaultWorktreePath, "origin", data.Branch)
				return remoteBranch{hash: hash, exists: exists}, err
			})
			return out.hash, out.exists, err
		}()
		if err != nil {
			return nil, err
		}
		data.HasRemoteBranch = exists
		if exists {
			data.RemoteMatchesHead = remoteHash == data.HeadHash
		}
	}

	return data, nil
}

func latestMTime(dir string, paths []string) (time.Time, error) {
	if dir == "" {
		return time.Time{}, errors.New("empty dir")
	}
	if len(paths) == 0 {
		return time.Time{}, errors.New("no paths")
	}
	var newest time.Time
	for _, path := range paths {
		if path == "" {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, path))
		if err != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if newest.IsZero() {
		return time.Time{}, errors.New("unable to stat paths")
	}
	return newest, nil
}
