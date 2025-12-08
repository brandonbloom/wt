package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/brandonbloom/wt/internal/processes"
	"github.com/brandonbloom/wt/internal/project"
	"github.com/brandonbloom/wt/internal/timefmt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type tidyPolicy string

const (
	tidyPolicyAuto   tidyPolicy = "auto"
	tidyPolicySafe   tidyPolicy = "safe"
	tidyPolicyAll    tidyPolicy = "all"
	tidyPolicyPrompt tidyPolicy = "prompt"
)

type tidyStage string

const (
	tidyStageScanning tidyStage = "scanning"
	tidyStageReady    tidyStage = "ready"
	tidyStagePrompt   tidyStage = "awaiting input"
	tidyStageCleaning tidyStage = "cleaning"
	tidyStageCleaned  tidyStage = "cleaned"
	tidyStageSkipped  tidyStage = "skipped"
	tidyStageBlocked  tidyStage = "blocked"
	tidyStageError    tidyStage = "error"
)

const blockReasonCurrentWorktree = "currently inside this worktree"

type tidyOptions struct {
	dryRun      bool
	policyFlag  string
	safeAlias   bool
	allAlias    bool
	promptAlias bool
	killFlag    string
	timeoutFlag string
}

func newTidyCommand() *cobra.Command {
	opts := &tidyOptions{}
	cmd := &cobra.Command{
		Use:   "tidy",
		Short: "Clean up merged or stale worktrees",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTidy(cmd, opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.dryRun, "dry-run", "n", false, "show actions without deleting anything")
	cmd.Flags().StringVar(&opts.policyFlag, "policy", "", "tidy policy: auto (default), safe, all, or prompt")
	cmd.Flags().BoolVarP(&opts.safeAlias, "safe", "s", false, "alias for --policy safe")
	cmd.Flags().BoolVarP(&opts.allAlias, "all", "a", false, "alias for --policy all")
	cmd.Flags().BoolVarP(&opts.promptAlias, "prompt", "p", false, "alias for --policy prompt")
	cmd.Flags().StringVarP(&opts.killFlag, "kill", "k", "", "terminate blocking processes before cleanup (optionally pass a signal)")
	if flag := cmd.Flags().Lookup("kill"); flag != nil {
		flag.NoOptDefVal = "true"
	}
	cmd.Flags().StringVar(&opts.timeoutFlag, "timeout", "", "time to wait for --kill to succeed (e.g. 3s)")
	return cmd
}

func runTidy(cmd *cobra.Command, opts *tidyOptions) error {
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

	policy, err := resolveTidyPolicy(opts, tidyPolicy(proj.Config.Tidy.Policy))
	if err != nil {
		return err
	}

	killEnabled := false
	killSignalSpec := ""
	if opts.killFlag != "" {
		killEnabled = true
		if opts.killFlag != "true" {
			killSignalSpec = opts.killFlag
		}
	}
	if opts.timeoutFlag != "" && !killEnabled {
		return fmt.Errorf("--timeout requires --kill")
	}
	var killCfg killSettings
	if killEnabled {
		killCfg, err = resolveKillSettings(killSignalSpec, opts.timeoutFlag, proj.Config.Process.KillTimeoutDuration())
		if err != nil {
			return err
		}
	}

	now := currentTimeOverride()
	candidates, err := collectTidyCandidates(cmd.Context(), proj, now)
	if err != nil {
		return err
	}

	if err := attachProcessesToCandidates(candidates); err != nil {
		return err
	}

	ui := newTidyUI(cmd.OutOrStdout(), candidates, now)

	if err := fetchTidyPullRequests(cmd.Context(), candidates, ui); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", singleLineError(err))
	}

	safe, gray, blocked := classifyCandidates(candidates, now, ui)

	var killPlan *killSettings
	if killEnabled {
		killPlan = &killCfg
		changed, err := tidyKillProcesses(cmd, safe, gray, killCfg, opts.dryRun, ui)
		if err != nil {
			return err
		}
		if changed {
			if err := attachProcessesToCandidates(candidates); err != nil {
				return err
			}
			safe, gray, blocked = classifyCandidates(candidates, now, ui)
		}
	}

	if opts.dryRun {
		if ui.Interactive() {
			return nil
		}
		return renderDryRun(cmd.OutOrStdout(), safe, gray, blocked, now, killPlan)
	}

	if !ui.Interactive() {
		fmt.Fprintln(cmd.OutOrStdout(), "Plan:")
		renderDryRun(cmd.OutOrStdout(), safe, gray, blocked, now, killPlan)
		fmt.Fprintln(cmd.OutOrStdout())
	}

	return executeTidies(cmd, proj, candidates, policy, now, ui, initialWD)
}

