package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/brandonbloom/wt/internal/gitutil"
	"github.com/brandonbloom/wt/internal/project"
	"github.com/brandonbloom/wt/internal/shellbridge"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the current repository for wt",
		RunE:  runInit,
	}
	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	return initializeInDirectory(cmd, wd)
}

func initializeInDirectory(cmd *cobra.Command, dir string) error {
	if handled, err := tryInitializeExistingLayout(cmd, dir); err != nil {
		return err
	} else if handled {
		return nil
	}

	repoRoot, err := gitutil.Run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("determine git root: %w", err)
	}
	repoRoot = filepath.Clean(repoRoot)
	branch, err := gitutil.CurrentBranch(repoRoot)
	if err != nil {
		return err
	}

	parent := filepath.Dir(repoRoot)
	if looksConverted(parent) {
		return finalizeExistingLayout(cmd, parent, branch)
	}

	if branch != "main" && branch != "master" {
		return fmt.Errorf("default branch must be main or master (current: %s)", branch)
	}

	projectRoot, err := convertLegacyRepo(repoRoot, branch)
	if err != nil {
		return err
	}

	if _, err := project.EnsureConfig(projectRoot, branch); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Converted repository to wt layout at %s\n", projectRoot)
	target := filepath.Join(projectRoot, branch)
	if err := shellbridge.ChangeDirectory(target); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Please cd into %s\n", target)
	}
	return nil
}

func tryInitializeExistingLayout(cmd *cobra.Command, dir string) (bool, error) {
	defaultBranch, _, err := project.DetectDefaultWorktree(dir)
	if err != nil {
		if errors.Is(err, project.ErrDefaultWorktreeMissing) {
			return false, nil
		}
		return false, err
	}
	if err := finalizeExistingLayout(cmd, dir, defaultBranch); err != nil {
		return false, err
	}
	return true, nil
}

func finalizeExistingLayout(cmd *cobra.Command, root, defaultBranch string) error {
	configExisted := wtConfigExists(root)
	if _, err := project.EnsureConfig(root, defaultBranch); err != nil {
		return err
	}
	if configExisted {
		fmt.Fprintf(cmd.OutOrStdout(), "wt already initialized at %s\n", root)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Initialized wt metadata at %s\n", root)
	target := filepath.Join(root, defaultBranch)
	if err := shellbridge.ChangeDirectory(target); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Please cd into %s\n", target)
	}
	return nil
}

func wtConfigExists(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".wt", "config.toml"))
	return err == nil
}

func looksConverted(dir string) bool {
	_, err := project.Load(dir)
	return err == nil
}

func convertLegacyRepo(repoRoot, branch string) (string, error) {
	parent := filepath.Dir(repoRoot)
	projectName := filepath.Base(repoRoot)
	tempName := fmt.Sprintf("%s-%s", projectName, branch)
	tempPath := filepath.Join(parent, tempName)
	if exists(tempPath) {
		return "", fmt.Errorf("temporary path already exists: %s", tempPath)
	}
	if err := os.Rename(repoRoot, tempPath); err != nil {
		return "", err
	}
	projectRoot := filepath.Join(parent, projectName)
	if err := os.Mkdir(projectRoot, 0o755); err != nil {
		_ = os.Rename(tempPath, repoRoot)
		return "", err
	}
	branchPath := filepath.Join(projectRoot, branch)
	if exists(branchPath) {
		_ = os.Remove(projectRoot)
		_ = os.Rename(tempPath, repoRoot)
		return "", fmt.Errorf("target branch directory already exists: %s", branchPath)
	}
	if err := os.Rename(tempPath, branchPath); err != nil {
		_ = os.Remove(projectRoot)
		_ = os.Rename(tempPath, repoRoot)
		return "", err
	}
	return projectRoot, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
