package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brandonbloom/wt/internal/processes"
	"github.com/brandonbloom/wt/internal/project"
	"github.com/brandonbloom/wt/internal/shellbridge"
	"github.com/brandonbloom/wt/internal/timefmt"
	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func runStatus(cmd *cobra.Command, args []string) error {
	statusPreflight(cmd)
	proj, err := loadProjectFromWD()
	if err != nil {
		return err
	}
	defaultCompareRef := defaultBranchComparisonRef(proj)
	ciRepo, ciRepoErr := resolveGitHubRepo(proj)

	worktrees, err := project.ListWorktrees(proj.Root)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	current := ""
	for _, wt := range worktrees {
		if isWithin(wd, wt.Path) {
			current = wt.Name
			break
		}
	}

	now := currentTimeOverride()
	statuses := make([]*worktreeStatus, 0, len(worktrees))
	for _, wt := range worktrees {
		status, err := collectWorktreeStatus(proj, wt, defaultCompareRef)
		if err != nil {
			msg := singleLineError(err)
			if friendly, ok := friendlyWorktreeGitError(wt.Name, err); ok {
				msg = friendly
			}
			statuses = append(statuses, &worktreeStatus{
				Name:      wt.Name,
				Path:      wt.Path,
				Branch:    wt.Name,
				Timestamp: now,
				PRStatus:  fmt.Sprintf("error: %s", msg),
				Error:     msg,
				HasError:  true,
			})
			continue
		}
		status.Current = wt.Name == current
		status.PRStatus = prLoadingLabel
		statuses = append(statuses, status)
	}

	if err := attachProcessesToStatuses(statuses, worktrees); err != nil {
		return err
	}

	sort.SliceStable(statuses, func(i, j int) bool {
		if statuses[i].Timestamp.Equal(statuses[j].Timestamp) {
			return statuses[i].Name < statuses[j].Name
		}
		return statuses[i].Timestamp.After(statuses[j].Timestamp)
	})

	out := cmd.OutOrStdout()
	termWidth, isTTY := terminalWidth(out)
	layout := buildColumnLayout(statuses, now, termWidth)
	layout.useColor = isTTY
	if os.Getenv("WT_DEBUG_STATUS") != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "status debug: tty=%t rows=%d\n", isTTY, len(statuses))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var renderer *statusRenderer
	if isTTY {
		renderer = newStatusRenderer(out)
		if renderer != nil {
			renderer.Render(statuses, layout, now)
		}
	}
	var rerender func(*worktreeStatus)
	if renderer != nil {
		rerender = func(*worktreeStatus) {
			renderer.Render(statuses, layout, now)
		}
	}

	err = fetchPullRequestStatuses(ctx, statuses, proj.DefaultWorktree, rerender)
	if err != nil && errors.Is(err, context.Canceled) {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: cancelled GitHub fetch")
	}

	ciOpts := ciFetchOptions{
		Repo:       ciRepo,
		RepoErr:    ciRepoErr,
		RemoteName: proj.Config.CIRemote(),
		Workdir:    proj.DefaultWorktreePath,
	}
	if err := fetchCIStatuses(ctx, ciOpts, statuses, now, rerender); err != nil && errors.Is(err, context.Canceled) {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: cancelled GitHub fetch")
	}

	if renderer == nil {
		printStatuses(out, statuses, now, layout)
	}
	printCIDetail(out, statuses, now)

	return nil
}

type worktreeStatus struct {
	Name           string
	Path           string
	Branch         string
	Dirty          bool
	HasStash       bool
	Ahead          int
	Behind         int
	BaseAhead      int
	BaseBehind     int
	UniqueAhead    int
	Timestamp      time.Time
	HeadHash       string
	Current        bool
	PRStatus       string
	Operation      string
	NeedsInput     bool
	Processes      []processes.Process
	ProcessWarn    bool
	Error          string
	HasError       bool
	HasPendingWork bool
	PullRequests   []pullRequestInfo
	CIStatus       string
	CIState        ciState
	CIDetail       []ciRunSummary
}