func resolveTidyPolicy(opts *tidyOptions, defaultPolicy tidyPolicy) (tidyPolicy, error) {
	requested := make([]tidyPolicy, 0, 3)
	if opts.policyFlag != "" {
		requested = append(requested, tidyPolicy(strings.ToLower(opts.policyFlag)))
	}
	if opts.safeAlias {
		requested = append(requested, tidyPolicySafe)
	}
	if opts.allAlias {
		requested = append(requested, tidyPolicyAll)
	}
	if opts.promptAlias {
		requested = append(requested, tidyPolicyPrompt)
	}

	policy := defaultPolicy
	if len(requested) > 0 {
		policy = requested[0]
		for _, val := range requested[1:] {
			if val != policy {
				return "", fmt.Errorf("conflicting policy flags")
			}
		}
	}

	switch policy {
	case tidyPolicyAuto, tidyPolicySafe, tidyPolicyAll, tidyPolicyPrompt:
		return policy, nil
	default:
		return "", fmt.Errorf("unknown policy %q (expected auto, safe, all, or prompt)", policy)
	}
}

type tidyCandidate struct {
	Worktree            project.Worktree
	Branch              string
	HeadHash            string
	Dirty               bool
	HasStash            bool
	IsCurrent           bool
	MergedIntoDefault   bool
	TreeMatchesDefault  bool
	HasRemoteBranch     bool
	RemoteMatchesHead   bool
	BaseAhead           int
	BaseBehind          int
	UniqueAhead         int
	LastActivity        time.Time
	PRs                 []pullRequestInfo
	BlockReasons        []string
	GrayReasons         []string
	extraGrayReasons    []string
	Classification      tidyClassification
	Stage               tidyStage
	sharedWith          []string
	divergenceThreshold int
	staleCutoffDays     int
	defaultBranch       string
	status              *worktreeStatus
	Processes           []processes.Process
}

func (cand *tidyCandidate) hasPendingWork() bool {
	if cand == nil {
		return false
	}
	return hasPendingWork(cand.Dirty, cand.HasStash, cand.UniqueAhead)
}

type tidyClassification int

const (
	tidyBlocked tidyClassification = iota
	tidySafe
	tidyGray
)

