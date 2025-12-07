package cli

import (
	"fmt"
	"path/filepath"

	"github.com/brandonbloom/wt/internal/project"
)

func resolveWorktreeArgs(worktrees []project.Worktree, args []string, wd string) ([]project.Worktree, error) {
	seen := make(map[string]bool, len(args))
	targets := make([]project.Worktree, 0, len(args))
	for _, arg := range args {
		var wt *project.Worktree
		if candidate := findWorktreeByName(worktrees, arg); candidate != nil {
			wt = candidate
		} else {
			found, err := findWorktreeByPath(worktrees, arg, wd)
			if err != nil {
				return nil, err
			}
			wt = found
		}
		if wt == nil {
			return nil, fmt.Errorf("no worktree matches %s", arg)
		}
		if seen[wt.Name] {
			continue
		}
		seen[wt.Name] = true
		targets = append(targets, *wt)
	}
	return targets, nil
}

func findWorktreeByName(worktrees []project.Worktree, name string) *project.Worktree {
	for _, wt := range worktrees {
		if wt.Name == name {
			copy := wt
			return &copy
		}
	}
	return nil
}

func findWorktreeContaining(worktrees []project.Worktree, path string) *project.Worktree {
	if path == "" {
		return nil
	}
	for _, wt := range worktrees {
		if isWithin(path, wt.Path) {
			copy := wt
			return &copy
		}
	}
	return nil
}

func findWorktreeByPath(worktrees []project.Worktree, arg, base string) (*project.Worktree, error) {
	path := arg
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, arg)
	}
	path = canonicalizePath(path)
	if path == "" {
		return nil, fmt.Errorf("invalid path %s", arg)
	}

	var match *project.Worktree
	for _, wt := range worktrees {
		root := canonicalizePath(wt.Path)
		if root == "" {
			continue
		}
		if isWithin(path, root) {
			if match != nil && match.Path != wt.Path {
				return nil, fmt.Errorf("path %s matches multiple worktrees (%s, %s)", arg, match.Name, wt.Name)
			}
			copy := wt
			match = &copy
		}
	}
	return match, nil
}
