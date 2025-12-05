package gitutil

import (
	"bytes"
	"errors"
	"fmt"
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

// AheadBehind counts commits relative to upstream. Missing upstream yields zeros.
func AheadBehind(dir string) (ahead, behind int, err error) {
	out, err := Run(dir, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	if err != nil {
		if strings.Contains(err.Error(), "no upstream") {
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