func collectTidyCandidates(ctx context.Context, proj *project.Project, now time.Time) ([]*tidyCandidate, error) {
	worktrees, err := project.ListWorktrees(proj.Root)
	if err != nil {
		return nil, err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	branchUsage := make(map[string][]string)
	base := make([]*tidyCandidate, 0, len(worktrees))
	for _, wt := range worktrees {
		if wt.Name == proj.DefaultWorktree {
			continue
		}
		cand, err := inspectWorktreeBase(ctx, proj, wt, wd)
		if err != nil {
			return nil, err
		}
		branchUsage[cand.Branch] = append(branchUsage[cand.Branch], wt.Name)
		base = append(base, cand)
	}

	for _, cand := range base {
		cand.sharedWith = filterOtherWorktrees(branchUsage[cand.Branch], cand.Worktree.Name)
		if len(cand.sharedWith) > 0 {
			cand.BlockReasons = append(cand.BlockReasons, fmt.Sprintf("branch also used by %s", strings.Join(cand.sharedWith, ", ")))
		}
	}

	return base, nil
}

func inspectWorktreeBase(ctx context.Context, proj *project.Project, wt project.Worktree, wd string) (*tidyCandidate, error) {
	cand := &tidyCandidate{
		Worktree:      wt,
		Stage:         tidyStageScanning,
		defaultBranch: proj.Config.DefaultBranch,
	}

	data, err := gatherWorktreeGitData(proj, wt)
	if err != nil {
		cand.Branch = "(unknown)"
		return markTidyGitError(cand, err)
	}
	cand.Branch = data.Branch

	if cand.Branch == "" || cand.Branch == "HEAD" {
		cand.BlockReasons = append(cand.BlockReasons, "detached HEAD")
	}
	if cand.Branch == proj.Config.DefaultBranch {
		cand.BlockReasons = append(cand.BlockReasons, fmt.Sprintf("branch is the default (%s)", proj.Config.DefaultBranch))
	}

	cand.IsCurrent = isWithin(wd, wt.Path)
	if cand.IsCurrent {
		cand.BlockReasons = append(cand.BlockReasons, blockReasonCurrentWorktree)
	}

	cand.Dirty = data.Dirty
	if cand.Dirty {
		cand.BlockReasons = append(cand.BlockReasons, "worktree has uncommitted changes")
	}

	cand.HasStash = data.HasStash
	if cand.HasStash {
		cand.BlockReasons = append(cand.BlockReasons, "stash entries reference this branch")
	}

	cand.BaseAhead = data.BaseAhead
	cand.BaseBehind = data.BaseBehind
	cand.LastActivity = data.Timestamp
	cand.HeadHash = data.HeadHash
	cand.MergedIntoDefault = data.MergedIntoDefault
	cand.TreeMatchesDefault = data.TreeMatchesDefault
	cand.UniqueAhead = data.UniqueAhead
	cand.HasRemoteBranch = data.HasRemoteBranch
	cand.RemoteMatchesHead = data.RemoteMatchesHead

	cand.divergenceThreshold = proj.Config.Tidy.DivergenceCommits
	cand.staleCutoffDays = proj.Config.Tidy.StaleDays

	if len(cand.BlockReasons) > 0 {
		cand.Stage = tidyStageBlocked
	}

	return cand, nil
}

func markTidyGitError(cand *tidyCandidate, err error) (*tidyCandidate, error) {
	msg := fmt.Sprintf("git error: %s", singleLineError(err))
	if friendly, ok := friendlyWorktreeGitError(cand.Worktree.Name, err); ok {
		msg = friendly
	}
	cand.BlockReasons = append(cand.BlockReasons, msg)
	cand.Stage = tidyStageBlocked
	if cand.Branch == "" {
		cand.Branch = "(unknown)"
	}
	if cand.LastActivity.IsZero() {
		cand.LastActivity = currentTimeOverride()
	}
	return cand, nil
}

func fetchTidyPullRequests(ctx context.Context, candidates []*tidyCandidate, ui *tidyUI) error {
	type result struct {
		cand *tidyCandidate
		prs  []pullRequestInfo
		err  error
	}

	results := make(chan result, len(candidates))
	var wg sync.WaitGroup
	for _, cand := range candidates {
		if len(cand.BlockReasons) > 0 {
			continue
		}
		cand := cand
		wg.Add(1)
		go func() {
			defer wg.Done()
			prs, err := queryPullRequests(ctx, cand.Worktree.Path, cand.Branch)
			if errors.Is(err, context.Canceled) {
				return
			}
			results <- result{cand: cand, prs: prs, err: err}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var combined error
	for res := range results {
		if res.err != nil {
			combined = errors.Join(combined, fmt.Errorf("%s: %w", res.cand.Worktree.Name, res.err))
			res.cand.extraGrayReasons = append(res.cand.extraGrayReasons, fmt.Sprintf("PR lookup failed: %s", singleLineError(res.err)))
		} else {
			res.cand.PRs = res.prs
			latest := res.cand.LastActivity
			for _, pr := range res.prs {
				if pr.UpdatedAt.After(latest) {
					latest = pr.UpdatedAt
				}
			}
			res.cand.LastActivity = latest
		}
		ui.Update(res.cand)
	}
	return combined
}

func classifyCandidates(candidates []*tidyCandidate, now time.Time, ui *tidyUI) ([]*tidyCandidate, []*tidyCandidate, []*tidyCandidate) {
	safe := make([]*tidyCandidate, 0)
	gray := make([]*tidyCandidate, 0)
	blocked := make([]*tidyCandidate, 0)
	for _, cand := range candidates {
		deriveClassification(cand, now)
		ui.Update(cand)
		switch cand.Classification {
		case tidySafe:
			safe = append(safe, cand)
		case tidyGray:
			gray = append(gray, cand)
		default:
			blocked = append(blocked, cand)
		}
	}
	return safe, gray, blocked
}

func deriveClassification(cand *tidyCandidate, now time.Time) {
	if len(cand.BlockReasons) > 0 {
		cand.Classification = tidyBlocked
		if cand.Stage != tidyStageCleaning && cand.Stage != tidyStageCleaned {
			cand.Stage = tidyStageBlocked
		}
		cand.GrayReasons = cand.GrayReasons[:0]
		return
	}

	reasons := make([]string, 0, len(cand.extraGrayReasons)+4)
	reasons = append(reasons, cand.extraGrayReasons...)

	hasUniqueCommits := cand.UniqueAhead > 0
	openPRs := openPullRequests(cand.PRs)
	needsCleanupDecision := hasUniqueCommits
	if needsCleanupDecision {
		reasons = append(reasons, fmt.Sprintf("commits not merged into %s", cand.defaultBranch))
		if len(openPRs) > 0 {
			for _, pr := range openPRs {
				reasons = append(reasons, fmt.Sprintf("PR #%d %s", pr.Number, formatPRState(pr)))
			}
		} else {
			summary := summarizePullRequestState(prContext{
				HasPendingWork:   cand.hasPendingWork(),
				HasUniqueCommits: cand.UniqueAhead > 0,
			}, cand.PRs)
			if summary.Reason != "" {
				reasons = append(reasons, summary.Reason)
			}
		}
		if cand.divergenceThreshold > 0 {
			divergence := maxInt(absInt(cand.BaseAhead), absInt(cand.BaseBehind))
			if divergence > cand.divergenceThreshold {
				reasons = append(reasons, fmt.Sprintf("diverged +%d/-%d from %s", cand.BaseAhead, cand.BaseBehind, cand.defaultBranch))
			}
		}
		if cand.staleCutoffDays > 0 {
			daysOld := int(now.Sub(cand.LastActivity).Hours() / 24)
			if daysOld > cand.staleCutoffDays {
				reasons = append(reasons, fmt.Sprintf("stale for %d days", daysOld))
			}
		}
	}

	if len(cand.PRs) > 1 {
		var nums []string
		for i, pr := range cand.PRs {
			if i >= 3 {
				break
			}
			nums = append(nums, fmt.Sprintf("#%d", pr.Number))
		}
		reasons = append(reasons, fmt.Sprintf("multiple PRs (%s)", strings.Join(nums, ", ")))
	}

	cand.GrayReasons = reasons
	if needsCleanupDecision && len(reasons) == 0 {
		reasons = append(reasons, "manual review")
	}

	if len(reasons) == 0 {
		cand.Classification = tidySafe
		cand.Stage = tidyStageReady
	} else {
		cand.Classification = tidyGray
		cand.Stage = tidyStagePrompt
	}
}

func renderDryRun(out io.Writer, safe, gray, blocked []*tidyCandidate, now time.Time, killPlan *killSettings) error {
	sections := 0
	if killPlan != nil {
		targets := append([]*tidyCandidate{}, safe...)
		targets = append(targets, gray...)
		renderKillPreview(out, targets, killPlan.SignalLabel)
	}
	if len(safe) > 0 {
		sections++
		fmt.Fprintln(out, "Will clean up:")
		for _, cand := range safe {
			fmt.Fprintf(out, "- %s (branch %s)\n", cand.Worktree.Name, cand.Branch)
			for _, action := range plannedActions(cand) {
				fmt.Fprintf(out, "    %s\n", action)
			}
		}
		fmt.Fprintln(out)
	}
	if len(gray) > 0 {
		sections++
		fmt.Fprintln(out, "Will prompt for:")
		for _, cand := range gray {
			fmt.Fprintf(out, "- %s (branch %s)\n", cand.Worktree.Name, cand.Branch)
			fmt.Fprintln(out, "    reasons:")
			for _, reason := range cand.GrayReasons {
				fmt.Fprintf(out, "      * %s\n", reason)
			}
		}
		fmt.Fprintln(out)
	}
	if len(blocked) > 0 {
		sections++
		fmt.Fprintln(out, "Will skip:")
		for _, cand := range blocked {
			fmt.Fprintf(out, "- %s (%s)\n", cand.Worktree.Name, strings.Join(cand.BlockReasons, "; "))
		}
	}
	if sections == 0 {
		fmt.Fprintln(out, "Nothing to tidy.")
	}
	if (len(safe) > 0 || len(gray) > 0) && sections > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Remote maintenance:")
		fmt.Fprintln(out, "- git remote prune origin")
	}
	return nil
}

func renderKillPreview(out io.Writer, candidates []*tidyCandidate, signalLabel string) bool {
	printed := false
	for _, cand := range candidates {
		if len(cand.Processes) == 0 {
			continue
		}
		if !printed {
			fmt.Fprintln(out, "Process cleanup:")
			printed = true
		}
		fmt.Fprintf(out, "- %s\n", cand.Worktree.Name)
		for _, proc := range cand.Processes {
			fmt.Fprintf(out, "    %s (%d)\n", processCommandLabel(proc.Command), proc.PID)
		}
		fmt.Fprintf(out, "    signal: %s\n", signalLabel)
	}
	if printed {
		fmt.Fprintln(out)
	}
	return printed
}

func plannedActions(cand *tidyCandidate) []string {
	actions := []string{
		fmt.Sprintf("remove worktree %s", cand.Worktree.Path),
		fmt.Sprintf("delete local branch %s", cand.Branch),
	}
	if cand.HasRemoteBranch {
		if cand.RemoteMatchesHead {
			actions = append(actions, fmt.Sprintf("delete remote branch origin/%s", cand.Branch))
		} else {
			actions = append(actions, fmt.Sprintf("skip remote branch origin/%s (tip changed)", cand.Branch))
		}
	}
	for _, pr := range cand.PRs {
		if pr.Open() {
			actions = append(actions, fmt.Sprintf("close PR #%d", pr.Number))
		}
	}
	return actions
}

func executeTidies(cmd *cobra.Command, proj *project.Project, candidates []*tidyCandidate, policy tidyPolicy, now time.Time, ui *tidyUI, initialWD string) error {
	out := cmd.OutOrStdout()
	reader := bufio.NewReader(cmd.InOrStdin())
	logWriter := out
	if ui.Interactive() {
		logWriter = nil
	}

	var remoteTouched bool
	var manualQuit bool
	var relocated bool
	for _, cand := range candidates {
		switch cand.Classification {
		case tidyBlocked:
			cand.Stage = tidyStageBlocked
			ui.Update(cand)
			if logWriter != nil {
				fmt.Fprintf(logWriter, "Skipped %s: %s\n", cand.Worktree.Name, strings.Join(cand.BlockReasons, "; "))
			}
			continue
		}

		if manualQuit {
			cand.Stage = tidyStageSkipped
			ui.Update(cand)
			if logWriter != nil {
				fmt.Fprintf(logWriter, "Skipped %s: quit selected\n", cand.Worktree.Name)
			}
			continue
		}

		if policy == tidyPolicySafe && cand.Classification == tidyGray {
			cand.Stage = tidyStageSkipped
			ui.Update(cand)
			if logWriter != nil {
				fmt.Fprintf(logWriter, "Skipped %s: --policy=safe\n", cand.Worktree.Name)
			}
			continue
		}

		prompt := shouldPrompt(cand.Classification, policy)
		if prompt {
			proceed, quit, lines, err := promptForCandidate(out, reader, cand, now, ui.Interactive())
			if ui.Interactive() {
				ui.AddExtraLines(lines)
			}
			if err != nil {
				return err
			}
			if quit {
				manualQuit = true
			}
			if !proceed {
				cand.Stage = tidyStageSkipped
				ui.Update(cand)
				if logWriter != nil {
					reason := "declined"
					if quit {
						reason = "quit selected"
					}
					fmt.Fprintf(logWriter, "Skipped %s: %s\n", cand.Worktree.Name, reason)
				}
				continue
			}
		}

		if !relocated && initialWD != "" && isWithin(initialWD, cand.Worktree.Path) {
			if err := os.Chdir(proj.Root); err != nil {
				return err
			}
			relocated = true
		}

		cand.Stage = tidyStageCleaning
		ui.Update(cand)

		touched, err := performCleanup(cmd.Context(), logWriter, proj, cand)
		if err != nil {
			cand.Stage = tidyStageError
			ui.Update(cand)
			return err
		}
		if touched {
			remoteTouched = true
		}

		cand.Stage = tidyStageCleaned
		ui.Update(cand)
	}

	if remoteTouched {
		if err := pruneRemote(logWriter, proj.DefaultWorktreePath); err != nil {
			return err
		}
	}
	return nil
}

func tidyKillProcesses(cmd *cobra.Command, safe, gray []*tidyCandidate, settings killSettings, dryRun bool, ui *tidyUI) (bool, error) {
	if len(safe) == 0 && len(gray) == 0 {
		return false, nil
	}
	targets := make([]*tidyCandidate, 0, len(safe)+len(gray))
	targets = append(targets, safe...)
	targets = append(targets, gray...)

	filtered := make([]*tidyCandidate, 0, len(targets))
	for _, cand := range targets {
		if len(cand.Processes) > 0 {
			filtered = append(filtered, cand)
		}
	}
	if len(filtered) == 0 || dryRun {
		return false, nil
	}

	logWriter := cmd.OutOrStdout()
	if ui != nil && ui.Interactive() {
		logWriter = nil
	}
	terminator := newProcessTerminator()
	changed := false

	for _, cand := range filtered {
		if logWriter != nil {
			fmt.Fprintf(logWriter, "Killing processes in %s (signal %s)\n", cand.Worktree.Name, settings.SignalLabel)
		}
		err := terminateWorktreeProcesses(cmd.Context(), cand.Worktree, cand.Processes, settings, terminator)
		if err != nil {
			if errors.Is(err, errProcessUnsupported) || errors.Is(err, context.Canceled) {
				return changed, err
			}
			msg := fmt.Sprintf("process cleanup failed: %s", singleLineError(err))
			cand.BlockReasons = append(cand.BlockReasons, msg)
			if logWriter != nil {
				fmt.Fprintf(logWriter, "  failed: %s\n", singleLineError(err))
			}
			continue
		}
		if logWriter != nil {
			fmt.Fprintln(logWriter, "  cleared")
		}
		changed = true
	}

	return changed, nil
}

func shouldPrompt(class tidyClassification, policy tidyPolicy) bool {
	switch policy {
	case tidyPolicyAll:
		return false
	case tidyPolicySafe:
		return false
	case tidyPolicyAuto:
		return class == tidyGray
	case tidyPolicyPrompt:
		return true
	default:
		return class == tidyGray
	}
}

func promptForCandidate(out io.Writer, reader *bufio.Reader, cand *tidyCandidate, now time.Time, useColor bool) (bool, bool, int, error) {
	var b strings.Builder

	title := fmt.Sprintf("%s (branch %s)", cand.Worktree.Name, cand.Branch)
	divider := promptDivider(len(title))
	if useColor {
		title = colorPromptTitle(title)
		divider = colorPromptDivider(divider)
	}
	fmt.Fprintf(&b, "\n%s\n%s\n", title, divider)

	label := func(s string) string {
		if useColor {
			return colorPromptLabel(s)
		}
		return s
	}
	value := func(s string) string {
		if useColor {
			return colorPromptValue(s)
		}
		return s
	}
	boolValue := func(v bool) string {
		text := boolLabel(v)
		if !useColor {
			return text
		}
		if v {
			return colorPromptWarn(text)
		}
		return colorPromptGood(text)
	}

	fmt.Fprintf(&b, "  %-14s %s\n", label("PR:"), value(describePRSummary(cand)))
	divergence := fmt.Sprintf("+%d/-%d vs %s", cand.BaseAhead, cand.BaseBehind, cand.defaultBranch)
	fmt.Fprintf(&b, "  %-14s %s\n", label("Divergence:"), value(divergence))
	fmt.Fprintf(&b, "  %-14s %s\n", label("Last activity:"), value(timefmt.Relative(cand.LastActivity, now)))
	fmt.Fprintf(&b, "  %-14s %s / %s\n", label("Dirty/Stash:"), boolValue(cand.Dirty), boolValue(cand.HasStash))
	fmt.Fprintf(&b, "  %-14s %s\n", label("Processes:"), value(summarizeProcesses(cand.Processes, defaultProcessSummaryLimit)))
	fmt.Fprintf(&b, "  %-14s %s\n", label("Worktree:"), value(cand.Worktree.Path))

	if len(cand.GrayReasons) > 0 {
		reasonsLabel := "  Reasons:"
		if useColor {
			reasonsLabel = colorPromptLabel(reasonsLabel)
		}
		fmt.Fprintln(&b, reasonsLabel)
		for _, reason := range cand.GrayReasons {
			reasonText := reason
			if useColor {
				reasonText = colorPromptReason(reasonText)
			}
			fmt.Fprintf(&b, "    - %s\n", reasonText)
		}
	} else {
		fmt.Fprintln(&b)
	}

	panel := b.String()
	fmt.Fprint(out, panel)
	prompt := "Proceed with cleanup? [y/N/q]: "
	if useColor {
		prompt = colorPromptLabel(prompt)
	}
	fmt.Fprint(out, prompt)

	resp, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, false, strings.Count(panel, "\n") + 2, err
	}
	fmt.Fprintln(out)

	resp = strings.TrimSpace(strings.ToLower(resp))
	ok := resp == "y" || resp == "yes"
	quit := resp == "q" || resp == "quit"
	lines := strings.Count(panel, "\n") + 2
	return ok, quit, lines, nil
}

func promptDivider(titleLen int) string {
	width := titleLen
	if width < 40 {
		width = 40
	}
	if width > 80 {
		width = 80
	}
	return strings.Repeat("-", width)
}

var (
	colorPromptTitle   = color.New(color.FgBlue, color.Bold).SprintFunc()
	colorPromptDivider = color.New(color.FgHiBlack).SprintFunc()
	colorPromptLabel   = color.New(color.FgBlack, color.Bold).SprintFunc()
	colorPromptValue   = color.New(color.FgHiBlue).SprintFunc()
	colorPromptReason  = color.New(color.FgMagenta).SprintFunc()
	colorPromptWarn    = color.New(color.FgHiRed, color.Bold).SprintFunc()
	colorPromptGood    = color.New(color.FgGreen, color.Bold).SprintFunc()
)

func boolLabel(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func performCleanup(ctx context.Context, log io.Writer, proj *project.Project, cand *tidyCandidate) (bool, error) {
	if log != nil {
		fmt.Fprintf(log, "Cleaning %s (branch %s)\n", cand.Worktree.Name, cand.Branch)
	}
	if err := gitWorktreeRemove(proj.DefaultWorktreePath, cand.Worktree.Path, log); err != nil {
		return false, err
	}
	if err := gitDeleteLocalBranch(proj.DefaultWorktreePath, cand.Branch, log); err != nil {
		return false, err
	}

	remoteTouched := false
	if cand.HasRemoteBranch {
		if cand.RemoteMatchesHead {
			if err := gitDeleteRemoteBranch(proj.DefaultWorktreePath, cand.Branch, log); err != nil {
				return remoteTouched, err
			}
			remoteTouched = true
		} else if log != nil {
			fmt.Fprintf(log, "  skipped remote branch origin/%s (tip changed)\n", cand.Branch)
		}
	}

	for _, pr := range cand.PRs {
		if !pr.Open() {
			continue
		}
		if err := closePullRequest(ctx, proj.DefaultWorktreePath, cand.Branch, pr.Number); err != nil {
			return remoteTouched, err
		}
		if log != nil {
			fmt.Fprintf(log, "  closed PR #%d\n", pr.Number)
		}
	}

	return remoteTouched, nil
}

func gitWorktreeRemove(repoDir, path string, log io.Writer) error {
	if err := makeTreeWritable(path); err != nil {
		return fmt.Errorf("reset permissions: %w", err)
	}
	if err := runGit(repoDir, nil, "worktree", "remove", "--force", path); err != nil {
		return err
	}
	if log != nil {
		fmt.Fprintf(log, "  removed worktree %s\n", path)
	}
	return nil
}

func gitDeleteLocalBranch(repoDir, branch string, log io.Writer) error {
	if err := runGit(repoDir, nil, "branch", "-D", branch); err != nil {
		return err
	}
	if log != nil {
		fmt.Fprintf(log, "  deleted local branch %s\n", branch)
	}
	return nil
}

func gitDeleteRemoteBranch(repoDir, branch string, log io.Writer) error {
	cmd := exec.Command("git", "-C", repoDir, "push", "origin", "--delete", branch)
	cmd.Stdin = os.Stdin

	var buf bytes.Buffer
	if log != nil {
		cmd.Stdout = &buf
		cmd.Stderr = &buf
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}

	if err := cmd.Run(); err != nil {
		missing, checkErr := remoteBranchMissing(repoDir, "origin", branch)
		if checkErr != nil || !missing {
			if log != nil {
				_, _ = log.Write(buf.Bytes())
			}
			return err
		}
		if log != nil {
			fmt.Fprintf(log, "  remote branch origin/%s already deleted\n", branch)
		}
		return nil
	}

	if log != nil {
		_, _ = log.Write(buf.Bytes())
		fmt.Fprintf(log, "  deleted remote branch origin/%s\n", branch)
	}
	return nil
}

func remoteBranchMissing(repoDir, remote, branch string) (bool, error) {
	exists, err := gitRemoteBranchExists(repoDir, remote, branch)
	if err != nil {
		return false, err
	}
	return !exists, nil
}

func gitRemoteBranchExists(repoDir, remote, branch string) (bool, error) {
	if remote == "" || branch == "" {
		return false, nil
	}
	var buf bytes.Buffer
	cmd := exec.Command("git", "-C", repoDir, "ls-remote", "--heads", remote, branch)
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return false, err
	}
	return strings.TrimSpace(buf.String()) != "", nil
}

func pruneRemote(log io.Writer, repoDir string) error {
	if err := runGit(repoDir, log, "remote", "prune", "origin"); err != nil {
		return err
	}
	if log != nil {
		fmt.Fprintln(log, "Pruned remote origin")
	}
	return nil
}

func runGit(dir string, out io.Writer, args ...string) error {
	_, err := runGitCapture(dir, out, args...)
	return err
}

func runGitCapture(dir string, out io.Writer, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdin = os.Stdin
	var buf bytes.Buffer
	writer := io.Writer(io.Discard)
	if out != nil {
		writer = out
	}
	multi := io.MultiWriter(writer, &buf)
	cmd.Stdout = multi
	cmd.Stderr = multi
	err := cmd.Run()
	return buf.String(), err
}

func makeTreeWritable(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			return nil
		}
		if mode&0o200 != 0 {
			return nil
		}
		return os.Chmod(path, mode|0o200)
	})
}

