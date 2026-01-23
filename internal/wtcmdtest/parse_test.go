package main

import "testing"

func TestParseArgs_SupportsFlagsAndCommandWithoutDashDash(t *testing.T) {
	opts, cmd, err := parseArgs([]string{
		"--skip-init",
		"--activate-wrapper",
		"--worktree", "main",
		"bash", "-lc", "echo hi",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !opts.skipInit {
		t.Fatalf("expected skipInit true")
	}
	if !opts.activateWrapper {
		t.Fatalf("expected activateWrapper true")
	}
	if opts.worktree != "main" {
		t.Fatalf("expected worktree=main, got %q", opts.worktree)
	}
	if len(cmd) != 3 || cmd[0] != "bash" || cmd[1] != "-lc" || cmd[2] != "echo hi" {
		t.Fatalf("unexpected command: %#v", cmd)
	}
}

func TestParseArgs_SupportsDashDashDelimiter(t *testing.T) {
	opts, cmd, err := parseArgs([]string{"--keep", "--", "bash", "-lc", "echo hi"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !opts.keepRepo {
		t.Fatalf("expected keepRepo true")
	}
	if len(cmd) != 3 || cmd[0] != "bash" || cmd[1] != "-lc" || cmd[2] != "echo hi" {
		t.Fatalf("unexpected command: %#v", cmd)
	}
}

func TestParseArgs_RequiresCommand(t *testing.T) {
	_, _, err := parseArgs([]string{"--keep"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseArgs_RejectsUnsafeWorktreePaths(t *testing.T) {
	if _, _, err := parseArgs([]string{"--worktree", "/abs", "bash", "-lc", "echo"}); err == nil {
		t.Fatalf("expected error for absolute worktree")
	}
	if _, _, err := parseArgs([]string{"--worktree", "../escape", "bash", "-lc", "echo"}); err == nil {
		t.Fatalf("expected error for worktree with ..")
	}
}
