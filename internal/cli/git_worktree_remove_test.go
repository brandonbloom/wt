package cli

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitWorktreeRemoveHandlesReadOnlyModuleCache(t *testing.T) {
	repo := initTempRepo(t)
	worktreePath := filepath.Join(repo, "feature")
	gitCmd(t, repo, "worktree", "add", worktreePath)

	createReadOnlyModuleCache(t, worktreePath)

	var buf bytes.Buffer
	if err := gitWorktreeRemove(repo, worktreePath, &buf); err != nil {
		t.Fatalf("gitWorktreeRemove failed: %v\noutput:\n%s", err, buf.String())
	}

	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected worktree to be removed, got err=%v", err)
	}
}

func initTempRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.email", "test@example.com")
	gitCmd(t, dir, "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	gitCmd(t, dir, "add", "README.md")
	gitCmd(t, dir, "commit", "-m", "init")

	return dir
}

func createReadOnlyModuleCache(t *testing.T, worktree string) {
	t.Helper()

	targetDir := filepath.Join(worktree, ".gopath/github.com/mattn/go-runewidth@v0.0.19/.github/workflows")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	filePath := filepath.Join(worktree, ".gopath/github.com/mattn/go-runewidth@v0.0.19/runewidth_windows.go")
	if err := os.WriteFile(filePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	makeReadOnly(t, filepath.Join(worktree, ".gopath"))
}

func makeReadOnly(t *testing.T, path string) {
	t.Helper()

	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		mode := info.Mode()
		if info.IsDir() {
			mode = (mode &^ 0o222) | 0o555
		} else {
			mode = (mode &^ 0o222) | 0o444
		}
		return os.Chmod(p, mode)
	})
	if err != nil {
		t.Fatalf("chmod readonly: %v", err)
	}
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()

	fullArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
}