func closePullRequest(ctx context.Context, dir, branch string, number int) error {
	comment := fmt.Sprintf("Closed via wt tidy (branch %s)", branch)
	cmd := exec.CommandContext(ctx, "gh", "pr", "close", fmt.Sprintf("%d", number), "--comment", comment)
	cmd.Dir = dir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh pr close #%d: %w", number, err)
	}
	return nil
}

type tidyUI struct {
	interactive bool
	renderer    *statusRenderer
	statuses    []*worktreeStatus
	layout      columnLayout
	now         time.Time
}

func newTidyUI(out io.Writer, candidates []*tidyCandidate, now time.Time) *tidyUI {
	sortCandidatesForDisplay(candidates)
	statuses := make([]*worktreeStatus, len(candidates))
	for i, cand := range candidates {
		status := candidateToStatus(cand, now)
		cand.status = status
		statuses[i] = status
	}

	width, interactive := terminalWidth(out)
	layout := buildColumnLayout(statuses, now, width)
	layout.useColor = interactive

	var renderer *statusRenderer
	if interactive {
		renderer = newStatusRenderer(out)
		if renderer != nil {
			renderer.Render(statuses, layout, now)
		} else {
			interactive = false
		}
	}

	return &tidyUI{interactive: interactive, renderer: renderer, statuses: statuses, layout: layout, now: now}
}