func collectWorktreeStatus(proj *project.Project, wt project.Worktree, defaultCompareRef string) (*worktreeStatus, error) {
	data, err := gatherWorktreeGitData(proj, wt, defaultCompareRef)
	if err != nil {
		return nil, err
	}
	status := &worktreeStatus{
		Name:        wt.Name,
		Path:        wt.Path,
		Branch:      data.Branch,
		Dirty:       data.Dirty,
		HasStash:    data.HasStash,
		Ahead:       data.Ahead,
		Behind:      data.Behind,
		BaseAhead:   data.BaseAhead,
		BaseBehind:  data.BaseBehind,
		UniqueAhead: data.UniqueAhead,
		Timestamp:   data.Timestamp,
		Operation:   data.Operation,
		HeadHash:    data.HeadHash,
	}
	status.HasPendingWork = hasPendingWork(status.Dirty, status.HasStash, status.UniqueAhead)
	return status, nil
}

func hasPendingWork(dirty bool, hasStash bool, uniqueAhead int) bool {
	return dirty || hasStash || uniqueAhead > 0
}

func terminalWidth(w io.Writer) (int, bool) {
	f, ok := w.(*os.File)
	if ok {
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			width, _, err := term.GetSize(fd)
			if err == nil && width > 0 {
				if os.Getenv("WT_DEBUG_STATUS") != "" {
					fmt.Fprintf(os.Stderr, "terminal width via term.GetSize: %d (tty)\n", width)
				}
				return width, true
			}
			if envWidth := envTerminalWidth(); envWidth > 0 {
				if os.Getenv("WT_DEBUG_STATUS") != "" {
					fmt.Fprintf(os.Stderr, "terminal width via $COLUMNS (tty fallback): %d\n", envWidth)
				}
				return envWidth, true
			}
			if os.Getenv("WT_DEBUG_STATUS") != "" {
				fmt.Fprintln(os.Stderr, "terminal width: using default 80 (tty fallback)")
			}
			return 80, true
		}
	}
	if envWidth := envTerminalWidth(); envWidth > 0 {
		if os.Getenv("WT_DEBUG_STATUS") != "" {
			fmt.Fprintf(os.Stderr, "terminal width via $COLUMNS (non-tty): %d\n", envWidth)
		}
		return envWidth, false
	}
	if os.Getenv("WT_DEBUG_STATUS") != "" {
		fmt.Fprintln(os.Stderr, "terminal width unknown, using 0")
	}
	return 0, false
}

func envTerminalWidth() int {
	if cols, ok := os.LookupEnv("COLUMNS"); ok {
		if v, err := strconv.Atoi(cols); err == nil && v > 0 {
			return v
		}
	}
	return 0
}

const (
	columnGap      = "   "
	columnGapWidth = len(columnGap)
)

const prLoadingLabel = "PR: loading..."

const statusColumnCount = 3

var columnMinWidths = [statusColumnCount]int{24, 16, 24}
var shrinkPriority = []int{2, 0, 1}

type columnLayout struct {
	widths         [statusColumnCount]int
	useColor       bool
	prDisplayWidth int
}

var (
	colorNameCurrent    = color.New(color.FgBlue, color.Bold).SprintFunc()
	colorBranchDirty    = color.New(color.FgRed).SprintFunc()
	colorBranchDiverged = color.New(color.FgMagenta).SprintFunc()
	colorBranchClean    = color.New(color.FgHiBlack).SprintFunc()
	colorTimeValue      = color.New(color.FgHiBlack).SprintFunc()
	colorOperation      = color.New(color.FgHiMagenta, color.Bold).SprintFunc()
	colorPRPending      = color.New(color.FgMagenta).SprintFunc()
	colorPRMerged       = color.New(color.FgGreen).SprintFunc()
	colorPRNone         = color.New(color.FgBlack, color.Faint).SprintFunc()
	colorPRError        = color.New(color.FgRed).SprintFunc()
	colorPROther        = color.New(color.FgCyan).SprintFunc()
	colorPRProcessWarn  = color.New(color.FgHiRed, color.Bold).SprintFunc()
)

