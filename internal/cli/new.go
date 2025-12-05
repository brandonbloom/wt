package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/brandonbloom/wt/internal/gitutil"
	"github.com/brandonbloom/wt/internal/naming"
	"github.com/brandonbloom/wt/internal/project"
	"github.com/brandonbloom/wt/internal/shellbridge"
	"github.com/spf13/cobra"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,40}$`)

func newNewCommand() *cobra.Command {
	opts := &newOptions{}
	cmd := &cobra.Command{
		Use:   "new [<name>]",
		Short: "Create a new git worktree with a memorable name",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(cmd, opts, args)
		},
	}
	cmd.Flags().StringVar(&opts.base, "base", "", "base branch for new worktree")
	return cmd
}

type newOptions struct {
	base string
}

func runNew(cmd *cobra.Command, opts *newOptions, args []string) error {
	proj, err := loadProjectFromWD()
	if err != nil {
		return err
	}

	name := ""
	if len(args) == 1 {
		name = args[0]
	} else {
		name, err = naming.Generate()
		if err != nil {
			return fmt.Errorf("generate worktree name: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Selected worktree name %s\n", name)
	}

	if err := validateWorktreeName(name); err != nil {
		return err
	}

	targetPath := filepath.Join(proj.Root, name)
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("worktree %s already exists", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	baseBranch, err := determineBaseBranch(opts.base, proj)
	if err != nil {
		return err
	}

	if err := addWorktree(cmd, proj, name, baseBranch, targetPath); err != nil {
		return err
	}

	if err := runBootstrap(cmd, proj.Config.Bootstrap.Run, targetPath); err != nil {
		return err
	}

	if err := shellbridge.ChangeDirectory(targetPath); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s at %s (run `cd %s`)\n", name, targetPath, targetPath)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s at %s\n", name, targetPath)
	}

	return nil
}

func validateWorktreeName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid worktree name %q (use lowercase letters, digits, and hyphens)", name)
	}
	if name == "main" || name == "master" {
		return errors.New("main/master are reserved for the default worktree")
	}
	return nil
}

func determineBaseBranch(flag string, proj *project.Project) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if wd, err := os.Getwd(); err == nil {
		if branch, berr := gitutil.CurrentBranch(wd); berr == nil {
			return branch, nil
		}
	}
	if proj.Config.DefaultBranch != "" {
		return proj.Config.DefaultBranch, nil
	}
	if proj.DefaultWorktree != "" {
		return proj.DefaultWorktree, nil
	}
	return "", errors.New("unable to determine base branch; pass --base")
}

func addWorktree(cmd *cobra.Command, proj *project.Project, name, baseBranch, targetPath string) error {
	args := []string{"-C", proj.DefaultWorktreePath, "worktree", "add", targetPath, baseBranch}
	gitCmd := exec.Command("git", args...)
	gitCmd.Stdout = cmd.OutOrStdout()
	gitCmd.Stderr = cmd.ErrOrStderr()
	gitCmd.Stdin = os.Stdin
	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("git worktree add failed: %w", err)
	}
	return nil
}

func runBootstrap(cmd *cobra.Command, script, dir string) error {
	script = strings.TrimSpace(script)
	if script == "" {
		return nil
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	run := exec.Command(sh, "-c", script)
	run.Dir = dir
	run.Stdout = cmd.OutOrStdout()
	run.Stderr = cmd.ErrOrStderr()
	run.Stdin = os.Stdin
	if err := run.Run(); err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	return nil
}
