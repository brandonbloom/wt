package cli

import (
	"bufio"
	"context"
	"errors"
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
	compareCtx := defaultBranchComparisonContext(proj)
	workflow := workflowExpectationsForProject(compareCtx)
	ciRepo, ciRepoErr := resolveGitHubRepo(proj)

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
	candidates, err := collectTidyCandidates(cmd.Context(), proj, compareCtx.CompareRef, now)
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

	forcedReasons := make(map[string][]string)
	if opts.force {
		for _, cand := range targetCands {
			if cand == nil || len(cand.BlockReasons) == 0 {
				continue
			}
			forcedReasons[cand.Worktree.Name] = append([]string(nil), cand.BlockReasons...)
			cand.BlockReasons = nil
			cand.Stage = tidyStageScanning
		}
	}

	if err := attachProcessesToCandidates(targetCands); err != nil {
		return err
	}
	for _, cand := range targetCands {
		if err := loadRmPullRequests(cmd.Context(), cand); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", singleLineError(err))
		}
	}

	statuses := make([]*worktreeStatus, len(targetCands))
	for i, cand := range targetCands {
		statuses[i] = candidateToStatus(cand, now)
		cand.status = statuses[i]
	}
	ciOpts := ciFetchOptions{
		Repo:       ciRepo,
		RepoErr:    ciRepoErr,
		RemoteName: proj.Config.CIRemote(),
		Workdir:    proj.DefaultWorktreePath,
	}
	if err := fetchCIStatuses(cmd.Context(), ciOpts, statuses, now, nil); err != nil && errors.Is(err, context.Canceled) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", singleLineError(err))
	}
	updateCandidatesCIState(targetCands, workflow)

	for _, cand := range targetCands {
		deriveClassification(cand, tidyDeriveContext{Now: now, Workflow: workflow})
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

		if opts.force {
			if reasons := forcedReasons[cand.Worktree.Name]; len(reasons) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: forcing removal of %s: %s\n", cand.Worktree.Name, strings.Join(reasons, "; "))
			}
		}

		touched, err := performRmCleanup(cmd.Context(), cmd.ErrOrStderr(), logWriter, proj, cand, opts.force)
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

func performRmCleanup(ctx context.Context, warn io.Writer, log io.Writer, proj *project.Project, cand *tidyCandidate, force bool) (bool, error) {
	if cand == nil {
		return false, nil
	}
	if log != nil {
		fmt.Fprintf(log, "Cleaning %s (branch %s)\n", cand.Worktree.Name, cand.Branch)
	}

	err := gitWorktreeRemove(proj.DefaultWorktreePath, cand.Worktree.Path, log)
	if err != nil && force {
		fmt.Fprintf(warn, "warning: git worktree remove failed for %s: %s\n", cand.Worktree.Name, singleLineError(err))
		fmt.Fprintf(warn, "warning: falling back to rm -rf for %s\n", cand.Worktree.Name)
		if rmErr := rmRfWorktree(proj, cand.Worktree.Path); rmErr != nil {
			return false, rmErr
		}
		if pruneErr := runGit(proj.DefaultWorktreePath, nil, "worktree", "prune"); pruneErr != nil {
			fmt.Fprintf(warn, "warning: git worktree prune failed: %s\n", singleLineError(pruneErr))
		}
		err = nil
	}
	if err != nil {
		return false, err
	}

	branch := cand.Branch
	if branch == "" || branch == "HEAD" || branch == proj.Config.DefaultBranch {
		if force && branch == proj.Config.DefaultBranch {
			fmt.Fprintf(warn, "warning: skipped deleting local branch %s (default branch)\n", branch)
		}
		return false, nil
	}

	remoteTouched := false

	if err := gitDeleteLocalBranch(proj.DefaultWorktreePath, branch, log); err != nil {
		if !force {
			return remoteTouched, err
		}
		fmt.Fprintf(warn, "warning: failed to delete local branch %s: %s\n", branch, singleLineError(err))
	}

	if cand.HasRemoteBranch && cand.RemoteMatchesHead {
		if err := gitDeleteRemoteBranch(proj.DefaultWorktreePath, branch, log); err != nil {
			if !force {
				return remoteTouched, err
			}
			fmt.Fprintf(warn, "warning: failed to delete remote branch origin/%s: %s\n", branch, singleLineError(err))
		} else {
			remoteTouched = true
		}
	}

	return remoteTouched, nil
}

func rmRfWorktree(proj *project.Project, worktreePath string) error {
	if proj == nil {
		return fmt.Errorf("rm -rf refused: missing project")
	}
	root := canonicalizePath(proj.Root)
	if root == "" {
		return fmt.Errorf("rm -rf refused: missing project root")
	}
	wt := canonicalizePath(worktreePath)
	if wt == "" {
		return fmt.Errorf("rm -rf refused: missing worktree path")
	}

	if _, err := os.Stat(filepath.Join(root, ".wt")); err != nil {
		return fmt.Errorf("rm -rf refused: missing .wt directory at %s", root)
	}

	if !isWithin(wt, root) {
		return fmt.Errorf("rm -rf refused: %s is outside project root %s", wt, root)
	}
	if filepath.Dir(wt) != root {
		return fmt.Errorf("rm -rf refused: %s is not an immediate child of project root %s", wt, root)
	}
	if wt == canonicalizePath(proj.DefaultWorktreePath) {
		return fmt.Errorf("rm -rf refused: %s is the default worktree", wt)
	}
	if filepath.Base(wt) == ".wt" {
		return fmt.Errorf("rm -rf refused: target is .wt")
	}

	if err := makeTreeWritable(wt); err != nil {
		return fmt.Errorf("reset permissions: %w", err)
	}
	if err := os.RemoveAll(wt); err != nil {
		return fmt.Errorf("rm -rf failed for %s: %w", wt, err)
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

	targets, err := resolveWorktreeArgs(worktrees, args, wd)
	if err != nil {
		return nil, err
	}
	result := make([]project.Worktree, 0, len(targets))
	for _, target := range targets {
		if target.Name == proj.DefaultWorktree {
			return nil, fmt.Errorf("cannot remove the default worktree (%s)", target.Name)
		}
		result = append(result, target)
	}
	return result, nil
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