func (cl columnLayout) totalWidth() int {
	total := 0
	for _, w := range cl.widths {
		total += w
	}
	if len(cl.widths) > 1 {
		total += (len(cl.widths) - 1) * columnGapWidth
	}
	return total
}

func buildColumnLayout(statuses []*worktreeStatus, now time.Time, maxWidth int) columnLayout {
	var widths [statusColumnCount]int
	var prBaseWidth int
	mins := columnMinWidths
	for _, status := range statuses {
		fields := statusFields(status, now, true, 0)
		for i, field := range fields {
			w := runewidth.StringWidth(field)
			if w > widths[i] {
				widths[i] = w
			}
			if i == 0 && w > mins[0] {
				mins[0] = w
			}
			if i == statusColumnCount-1 && w > prBaseWidth {
				prBaseWidth = w
			}
		}
	}
	for i, min := range mins {
		if widths[i] < min {
			widths[i] = min
		}
	}
	if maxWidth > 0 {
		widths = shrinkWidths(widths, mins, maxWidth)
		layout := columnLayout{widths: widths}
		total := layout.totalWidth()
		if total < maxWidth {
			widths[len(widths)-1] += maxWidth - total
		}
		layout.widths = widths
		layout.prDisplayWidth = widths[statusColumnCount-1]
		return layout
	}
	if prBaseWidth == 0 {
		prBaseWidth = widths[statusColumnCount-1]
	}
	if prBaseWidth < defaultProcessSummaryLimit {
		prBaseWidth = defaultProcessSummaryLimit
	}
	if widths[statusColumnCount-1] < prBaseWidth {
		widths[statusColumnCount-1] = prBaseWidth
	}
	return columnLayout{widths: widths, prDisplayWidth: prBaseWidth}
}

func shrinkWidths(widths [statusColumnCount]int, mins [statusColumnCount]int, maxWidth int) [statusColumnCount]int {
	layout := columnLayout{widths: widths}
	excess := layout.totalWidth() - maxWidth
	if excess <= 0 {
		return widths
	}
	for excess > 0 {
		shrunk := false
		for _, idx := range shrinkPriority {
			if widths[idx] > mins[idx] {
				widths[idx]--
				excess--
				shrunk = true
				if excess == 0 {
					break
				}
			}
		}
		if !shrunk {
			break
		}
	}
	return widths
}

func statusFields(status *worktreeStatus, now time.Time, includeSummary bool, prWidth int) [statusColumnCount]string {
	prefix := "  "
	if status.Current {
		prefix = "* "
	}
	mergedPR := status.PRStatus != "" && strings.Contains(strings.ToLower(status.PRStatus), "merged")
	branch := formatBranchStatus(status, !mergedPR)
	nameField := fmt.Sprintf("%s%s", prefix, status.Name)
	if branch != "" {
		nameField = fmt.Sprintf("%s  %s", nameField, branch)
	}
	relative := "-"
	if !status.Timestamp.IsZero() {
		relative = timefmt.Relative(status.Timestamp, now)
	}
	detail := combineStatusDetail(status.PRStatus, status.CIStatus)
	if includeSummary {
		if summary := summarizeProcesses(status.Processes, defaultProcessSummaryLimit); summary != "" {
			detail = appendProcessSummary(detail, summary, prWidth)
		}
	}
	if detail == "" {
		detail = "-"
	}
	return [statusColumnCount]string{
		nameField,
		relative,
		detail,
	}
}