func (ui *tidyUI) Interactive() bool {
	return ui != nil && ui.interactive && ui.renderer != nil
}

func (ui *tidyUI) Update(cand *tidyCandidate) {
	if cand.status != nil {
		populateStatusFromCandidate(cand, cand.status, ui.now)
	}
	if ui.Interactive() {
		ui.renderer.Render(ui.statuses, ui.layout, ui.now)
	}
}

func (ui *tidyUI) AddExtraLines(n int) {
	if ui.Interactive() {
		ui.renderer.AddExtraLines(n)
	}
}

func candidateToStatus(cand *tidyCandidate, now time.Time) *worktreeStatus {
	status := &worktreeStatus{
		Name:       cand.Worktree.Name,
		Path:       cand.Worktree.Path,
		Branch:     cand.Branch,
		Dirty:      cand.Dirty,
		BaseAhead:  cand.BaseAhead,
		BaseBehind: cand.BaseBehind,
		Timestamp:  cand.LastActivity,
	}
	populateStatusFromCandidate(cand, status, now)
	return status
}

func populateStatusFromCandidate(cand *tidyCandidate, status *worktreeStatus, now time.Time) {
	status.Name = cand.Worktree.Name
	status.Branch = cand.Branch
	status.Dirty = cand.Dirty
	status.HasStash = cand.HasStash
	status.BaseAhead = cand.BaseAhead
	status.BaseBehind = cand.BaseBehind
	status.UniqueAhead = cand.UniqueAhead
	status.Timestamp = cand.LastActivity
	status.HasPendingWork = cand.hasPendingWork()
	summary := summarizePullRequestState(prContext{
		HasPendingWork:   status.HasPendingWork,
		HasUniqueCommits: cand.UniqueAhead > 0,
	}, cand.PRs)
	status.Operation = summary.Operation
	status.PRStatus = tidyActionLabel(cand)
	status.NeedsInput = cand.Stage == tidyStagePrompt
	status.Processes = append([]processes.Process(nil), cand.Processes...)
	status.ProcessWarn = len(cand.Processes) > 0 && cand.Classification != tidySafe
	status.HasError = cand.Stage == tidyStageBlocked || cand.Stage == tidyStageError
}

