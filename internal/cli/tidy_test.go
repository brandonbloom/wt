package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brandonbloom/wt/internal/project"
)

func TestDescribePRSummarySuppressesWhenNoUniqueCommits(t *testing.T) {
	cand := &tidyCandidate{
		UniqueAhead: 0,
		Dirty:       false,
		HasStash:    false,
		PRs: []pullRequestInfo{
			{Number: 107, State: "MERGED"},
		},
	}
	if got := describePRSummary(cand); got != "none" {
		t.Fatalf("expected summary to be suppressed, got %q", got)
	}
}

func TestDescribePRSummaryShowsPRsWhenUniqueCommitsRemain(t *testing.T) {
	cand := &tidyCandidate{
		UniqueAhead: 1,
		PRs: []pullRequestInfo{
			{Number: 92, State: "OPEN"},
		},
	}
	got := describePRSummary(cand)
	if !strings.Contains(got, "#92") {
		t.Fatalf("expected summary to mention PR #92, got %q", got)
	}
}

func TestPromptForCandidateShowsRecentCommitsGraph(t *testing.T) {
	disablePromptColors(t)

	repo := t.TempDir()
	cand := initPromptTestCandidate(t, repo)

	reader := bufio.NewReader(strings.NewReader("n\n"))
	var out bytes.Buffer
	if _, _, _, err := promptForCandidate(&out, reader, cand, time.Now(), true); err != nil {
		t.Fatalf("promptForCandidate: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Recent commits:") {
		t.Fatalf("expected prompt to include recent commits label, got:\n%s", output)
	}
	if !strings.Contains(output, "feature polish") {
		t.Fatalf("expected prompt to include latest commit message, got:\n%s", output)
	}
}

func TestPromptForCandidateSkipsCommitGraphWhenNotInteractive(t *testing.T) {
	repo := t.TempDir()
	cand := initPromptTestCandidate(t, repo)

	reader := bufio.NewReader(strings.NewReader("n\n"))
	var out bytes.Buffer
	if _, _, _, err := promptForCandidate(&out, reader, cand, time.Now(), false); err != nil {
		t.Fatalf("promptForCandidate: %v", err)
	}
	if strings.Contains(out.String(), "Recent commits:") {
		t.Fatalf("expected prompt to omit commit graph when not interactive:\n%s", out.String())
	}
}

func initPromptTestCandidate(t *testing.T, repo string) *tidyCandidate {
	runGitCmd(t, repo, "init", "-b", "main")
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "initial commit")

	runGitCmd(t, repo, "checkout", "-b", "feature/prompts")
	writeFile(t, filepath.Join(repo, "notes.txt"), "first\n")
	runGitCmd(t, repo, "add", "notes.txt")
	runGitCmd(t, repo, "commit", "-m", "feature draft")

	writeFile(t, filepath.Join(repo, "notes.txt"), "first\nsecond\n")
	runGitCmd(t, repo, "add", "notes.txt")
	runGitCmd(t, repo, "commit", "-m", "feature polish")

	return &tidyCandidate{
		Worktree:       project.Worktree{Name: "feature/prompts", Path: repo},
		Branch:         "feature/prompts",
		BaseAhead:      2,
		BaseBehind:     0,
		defaultBranch:  "main",
		Classification: tidyGray,
		GrayReasons:    []string{"stale"},
	}
}

func disablePromptColors(t *testing.T) {
	t.Helper()
	plain := func(a ...interface{}) string {
		return fmt.Sprint(a...)
	}
	restore := func(orig *func(a ...interface{}) string, replacement func(a ...interface{}) string) func() {
		prev := *orig
		*orig = replacement
		return func() {
			*orig = prev
		}
	}
	cleanups := []func(){
		restore(&colorPromptTitle, plain),
		restore(&colorPromptDivider, plain),
		restore(&colorPromptLabel, plain),
		restore(&colorPromptValue, plain),
		restore(&colorPromptReason, plain),
		restore(&colorPromptWarn, plain),
		restore(&colorPromptGood, plain),
	}
	t.Cleanup(func() {
		for _, fn := range cleanups {
			fn()
		}
	})
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	env := append([]string{}, os.Environ()...)
	ident := []string{
		"GIT_AUTHOR_NAME=wt-test",
		"GIT_AUTHOR_EMAIL=wt@example.com",
		"GIT_COMMITTER_NAME=wt-test",
		"GIT_COMMITTER_EMAIL=wt@example.com",
	}
	env = append(env, ident...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writeFile(%s): %v", path, err)
	}
}
