package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/brandonbloom/wt/internal/project"
	"github.com/brandonbloom/wt/internal/shellbridge"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type rmOptions struct {
	dryRun bool
	force  bool
}

func newRmCommand() *cobra.Command {
	opts := &rmOptions{}
	cmd := &cobra.Command{
		Use:   "rm [targets...]",
		Short: "Remove specific worktrees using wt tidy safety checks",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRm(cmd, opts, args)
		},
	}
	cmd.Flags().BoolVarP(&opts.dryRun, "dry-run", "n", false, "show actions without deleting anything")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "skip the confirmation prompt for gray worktrees")
	return cmd
}

func runRm(cmd *cobra.Command, opts *rmOptions, args []string) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI required: %w", err)
	}

	proj, err := loadProjectFromWD()
	if err != nil {
		return err
	}

	initialWD, err := os.Getwd()
	if err != nil {
		return err
	}

	worktrees, err := project.ListWorktrees(proj.Root)
	if err != nil {
		return err
	}

	targets, err := resolveRmTargets(worktrees, proj, args, initialWD)
	if err != nil {
		return err
	}

	now := currentTimeOverride()
	candidates, err := collectTidyCandidates(cmd.Context(), proj, now)
	if err != nil {
		return err
	}
	candidatesByName := make(map[string]*tidyCandidate, len(candidates))
	for _, c := range candidates {
		candidatesByName[c.Worktree.Name] = c
	}

	targetCands := make([]*tidyCandidate, 0, len(targets))
	for _, target := range targets {
		cand := candidatesByName[target.Name]
		if cand == nil {
			return fmt.Errorf("worktree %s is not removable", target.Name)
		}
		if cand.IsCurrent {
			if removeCurrent := removeBlockReason(cand, blockReasonCurrentWorktree); removeCurrent && len(cand.BlockReasons) == 0 {
				cand.Stage = tidyStageScanning
			}
		}
		targetCands = append(targetCands, cand)
	}

	if err := attachProcessesToCandidates(targetCands); err != nil {
		return err
	}
	for _, cand := range targetCands {
		if err := loadRmPullRequests(cmd.Context(), cand); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", singleLineError(err))
		}
		deriveClassification(cand, now)
		if cand.Classification == tidyBlocked {
			return fmt.Errorf("cannot remove %s: %s", cand.Worktree.Name, strings.Join(cand.BlockReasons, "; "))
		}
	}

	if opts.dryRun {
		return renderRmDryRun(cmd.OutOrStdout(), targetCands)
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	useColor := writerIsTerminal(cmd.OutOrStdout())

	logWriter := cmd.OutOrStdout()
	var relocated bool
	manualCd := false
	manualCdTarget := ""
	var remoteTouched bool

	for _, cand := range targetCands {
		if !relocated && initialWD != "" && isWithin(initialWD, cand.Worktree.Path) {
			relocated = true
			if err := shellbridge.ChangeDirectory(proj.Root); err != nil {
				manualCd = true
				manualCdTarget = cand.Worktree.Name
			}
			if err := os.Chdir(proj.Root); err != nil {
				return err
			}
		}

		if cand.Classification == tidyGray && !opts.force {
			proceed, quit, _, err := promptForCandidate(cmd.OutOrStdout(), reader, cand, now, useColor)
			if err != nil {
				return err
			}
			if !proceed {
				reason := "declined"
				if quit {
					reason = "quit selected"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Skipped %s: %s\n", cand.Worktree.Name, reason)
				if quit {
					return nil
				}
				continue
			}
		}

		touched, err := performCleanup(cmd.Context(), logWriter, proj, cand)
		if err != nil {
			return err
		}
		if touched {
			remoteTouched = true
		}
	}

	if remoteTouched {
		if err := pruneRemote(logWriter, proj.DefaultWorktreePath); err != nil {
			return err
		}
	}
	if manualCd {
		fmt.Fprintf(logWriter, "Removed %s; run `cd %s` to leave the deleted worktree\n", manualCdTarget, proj.Root)
	}
	return nil
}

func resolveRmTargets(worktrees []project.Worktree, proj *project.Project, args []string, wd string) ([]project.Worktree, error) {
	if len(args) == 0 {
		wt := findWorktreeContaining(worktrees, wd)
		if wt == nil {
			return nil, fmt.Errorf("not inside a worktree; specify a target")
		}
		if wt.Name == proj.DefaultWorktree {
			return nil, fmt.Errorf("cannot remove the default worktree (%s)", wt.Name)
		}
		return []project.Worktree{*wt}, nil
	}

	seen := make(map[string]bool, len(args))
	targets := make([]project.Worktree, 0, len(args))
	for _, arg := range args {
		var wt *project.Worktree
		if candidate := findWorktreeByName(worktrees, arg); candidate != nil {
			wt = candidate
		} else {
			found, err := findWorktreeByPath(worktrees, arg, wd)
			if err != nil {
				return nil, err
			}
			wt = found
		}
		if wt == nil {
			return nil, fmt.Errorf("no worktree matches %s", arg)
		}
		if wt.Name == proj.DefaultWorktree {
			return nil, fmt.Errorf("cannot remove the default worktree (%s)", wt.Name)
		}
		if seen[wt.Name] {
			continue
		}
		seen[wt.Name] = true
		targets = append(targets, *wt)
	}
	return targets, nil
}

func findWorktreeByName(worktrees []project.Worktree, name string) *project.Worktree {
	for _, wt := range worktrees {
		if wt.Name == name {
			copy := wt
			return &copy
		}
	}
	return nil
}

func findWorktreeContaining(worktrees []project.Worktree, path string) *project.Worktree {
	if path == "" {
		return nil
	}
	for _, wt := range worktrees {
		if isWithin(path, wt.Path) {
			copy := wt
			return &copy
		}
	}
	return nil
}

func findWorktreeByPath(worktrees []project.Worktree, arg, base string) (*project.Worktree, error) {
	path := arg
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, arg)
	}
	path = canonicalizePath(path)
	if path == "" {
		return nil, fmt.Errorf("invalid path %s", arg)
	}

	var match *project.Worktree
	for _, wt := range worktrees {
		root := canonicalizePath(wt.Path)
		if root == "" {
			continue
		}
		if isWithin(path, root) {
			if match != nil && match.Path != wt.Path {
				return nil, fmt.Errorf("path %s matches multiple worktrees (%s, %s)", arg, match.Name, wt.Name)
			}
			copy := wt
			match = &copy
		}
	}
	return match, nil
}

