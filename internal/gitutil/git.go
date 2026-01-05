package gitutil

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Run executes git within dir and returns trimmed stdout.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RemoteURL returns the configured URL for the given remote name.
func RemoteURL(dir, remote string) (string, error) {
	if strings.TrimSpace(remote) == "" {
		remote = "origin"
	}
	if url, ok, err := gitConfigGet(dir, fmt.Sprintf("remote.%s.url", remote)); err != nil {
		return "", err
	} else if ok && strings.TrimSpace(url) != "" {
		return strings.TrimSpace(url), nil
	}

	// Fall back to `git remote get-url` for repositories that define remotes
	// in a non-standard way, even though it can be affected by url.insteadOf.
	out, err := Run(dir, "remote", "get-url", remote)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func gitConfigGet(dir, key string) (string, bool, error) {
	cmd := exec.Command("git", "-C", dir, "config", "--get", key)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("git config --get %s: %v\n%s", key, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), true, nil
}

// ParseGitHubRemote extracts owner/repo from a GitHub remote URL.
func ParseGitHubRemote(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", errors.New("empty remote URL")
	}
	trimmed = strings.TrimSuffix(trimmed, ".git")

	var host, path string
	switch {
	case strings.HasPrefix(trimmed, "git@"):
		parts := strings.SplitN(strings.TrimPrefix(trimmed, "git@"), ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid ssh remote: %s", raw)
		}
		host = parts[0]
		path = parts[1]
	case strings.HasPrefix(trimmed, "ssh://"):
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", "", err
		}
		host = u.Hostname()
		path = strings.TrimPrefix(u.Path, "/")
	case strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://"):
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", "", err
		}
		host = u.Hostname()
		path = strings.TrimPrefix(u.Path, "/")
	default:
		// treat as bare <host>:<path>?
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			host = parts[0]
			path = parts[1]
		} else {
			return "", "", fmt.Errorf("unsupported remote URL: %s", raw)
		}
	}

	if !strings.EqualFold(host, "github.com") {
		return "", "", fmt.Errorf("remote host %s is not github.com", host)
	}

	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return "", "", fmt.Errorf("invalid GitHub remote path: %s", path)
	}
	owner := segments[0]
	repo := segments[1]
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("invalid GitHub remote: %s", raw)
	}
	return owner, repo, nil
}

// CurrentBranch reports the checked-out branch name for a worktree.
func CurrentBranch(dir string) (string, error) {
	out, err := Run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}

// Dirty reports whether the worktree has uncommitted/staged changes.
func Dirty(dir string) (bool, error) {
	out, err := Run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// HasBranchStash reports whether any stash entries mention the given branch.
func HasBranchStash(dir, branch string) (bool, error) {
	out, err := Run(dir, "stash", "list")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(out) == "" || branch == "" {
		return false, nil
	}
	pattern := fmt.Sprintf("on %s:", branch)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, pattern) {
			return true, nil
		}
	}
	return false, nil
}

// AheadBehind counts commits relative to upstream. Missing upstream yields zeros.
func AheadBehind(dir, branch string) (ahead, behind int, err error) {
	if ahead, behind, ok, err := aheadBehindFromStatus(dir); err == nil && ok {
		return ahead, behind, nil
	}
	out, err := Run(dir, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	if err != nil {
		if strings.Contains(err.Error(), "no upstream") {
			if ahead, behind, ok, fbErr := aheadBehindFromRemote(dir, branch); fbErr == nil && ok {
				return ahead, behind, nil
			}
			return 0, 0, nil
		}
		return 0, 0, err
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output: %s", out)
	}
	behind, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, err
	}
	ahead, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// HeadTimestamp returns the timestamp of the HEAD commit.
func HeadTimestamp(dir string) (time.Time, error) {
	out, err := Run(dir, "log", "-1", "--format=%cI", "HEAD")
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339, out)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// LatestDirtyTimestamp approximates the newest mtime of files mentioned by git status.
func LatestDirtyTimestamp(dir string) (time.Time, error) {
	out, err := Run(dir, "status", "--porcelain", "-z")
	if err != nil {
		return time.Time{}, err
	}
	if out == "" {
		return time.Time{}, errors.New("worktree is clean")
	}
	var newest time.Time
	entries := strings.Split(out, "\x00")
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		if len(entry) < 4 {
			continue
		}
		path := strings.TrimSpace(entry[3:])
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
		return time.Time{}, errors.New("unable to find dirty files")
	}
	return newest, nil
}

