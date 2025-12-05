package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/brandonbloom/wt/internal/project"
	"github.com/brandonbloom/wt/internal/shellbridge"
	"github.com/spf13/cobra"
)

func newDoctorCommand() *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose wt prerequisites and environment issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd, verbose)
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show passing checks too")
	return cmd
}

type doctorContext struct {
	Project *project.Project
}

type doctorCheck struct {
	Name string
	Fn   func(*doctorContext) error
}

func runDoctor(cmd *cobra.Command, verbose bool) error {
	ctx := &doctorContext{}
	wd, _ := os.Getwd()
	checks := []doctorCheck{
		{Name: "git installed", Fn: requireOnPath("git")},
		{Name: "gh installed", Fn: requireOnPath("gh")},
		{Name: "gh authenticated", Fn: checkGhAuth},
		{Name: "project layout", Fn: func(c *doctorContext) error {
			proj, err := project.Discover(wd)
			if err != nil {
				return err
			}
			c.Project = proj
			return nil
		}},
		{Name: "default branch matches GitHub", Fn: checkDefaultBranch},
		{Name: "shell wrapper active", Fn: func(*doctorContext) error {
			if !shellbridge.Active() {
				return errors.New("shell wrapper inactive; add `eval \"$(wt activate)\"` to your shell")
			}
			return nil
		}},
	}

	var failures []string
	for _, check := range checks {
		err := check.Fn(ctx)
		if err != nil {
			failures = append(failures, fmt.Sprintf("✗ %s: %v", check.Name, err))
			continue
		}
		if verbose {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s\n", check.Name)
		}
	}

	if len(failures) > 0 {
		for _, failure := range failures {
			fmt.Fprintln(cmd.ErrOrStderr(), failure)
		}
		return fmt.Errorf("%d doctor checks failed", len(failures))
	}

	fmt.Fprintln(cmd.OutOrStdout(), "healthy!")
	return nil
}

func requireOnPath(binary string) func(*doctorContext) error {
	return func(*doctorContext) error {
		if _, err := exec.LookPath(binary); err != nil {
			return fmt.Errorf("%s not found on PATH", binary)
		}
		return nil
	}
}

func checkGhAuth(*doctorContext) error {
	cmd := exec.Command("gh", "auth", "status", "--exit-status")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func checkDefaultBranch(ctx *doctorContext) error {
	if ctx.Project == nil {
		return errors.New("project not initialized")
	}
	want := ctx.Project.Config.DefaultBranch
	if want == "" {
		return errors.New("default_branch missing from config")
	}
	cmd := exec.Command("gh", "repo", "view", "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	cmd.Dir = ctx.Project.DefaultWorktreePath
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("gh repo view: %w", err)
	}
	got := string(bytes.TrimSpace(output))
	if got != want {
		return fmt.Errorf("config default_branch=%s but GitHub reports %s", want, got)
	}
	return nil
}
