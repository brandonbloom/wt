package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newCloneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone <url> [<dest>]",
		Short: "Clone a repository and automatically initialize wt",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runClone,
	}
	return cmd
}

func runClone(cmd *cobra.Command, args []string) error {
	url := args[0]
	var dest string
	if len(args) == 2 {
		dest = args[1]
	}

	cloneArgs := []string{"clone", url}
	if dest != "" {
		cloneArgs = append(cloneArgs, dest)
	}
	git := exec.Command("git", cloneArgs...)
	git.Stdout = cmd.OutOrStdout()
	git.Stderr = cmd.ErrOrStderr()
	git.Stdin = os.Stdin
	if err := git.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	targetDir := dest
	if targetDir == "" {
		targetDir = deriveCloneDir(url)
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}
	return initializeInDirectory(cmd, absTarget)
}

func deriveCloneDir(url string) string {
	base := path.Base(url)
	if strings.HasSuffix(base, ".git") {
		base = strings.TrimSuffix(base, ".git")
	}
	base = strings.TrimSuffix(base, "/")
	return base
}