func formatBranchStatus(status *worktreeStatus, includeBase bool) string {
	branchName := strings.TrimSpace(status.Branch)
	if branchName == "" {
		branchName = "-"
	}
	showBranchName := branchName == "-" || !strings.EqualFold(branchName, status.Name)

	parts := make([]string, 0, 5)
	if showBranchName {
		parts = append(parts, branchName)
	}
	if status.Dirty {
		parts = append(parts, "dirty")
	}
	if status.Operation != "" {
		parts = append(parts, fmt.Sprintf("(%s)", status.Operation))
	}
	if delta := formatDelta(status.Ahead, status.Behind); delta != "" {
		parts = append(parts, delta)
	}
	if includeBase {
		if base := formatBaseDelta(status.BaseAhead, status.BaseBehind); base != "" {
			parts = append(parts, base)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func formatDelta(ahead, behind int) string {
	parts := make([]string, 0, 2)
	if ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", ahead))
	}
	if behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", behind))
	}
	return strings.Join(parts, " ")
}

func formatBaseDelta(ahead, behind int) string {
	if ahead == 0 && behind == 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	if ahead > 0 {
		parts = append(parts, fmt.Sprintf("+%d", ahead))
	}
	if behind > 0 {
		parts = append(parts, fmt.Sprintf("-%d", behind))
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, " "))
}

func appendProcessSummary(pr, summary string, prWidth int) string {
	if summary == "" || summary == "-" {
		return pr
	}
	if prWidth <= 0 {
		prWidth = defaultProcessSummaryLimit
	}
	if pr == "" || pr == "-" {
		if runewidth.StringWidth(summary) > prWidth {
			return runewidth.Truncate(summary, prWidth, "…")
		}
		return summary
	}
	sep := " · "
	baseWidth := runewidth.StringWidth(pr)
	sepWidth := runewidth.StringWidth(sep)
	available := prWidth - baseWidth - sepWidth
	if available <= 0 {
		return pr
	}
	if runewidth.StringWidth(summary) > available {
		if available <= 1 {
			return pr
		}
		summary = runewidth.Truncate(summary, available, "…")
	}
	return pr + sep + summary
}

func padOrTrim(text string, width int) string {
	if width <= 0 {
		return ""
	}
	textWidth := runewidth.StringWidth(text)
	if textWidth < width {
		return text + strings.Repeat(" ", width-textWidth)
	}
	if textWidth == width {
		return text
	}
	indicator := "…"
	if strings.HasSuffix(text, ")") && width > 1 {
		indicator = "…)"
	}
	indicatorWidth := runewidth.StringWidth(indicator)
	if indicatorWidth >= width {
		return runewidth.Truncate(indicator, width, "")
	}
	keepWidth := width - indicatorWidth
	trimmed := runewidth.Truncate(text, keepWidth, "")
	result := trimmed + indicator
	resultWidth := runewidth.StringWidth(result)
	if resultWidth < width {
		result += strings.Repeat(" ", width-resultWidth)
	}
	return result
}

func formatStatusLine(status *worktreeStatus, now time.Time, layout columnLayout) string {
	prWidth := layout.prDisplayWidth
	if prWidth <= 0 {
		prWidth = defaultProcessSummaryLimit
	}
	fields := statusFields(status, now, true, prWidth)
	parts := make([]string, len(fields))
	for i, field := range fields {
		parts[i] = padOrTrim(field, layout.widths[i])
	}
	if layout.useColor {
		colorizeParts(parts, status)
	}
	return strings.Join(parts, columnGap)
}

type statusRenderer struct {
	w     *os.File
	lines int
}

func newStatusRenderer(writer io.Writer) *statusRenderer {
	f, ok := writer.(*os.File)
	if !ok {
		return nil
	}
	if !term.IsTerminal(int(f.Fd())) {
		return nil
	}
	return &statusRenderer{w: f}
}

func (r *statusRenderer) Render(statuses []*worktreeStatus, layout columnLayout, now time.Time) {
	if r == nil {
		return
	}
	lines := formatStatusLines(statuses, now, layout)
	if r.lines > 0 {
		fmt.Fprintf(r.w, "\x1b[%dA", r.lines)
		fmt.Fprint(r.w, "\r\x1b[J")
	}
	for _, line := range lines {
		fmt.Fprintln(r.w, line)
	}
	r.lines = len(lines)
}

func (r *statusRenderer) AddExtraLines(n int) {
	if r == nil || n <= 0 {
		return
	}
	r.lines += n
}

