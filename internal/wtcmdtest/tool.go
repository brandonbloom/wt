// Implementation of the `wtcmdtest` harness.
//
// Key behaviors:
//   - Creates `/tmp/wt-transcripts/tmprepo-<id>` and symlinks `/tmp/wt-transcripts/bin -> <repo>/bin`.
//   - Installs a hermetic `gh` stub by copying `bin/wtghstub` into the temp repo as `bin/gh`.
//   - Seeds deterministic git author/commit timestamps for stable transcripts.
//   - Honors `WT_CMDTEST_TIMEOUT` (default 10s) to cap setup + command runtime.
//   - Honors `WT_CMDTEST_ID` to isolate temp repos for parallel tests.
//   - Honors `WT_SKIP_DEFAULT_ORIGIN=1` to omit the default `origin` remote.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type tool struct {
	repoRoot        string
	transcriptsRoot string
	ghStubBinary    string

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

const defaultTimeout = 10 * time.Second

func newToolFromExecutable() (*tool, error) {
	if root := os.Getenv("WT_REPO_ROOT"); root != "" {
		return newTool(root), nil
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, err
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(exe), ".."))
	return newTool(repoRoot), nil
}

func newTool(repoRoot string) *tool {
	repoRoot = filepath.Clean(repoRoot)
	return &tool{
		repoRoot:        repoRoot,
		transcriptsRoot: "/tmp/wt-transcripts",
		ghStubBinary:    filepath.Join(repoRoot, "bin", "wtghstub"),
		stdin:           os.Stdin,
		stdout:          os.Stdout,
		stderr:          os.Stderr,
	}
}

func (t *tool) runCLI(ctx context.Context, args []string) int {
	ctx, cancel, timeout := withTimeoutFromEnv(ctx, "WT_CMDTEST_TIMEOUT", defaultTimeout)
	if cancel != nil {
		defer cancel()
	}

	opts, cmdArgs, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(t.stderr, err)
		t.printUsage()
		return 2
	}
	if opts.help {
		t.printUsage()
		return 0
	}

	exitCode, err := t.run(ctx, opts, cmdArgs, timeout)
	if err != nil {
		fmt.Fprintln(t.stderr, err)
		return 1
	}
	return exitCode
}

func (t *tool) printUsage() {
	fmt.Fprint(t.stderr, `Usage: wtcmdtest [options] -- <command> [args...]

Sets up a disposable wt test project, runs the given command inside it,
and cleans up afterward. Intended for transcript integration tests.

Options:
  --skip-init          Leave the repository unconverted (for wt init tests).
  --activate-wrapper   Simulate the wt shell wrapper being active.
  --worktree DIR       cd into DIR (relative to the temp repo) before running.
  --keep               Preserve the temp repo for debugging (prints its path).
`)
}

func (t *tool) run(ctx context.Context, opts options, cmdArgs []string, timeout time.Duration) (int, error) {
	if t.repoRoot == "" {
		return 1, errors.New("repo root is required")
	}
	if _, err := os.Stat(filepath.Join(t.repoRoot, "go.mod")); err != nil {
		return 1, fmt.Errorf("unable to locate wt repo root: %w", err)
	}

	if err := os.MkdirAll(t.transcriptsRoot, 0o755); err != nil {
		return 1, err
	}

	if err := t.ensureBinSymlink(); err != nil {
		return 1, err
	}

	tmprepo := filepath.Join(t.transcriptsRoot, tmprepoDirName())
	if err := removeAllUnder(t.transcriptsRoot, tmprepo); err != nil {
		return 1, err
	}
	if err := os.MkdirAll(tmprepo, 0o755); err != nil {
		return 1, err
	}

	childEnv := deterministicEnv(os.Environ())

	if err := t.seedGitRepo(ctx, tmprepo, childEnv); err != nil {
		return 1, err
	}
	if !opts.skipInit {
		if err := t.runQuiet(ctx, tmprepo, childEnv, filepath.Join(t.repoRoot, "bin", "wt"), "init"); err != nil {
			return 1, err
		}
	}
	if err := t.installGHStub(tmprepo); err != nil {
		return 1, err
	}

	childEnv = withEnv(childEnv, "WT_GH_STATE_FILE", filepath.Join(tmprepo, ".gh-prs"))
	childEnv = withEnv(childEnv, "WT_GH_CI_FILE", filepath.Join(tmprepo, ".gh-ci"))
	childEnv = withEnv(childEnv, "PATH", filepath.Join(tmprepo, "bin")+string(os.PathListSeparator)+getEnv(childEnv, "PATH"))

	if opts.activateWrapper {
		childEnv = withEnv(childEnv, "WT_WRAPPER_ACTIVE", "1")
		childEnv = withEnv(childEnv, "WT_INSTRUCTION_FILE", filepath.Join(tmprepo, ".wt-instruction"))
	}

	workdir := tmprepo
	if opts.worktree != "" {
		workdir = filepath.Join(tmprepo, opts.worktree)
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = workdir
	cmd.Env = withEnv(childEnv, "PWD", workdir)
	cmd.Stdin = t.stdin
	cmd.Stdout = t.stdout
	cmd.Stderr = t.stderr

	runErr := cmd.Run()
	if runErr != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return 124, fmt.Errorf("wtcmdtest: timed out after %s", timeout)
	}
	exitCode := exitStatus(runErr)

	if opts.keepRepo {
		fmt.Fprintf(t.stderr, "temp repo kept at %s\n", tmprepo)
	} else if cleanupErr := removeAllUnder(t.transcriptsRoot, tmprepo); cleanupErr != nil {
		return 1, cleanupErr
	}

	return exitCode, nil
}