func loadRmPullRequests(ctx context.Context, cand *tidyCandidate) error {
	if len(cand.BlockReasons) > 0 {
		return nil
	}
	prs, err := queryPullRequests(ctx, cand.Worktree.Path, cand.Branch)
	if err != nil {
		cand.extraGrayReasons = append(cand.extraGrayReasons, fmt.Sprintf("PR lookup failed: %s", singleLineError(err)))
		return fmt.Errorf("%s: %w", cand.Worktree.Name, err)
	}
	cand.PRs = prs
	latest := cand.LastActivity
	for _, pr := range prs {
		if pr.UpdatedAt.After(latest) {
			latest = pr.UpdatedAt
		}
	}
	cand.LastActivity = latest
	return nil
}

func renderRmDryRun(out io.Writer, cands []*tidyCandidate) error {
	var needsRemote bool
	for i, cand := range cands {
		fmt.Fprintf(out, "Will clean up %s (branch %s)\n", cand.Worktree.Name, cand.Branch)
		for _, action := range plannedActions(cand) {
			fmt.Fprintf(out, "  - %s\n", action)
		}
		fmt.Fprintln(out)
		if cand.Classification == tidyGray {
			fmt.Fprintln(out, "Worktree requires confirmation:")
			for _, reason := range cand.GrayReasons {
				fmt.Fprintf(out, "  - %s\n", reason)
			}
			fmt.Fprintln(out)
		}
		if cand.HasRemoteBranch && cand.RemoteMatchesHead {
			needsRemote = true
		}
		if i < len(cands)-1 {
			fmt.Fprintln(out)
		}
	}
	if needsRemote {
		fmt.Fprintln(out, "Remote maintenance:")
		fmt.Fprintln(out, "- git remote prune origin")
	}
	return nil
}

func removeBlockReason(cand *tidyCandidate, target string) bool {
	if len(cand.BlockReasons) == 0 {
		return false
	}
	result := cand.BlockReasons[:0]
	removed := false
	for _, reason := range cand.BlockReasons {
		if !removed && reason == target {
			removed = true
			continue
		}
		result = append(result, reason)
	}
	if removed {
		cand.BlockReasons = result
	}
	return removed
}

func writerIsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