func colorizeParts(parts []string, status *worktreeStatus) {
	branchColor := colorBranchClean
	switch {
	case status.HasError:
		branchColor = colorPRError
	case status.Operation != "":
		branchColor = colorOperation
	case status.Dirty:
		branchColor = colorBranchDirty
	case status.Ahead > 0 || status.Behind > 0:
		branchColor = colorBranchDiverged
	}
	if status.Current {
		parts[0] = colorNameCurrent(parts[0])
	} else {
		parts[0] = branchColor(parts[0])
	}
	parts[1] = colorTimeValue(parts[1])
	parts[2] = chooseStatusColor(status)(parts[2])
}

func chooseStatusColor(status *worktreeStatus) func(a ...interface{}) string {
	if status.HasError {
		return colorPRError
	}
	if status.ProcessWarn {
		return colorPRProcessWarn
	}
	if status.NeedsInput {
		return color.New(color.FgHiRed).SprintFunc()
	}
	switch status.CIState {
	case ciStateFailure, ciStateError:
		return colorPRError
	case ciStatePending:
		return colorPRPending
	case ciStateWarning:
		return colorPROther
	}
	return choosePRStringColor(status.PRStatus)
}

func choosePRStringColor(prText string) func(a ...interface{}) string {
	pr := strings.ToLower(prText)
	switch {
	case strings.Contains(pr, "merged"):
		return colorPRMerged
	case strings.Contains(pr, "open"), strings.Contains(pr, "draft"), strings.Contains(pr, "pending"):
		return colorPRPending
	case strings.Contains(pr, "none"):
		return colorPRNone
	case strings.Contains(pr, "unavailable") || strings.Contains(pr, "multiple"):
		return colorPRError
	default:
		return colorPROther
	}
}

func printStatuses(w io.Writer, statuses []*worktreeStatus, now time.Time, layout columnLayout) {
	for _, status := range statuses {
		fmt.Fprintln(w, formatStatusLine(status, now, layout))
	}
}

func printCIDetail(w io.Writer, statuses []*worktreeStatus, now time.Time) {
	status := currentStatus(statuses)
	if status == nil || len(status.CIDetail) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "CI details (%s):\n", status.Name)
	for _, run := range status.CIDetail {
		state := strings.TrimSpace(run.Conclusion)
		if state == "" {
			state = strings.TrimSpace(run.Status)
		}
		if state == "" {
			state = "unknown"
		}
		fmt.Fprintf(w, "- %s — %s\n", run.Name, state)
		if times := formatCIDetailTimes(run, now); times != "" {
			fmt.Fprintf(w, "  %s\n", times)
		}
		if run.URL != "" {
			fmt.Fprintf(w, "  %s\n", run.URL)
		}
	}
}

func formatCIDetailTimes(run ciRunSummary, now time.Time) string {
	parts := make([]string, 0, 2)
	if !run.StartedAt.IsZero() {
		parts = append(parts, fmt.Sprintf("started %s", timefmt.Relative(run.StartedAt, now)))
	}
	if !run.CompletedAt.IsZero() {
		parts = append(parts, fmt.Sprintf("completed %s", timefmt.Relative(run.CompletedAt, now)))
	}
	return strings.Join(parts, " · ")
}

func currentStatus(statuses []*worktreeStatus) *worktreeStatus {
	for _, status := range statuses {
		if status.Current {
			return status
		}
	}
	return nil
}

func formatStatusLines(statuses []*worktreeStatus, now time.Time, layout columnLayout) []string {
	lines := make([]string, 0, len(statuses))
	for _, status := range statuses {
		lines = append(lines, formatStatusLine(status, now, layout))
	}
	return lines
}