func tidyActionLabel(cand *tidyCandidate) string {
	switch cand.Stage {
	case tidyStageReady:
		return "will clean"
	case tidyStagePrompt:
		if len(cand.GrayReasons) > 0 {
			return cand.GrayReasons[0]
		}
		return "awaiting review"
	case tidyStageCleaning:
		return "cleaning"
	case tidyStageCleaned:
		return "cleaned"
	case tidyStageSkipped:
		return "skipped"
	case tidyStageBlocked:
		if len(cand.BlockReasons) > 0 {
			return "blocked: " + cand.BlockReasons[0]
		}
		return "blocked"
	case tidyStageError:
		return "error"
	case tidyStageScanning:
		return "scanning"
	default:
		return string(cand.Stage)
	}
}

func describePRSummary(cand *tidyCandidate) string {
	if cand == nil {
		return "none"
	}
	if cand.UniqueAhead == 0 && !cand.Dirty && !cand.HasStash {
		return "none"
	}
	active := openPullRequests(cand.PRs)
	if len(active) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(active))
	for _, pr := range active {
		state := strings.ToLower(pr.State)
		if pr.IsDraft && state == "open" {
			state = "draft"
		}
		parts = append(parts, fmt.Sprintf("#%d %s", pr.Number, state))
	}
	return strings.Join(parts, ", ")
}

func filterOtherWorktrees(names []string, current string) []string {
	var result []string
	for _, name := range names {
		if name == current {
			continue
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func sortCandidatesForDisplay(cands []*tidyCandidate) {
	sort.SliceStable(cands, func(i, j int) bool {
		ti := cands[i].LastActivity
		tj := cands[j].LastActivity
		if ti.Equal(tj) {
			return cands[i].Worktree.Name < cands[j].Worktree.Name
		}
		return ti.After(tj)
	})
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