// HeadMergedInto reports whether HEAD is already an ancestor of the given ref.
func HeadMergedInto(dir, ref string) (bool, error) {
	if ref == "" {
		return false, nil
	}
	cmd := exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", "HEAD", ref)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// HeadSameTree reports whether HEAD has the same tree as the given ref.
func HeadSameTree(dir, ref string) (bool, error) {
	if ref == "" {
		return false, nil
	}
	cmd := exec.Command("git", "-C", dir, "diff", "--quiet", "HEAD", ref, "--")
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// UniqueCommitsComparedTo counts commits reachable from HEAD whose changes are
// not present in the given ref (based on git-cherry's patch-id comparison).
func UniqueCommitsComparedTo(dir, ref string) (int, error) {
	if ref == "" {
		return 0, nil
	}
	out, err := Run(dir, "cherry", ref, "HEAD")
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(out) == "" {
		return 0, nil
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "+") {
			count++
		}
	}
	return count, nil
}

// WorktreeOperation inspects git metadata to determine if a high-level operation is in progress.
func WorktreeOperation(dir string) (string, error) {
	gitDir, err := Run(dir, "rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	checks := []struct {
		state string
		paths []string
	}{
		{state: "rebasing", paths: []string{"rebase-merge", "rebase-apply"}},
		{state: "merging", paths: []string{"MERGE_HEAD"}},
	}
	for _, check := range checks {
		for _, rel := range check.paths {
			if exists(filepath.Join(gitDir, rel)) {
				return check.state, nil
			}
		}
	}
	return "", nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func gitRefExists(dir, ref string) bool {
	cmd := exec.Command("git", "-C", dir, "show-ref", "--verify", "--quiet", ref)
	return cmd.Run() == nil
}

func aheadBehindFromStatus(dir string) (ahead, behind int, ok bool, err error) {
	out, err := Run(dir, "status", "--porcelain=2", "--branch")
	if err != nil {
		return 0, 0, false, err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# branch.ab") {
			var plus, minus int
			_, scanErr := fmt.Sscanf(line, "# branch.ab +%d -%d", &plus, &minus)
			if scanErr != nil {
				return 0, 0, false, scanErr
			}
			return plus, minus, true, nil
		}
	}
	return 0, 0, false, nil
}

func aheadBehindFromRemote(dir, branch string) (ahead, behind int, ok bool, err error) {
	if branch == "" {
		return 0, 0, false, nil
	}
	remote, err := Run(dir, "config", "--get", fmt.Sprintf("branch.%s.remote", branch))
	if err != nil || remote == "" {
		remote = "origin"
	}
	remoteRef := fmt.Sprintf("%s/%s", remote, branch)
	fullRef := fmt.Sprintf("refs/remotes/%s/%s", remote, branch)
	if !gitRefExists(dir, fullRef) {
		return 0, 0, false, nil
	}
	ahead, behind, err = aheadBehindAgainstRef(dir, remoteRef)
	if err != nil {
		return 0, 0, false, err
	}
	return ahead, behind, true, nil
}

// AheadBehindDefaultBranch compares HEAD to origin/<defaultBranch>.
func AheadBehindDefaultBranch(dir, defaultBranch string) (ahead, behind int, err error) {
	if defaultBranch == "" {
		return 0, 0, nil
	}
	remote := "origin"
	fullRef := fmt.Sprintf("refs/remotes/%s/%s", remote, defaultBranch)
	if !gitRefExists(dir, fullRef) {
		return 0, 0, nil
	}
	return aheadBehindAgainstRef(dir, fmt.Sprintf("%s/%s", remote, defaultBranch))
}

type DefaultBranchSyncMode string

const (
	DefaultBranchLocalFirst  DefaultBranchSyncMode = "local"
	DefaultBranchRemoteFirst DefaultBranchSyncMode = "remote"
)

// DefaultBranchComparisonRef selects the ref that should act as the "default
// branch" for safety checks when a repository may be operated in either a local-
// first or remote-first workflow.
//
// Policy:
// - If refs/remotes/<remote>/<defaultBranch> is missing, return <defaultBranch>
//   (local-first).
// - If the local default branch is ahead of the remote-tracking default branch,
//   return <defaultBranch> (local-first).
// - Otherwise return <remote>/<defaultBranch> (remote-first).
func DefaultBranchComparisonRef(dir, remote, defaultBranch string) (string, DefaultBranchSyncMode, error) {
	defaultBranch = strings.TrimSpace(defaultBranch)
	if defaultBranch == "" {
		return "", DefaultBranchLocalFirst, nil
	}
	remote = strings.TrimSpace(remote)
	if remote == "" {
		remote = "origin"
	}

	remoteFullRef := fmt.Sprintf("refs/remotes/%s/%s", remote, defaultBranch)
	if !gitRefExists(dir, remoteFullRef) {
		return defaultBranch, DefaultBranchLocalFirst, nil
	}
	localFullRef := fmt.Sprintf("refs/heads/%s", defaultBranch)
	if !gitRefExists(dir, localFullRef) {
		return fmt.Sprintf("%s/%s", remote, defaultBranch), DefaultBranchRemoteFirst, nil
	}

	out, err := Run(dir, "rev-list", "--left-right", "--count", fmt.Sprintf("%s/%s...%s", remote, defaultBranch, defaultBranch))
	if err != nil {
		// If we can't determine which side is ahead, prefer the remote-first view
		// to avoid treating unpushed work as integrated by accident.
		return fmt.Sprintf("%s/%s", remote, defaultBranch), DefaultBranchRemoteFirst, nil
	}

	fields := strings.Fields(out)
	if len(fields) != 2 {
		return "", DefaultBranchLocalFirst, fmt.Errorf("unexpected rev-list output: %s", out)
	}
	localAhead, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", DefaultBranchLocalFirst, err
	}
	if localAhead > 0 {
		return defaultBranch, DefaultBranchLocalFirst, nil
	}
	return fmt.Sprintf("%s/%s", remote, defaultBranch), DefaultBranchRemoteFirst, nil
}

func aheadBehindAgainstRef(dir, ref string) (ahead, behind int, err error) {
	out, err := Run(dir, "rev-list", "--left-right", "--count", ref+"...HEAD")
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output: %s", out)
	}
	behind, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, err
	}
	ahead, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// RemoteBranchHead reports the current commit for remote/branch if it exists.
func RemoteBranchHead(dir, remote, branch string) (string, bool, error) {
	if remote == "" || branch == "" {
		return "", false, nil
	}
	ref := fmt.Sprintf("refs/remotes/%s/%s", remote, branch)
	if !gitRefExists(dir, ref) {
		return "", false, nil
	}
	out, err := Run(dir, "rev-parse", ref)
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(out), true, nil
}
