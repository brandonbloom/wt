package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newBootstrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Re-run the configured bootstrap script in the current worktree",
		Args:  cobra.NoArgs,
		RunE:  runBootstrapCmd,
	}
	cmd.Flags().Bool("strict", false, "force strict mode (set -euo pipefail) for the bootstrap script")
	cmd.Flags().Bool("no-strict", false, "disable strict mode even if enabled in config")
	cmd.Flags().BoolP("xtrace", "x", false, "print each bootstrap command as it runs (set -x)")
	return cmd
}

func runBootstrapCmd(cmd *cobra.Command, args []string) error {
	proj, err := loadProjectFromWD()
	if err != nil {
		return err
	}

	script := strings.TrimSpace(proj.Config.Bootstrap.Run)
	if script == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "No bootstrap command configured; edit .wt/config.toml to set [bootstrap].run.")
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	worktreeRoot, err := locateWorktreeRoot(wd, proj.Root)
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	strict := proj.Config.Bootstrap.StrictEnabled()
	if flags.Changed("strict") && flags.Changed("no-strict") {
		return fmt.Errorf("cannot use --strict and --no-strict together")
	}
	if flags.Changed("strict") {
		strict = true
	} else if flags.Changed("no-strict") {
		strict = false
	}
	xtrace, err := flags.GetBool("xtrace")
	if err != nil {
		return err
	}

	if err := runBootstrap(cmd, script, worktreeRoot, bootstrapOptions{
		strict: strict,
		xtrace: xtrace,
	}); err != nil {
		return err
	}

	return nil
}

func locateWorktreeRoot(start, projectRoot string) (string, error) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}

	for {
		if hasGitMetadata(cur) {
			return cur, nil
		}
		if samePath(cur, root) {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		if !isWithinProject(parent, root) {
			break
		}
		cur = parent
	}

	return "", errors.New("wt bootstrap must be run from inside a worktree (no .git directory found)")
}

func hasGitMetadata(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func isWithinProject(path, projectRoot string) bool {
	if samePath(path, projectRoot) {
		return true
	}
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, fmt.Sprintf("..%c", os.PathSeparator))
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