func fetchPullRequestStatuses(ctx context.Context, statuses []*worktreeStatus, defaultWorktree string, onUpdate func(*worktreeStatus)) error {
	if len(statuses) == 0 {
		return nil
	}

	type prResult struct {
		status *worktreeStatus
		prs    []pullRequestInfo
		err    error
	}

	ordered := make([]*worktreeStatus, 0, len(statuses))
	if defaultWorktree != "" {
		for _, status := range statuses {
			if status != nil && status.Name == defaultWorktree {
				ordered = append(ordered, status)
				break
			}
		}
	}
	for _, status := range statuses {
		if status == nil {
			continue
		}
		if defaultWorktree != "" && status.Name == defaultWorktree {
			continue
		}
		ordered = append(ordered, status)
	}

	results := make(chan prResult, len(statuses))
	var wg sync.WaitGroup
	for _, status := range statuses {
		status := status
		wg.Add(1)
		go func() {
			defer wg.Done()
			prs, err := queryPullRequests(ctx, status.Path, status.Branch)
			if errors.Is(err, context.Canceled) {
				return
			}
			results <- prResult{status: status, prs: prs, err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var combined error
	pending := make(map[*worktreeStatus]prResult, len(statuses))
	next := 0
	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if errors.Is(err, context.Canceled) {
				markPRInterrupted(statuses, onUpdate)
			}
			return err
		case res, ok := <-results:
			if !ok {
				return combined
			}
			if res.status != nil {
				pending[res.status] = res
			}
			for next < len(ordered) {
				status := ordered[next]
				prRes, ready := pending[status]
				if !ready {
					break
				}
				delete(pending, status)
				next++

				if prRes.err != nil {
					msg := singleLineError(prRes.err)
					if msg == "" {
						msg = "error"
					}
					status.PRStatus = fmt.Sprintf("PR: unavailable (%s)", msg)
					combined = errors.Join(combined, fmt.Errorf("%s: %w", status.Name, prRes.err))
					if onUpdate != nil {
						onUpdate(status)
					}
					continue
				}
				status.PullRequests = append([]pullRequestInfo(nil), prRes.prs...)
				summary := summarizePullRequestState(prContext{
					HasPendingWork:   status.HasPendingWork,
					HasUniqueCommits: status.UniqueAhead > 0,
				}, prRes.prs)
				status.PRStatus = summary.Column
				if onUpdate != nil {
					onUpdate(status)
				}
			}
		}
	}
}

func markPRInterrupted(statuses []*worktreeStatus, onUpdate func(*worktreeStatus)) {
	for _, status := range statuses {
		if status == nil {
			continue
		}
		pr := strings.TrimSpace(status.PRStatus)
		switch {
		case pr == "":
		case strings.EqualFold(pr, prLoadingLabel):
		case strings.EqualFold(pr, prInterruptedLabel):
		default:
			continue
		}
		status.PRStatus = prInterruptedLabel
		if onUpdate != nil {
			onUpdate(status)
		}
	}
}

func statusPreflight(cmd *cobra.Command) {
	if shellbridge.Active() && shellbridge.InstructionFile() != "" {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", shellbridge.ErrWrapperMissing)
}

func singleLineError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\r\n", "\n")
	msg = strings.TrimSpace(msg)
	return strings.ReplaceAll(msg, "\n", "; ")
}

func isDetachedHeadError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "does not point to a branch") || strings.Contains(msg, "You are not currently on a branch")
}
func combineStatusDetail(prStatus, ciStatus string) string {
	pr := strings.TrimSpace(prStatus)
	ci := strings.TrimSpace(ciStatus)
	if isNoPRStatus(pr) && isCIMissingCommit(ci) {
		ci = ""
	}
	switch {
	case pr != "" && ci != "":
		switch {
		case pr == prInterruptedLabel && ci == ciInterruptedLabel:
			return "PR/CI: interrupted"
		case strings.EqualFold(pr, ci):
			return pr
		default:
			return fmt.Sprintf("%s · %s", pr, ci)
		}
	case pr != "":
		return pr
	case ci != "":
		return ci
	default:
		return ""
	}
}

func isNoPRStatus(text string) bool {
	return strings.EqualFold(strings.TrimSpace(text), "No PR")
}

func isCIMissingCommit(text string) bool {
	return strings.EqualFold(strings.TrimSpace(text), ciMissingCommitLabel)
}