func (t *tool) ensureBinSymlink() error {
	dst := filepath.Join(t.transcriptsRoot, "bin")
	src := filepath.Join(t.repoRoot, "bin")

	if info, err := os.Lstat(dst); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refusing to overwrite non-symlink: %s", dst)
		}
		if target, err := os.Readlink(dst); err == nil {
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(dst), target)
			}
			if filepath.Clean(target) == src {
				return nil
			}
		}
		return fmt.Errorf("symlink %s points somewhere else; remove it to continue", dst)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Symlink(src, dst); err != nil {
		if errors.Is(err, os.ErrExist) {
			if target, err := os.Readlink(dst); err == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(dst), target)
				}
				if filepath.Clean(target) == src {
					return nil
				}
			}
		}
		return err
	}
	return nil
}

func (t *tool) seedGitRepo(ctx context.Context, dir string, env []string) error {
	if err := t.runQuiet(ctx, dir, env, "git", "init", "-b", "main"); err != nil {
		return err
	}
	if os.Getenv("WT_SKIP_DEFAULT_ORIGIN") != "1" {
		if err := t.runQuiet(ctx, dir, env, "git", "remote", "add", "origin", "git@github.com:brandonbloom/wt.git"); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		return err
	}
	if err := t.runQuiet(ctx, dir, env, "git", "add", "README.md"); err != nil {
		return err
	}
	if err := t.runQuiet(ctx, dir, env, "git", "commit", "-m", "init"); err != nil {
		return err
	}
	return nil
}

func (t *tool) installGHStub(tmprepo string) error {
	ghStateFile := filepath.Join(tmprepo, ".gh-prs")
	ciStateFile := filepath.Join(tmprepo, ".gh-ci")

	if err := os.WriteFile(ghStateFile, []byte(strings.Join([]string{
		"demo-branch|42|OPEN|false|2000-01-02T00:00:00Z|https://example.com/pr/42",
		"merged-branch|99|MERGED|false|2000-01-02T00:00:00Z|https://example.com/pr/99",
		"",
	}, "\n")), 0o644); err != nil {
		return err
	}

	if err := os.WriteFile(ciStateFile, []byte(strings.Join([]string{
		"commit|*|build|completed|success|https://example.com/run/success|2000-01-02T00:00:00Z|2000-01-02T00:05:00Z",
		"pr|42|Pull Request Checks|completed|failure|https://example.com/run/pr-42|2000-01-02T23:59:00Z|2000-01-02T23:59:59Z",
		"",
	}, "\n")), 0o644); err != nil {
		return err
	}

	stub, err := os.ReadFile(t.ghStubBinary)
	if err != nil {
		return err
	}

	binDir := filepath.Join(tmprepo, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}

	ghPath := filepath.Join(binDir, "gh")
	if err := os.WriteFile(ghPath, stub, 0o755); err != nil {
		return err
	}
	return nil
}

func (t *tool) runQuiet(ctx context.Context, dir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = withEnv(env, "PWD", dir)

	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			msg = ": " + msg
		}
		return fmt.Errorf("%s %s failed%s: %w", name, strings.Join(args, " "), msg, err)
	}
	return nil
}

func deterministicEnv(base []string) []string {
	env := envMap(base)
	env["GIT_AUTHOR_NAME"] = "wt-test"
	env["GIT_AUTHOR_EMAIL"] = "wt@example.com"
	env["GIT_COMMITTER_NAME"] = "wt-test"
	env["GIT_COMMITTER_EMAIL"] = "wt@example.com"
	env["GIT_AUTHOR_DATE"] = "2000-01-01T00:00:00Z"
	env["GIT_COMMITTER_DATE"] = "2000-01-01T00:00:00Z"
	env["NO_COLOR"] = "1"
	env["CLICOLOR"] = "0"
	env["CLICOLOR_FORCE"] = "0"
	return envSlice(env)
}

func removeAllUnder(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == "." {
		return fmt.Errorf("refusing to remove root: %s", root)
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("refusing to remove outside root: %s", target)
	}
	return os.RemoveAll(target)
}

func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 127
}

func withTimeoutFromEnv(ctx context.Context, key string, def time.Duration) (context.Context, context.CancelFunc, time.Duration) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		raw = def.String()
	}
	if raw == "0" || raw == "0s" {
		return ctx, nil, 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		d = def
	}
	next, cancel := context.WithTimeout(ctx, d)
	return next, cancel, d
}

func envMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		out[key] = value
	}
	return out
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func withEnv(env []string, key, value string) []string {
	m := envMap(env)
	m[key] = value
	return envSlice(m)
}

func getEnv(env []string, key string) string {
	m := envMap(env)
	return m[key]
}

func tmprepoDirName() string {
	raw := strings.TrimSpace(os.Getenv("WT_CMDTEST_ID"))
	if raw != "" {
		safe := make([]rune, 0, len(raw))
		for _, r := range raw {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
				safe = append(safe, r)
				continue
			}
			safe = append(safe, '_')
		}
		id := strings.Trim(strings.TrimSpace(string(safe)), "._-")
		if id != "" {
			return "tmprepo-" + id
		}
	}

	// Fallback: generate a unique, non-guessable ID to avoid collisions in `/tmp`.
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("tmprepo-%d", os.Getpid())
	}
	return "tmprepo-" + hex.EncodeToString(b[:])
}
