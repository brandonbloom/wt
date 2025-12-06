package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/brandonbloom/wt/internal/gitutil"
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
		status, err := collectWorktreeStatus(wt)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %s\n", wt.Name, singleLineError(err))
			continue
		}
		status.Current = wt.Name == current
		status.PRStatus = "(PR: pending)"
		statuses = append(statuses, status)
	}

	sort.SliceStable(statuses, func(i, j int) bool {
		if statuses[i].Timestamp.Equal(statuses[j].Timestamp) {
			return statuses[i].Name < statuses[j].Name
		}
		return statuses[i].Timestamp.After(statuses[j].Timestamp)
	})

	out := cmd.OutOrStdout()
	termWidth, interactive := terminalWidth(out)
	layout := buildColumnLayout(statuses, now, termWidth)
	layout.useColor = interactive

	var renderer *statusRenderer
	if interactive {
		renderer = newStatusRenderer(out)
		if renderer == nil {
			interactive = false
			layout.useColor = false
		} else {
			renderer.Render(statuses, layout, now)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err = fetchPullRequestStatuses(ctx, statuses, layout, renderer, now)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: cancelled GitHub fetch")
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", singleLineError(err))
		}
	}

	if !interactive {
		printStatuses(out, statuses, now, layout)
	} else if renderer != nil {
		renderer.Render(statuses, layout, now)
	}

	return nil
}

type worktreeStatus struct {
	Name      string
	Path      string
	Branch    string
	Dirty     bool
	Ahead     int
	Behind    int
	Timestamp time.Time
	Current   bool
	PRStatus  string
}

func collectWorktreeStatus(wt project.Worktree) (*worktreeStatus, error) {
	branch, err := gitutil.CurrentBranch(wt.Path)
	if err != nil {
		return nil, err
	}
	dirty, err := gitutil.Dirty(wt.Path)
	if err != nil {
		return nil, err
	}
	ahead, behind, err := gitutil.AheadBehind(wt.Path)
	if err != nil {
		return nil, err
	}
	ts, err := gitutil.HeadTimestamp(wt.Path)
	if err != nil {
		return nil, err
	}
	if dirty {
		if dirtyTS, derr := gitutil.LatestDirtyTimestamp(wt.Path); derr == nil {
			ts = dirtyTS
		}
	}
	return &worktreeStatus{
		Name:      wt.Name,
		Path:      wt.Path,
		Branch:    branch,
		Dirty:     dirty,
		Ahead:     ahead,
		Behind:    behind,
		Timestamp: ts,
	}, nil
}

func terminalWidth(w io.Writer) (int, bool) {
	f, ok := w.(*os.File)
	if !ok {
		return 0, false
	}
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		return 0, false
	}
	width, _, err := term.GetSize(fd)
	if err != nil || width <= 0 {
		return 80, true
	}
	return width, true
}

const columnGap = "   "
const columnGapWidth = len(columnGap)

var columnMinWidths = [4]int{4, 12, 8, 12}
var shrinkPriority = []int{3, 1, 0, 2}

type columnLayout struct {
	widths   [4]int
	useColor bool
}

var (
	colorNameCurrent    = color.New(color.FgBlue, color.Bold).SprintFunc()
	colorNameDefault    = color.New(color.FgBlack).SprintFunc()
	colorBranchDirty    = color.New(color.FgRed).SprintFunc()
	colorBranchDiverged = color.New(color.FgMagenta).SprintFunc()
	colorBranchClean    = color.New(color.FgHiBlack).SprintFunc()
	colorTimeValue      = color.New(color.FgHiBlack).SprintFunc()
	colorPRPending      = color.New(color.FgMagenta).SprintFunc()
	colorPRMerged       = color.New(color.FgGreen).SprintFunc()
	colorPRNone         = color.New(color.FgBlack, color.Faint).SprintFunc()
	colorPRError        = color.New(color.FgRed).SprintFunc()
	colorPROther        = color.New(color.FgCyan).SprintFunc()
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
	var widths [4]int
	for _, status := range statuses {
		fields := statusFields(status, now)
		for i, field := range fields {
			w := runewidth.StringWidth(field)
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	for i, min := range columnMinWidths {
		if widths[i] < min {
			widths[i] = min
		}
	}
	if maxWidth > 0 {
		widths = shrinkWidths(widths, maxWidth)
	}
	return columnLayout{widths: widths}
}

func shrinkWidths(widths [4]int, maxWidth int) [4]int {
	layout := columnLayout{widths: widths}
	excess := layout.totalWidth() - maxWidth
	if excess <= 0 {
		return widths
	}
	for excess > 0 {
		shrunk := false
		for _, idx := range shrinkPriority {
			if widths[idx] > columnMinWidths[idx] {
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

func statusFields(status *worktreeStatus, now time.Time) [4]string {
	prefix := "  "
	if status.Current {
		prefix = "* "
	}
	branch := status.Branch
	if status.Dirty {
		branch += "!"
	}
	if delta := formatDelta(status.Ahead, status.Behind); delta != "" {
		if branch != "" {
			branch += " "
		}
		branch += delta
	}
	relative := timefmt.Relative(status.Timestamp, now)
	pr := status.PRStatus
	if pr == "" {
		pr = "(PR: pending)"
	}
	return [4]string{
		fmt.Sprintf("%s%s", prefix, status.Name),
		strings.TrimSpace(branch),
		relative,
		pr,
	}
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
	fields := statusFields(status, now)
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

func colorizeParts(parts []string, status *worktreeStatus) {
	if status.Current {
		parts[0] = colorNameCurrent(parts[0])
	} else {
		parts[0] = colorNameDefault(parts[0])
	}

	branchColor := colorBranchClean
	switch {
	case status.Dirty:
		branchColor = colorBranchDirty
	case status.Ahead > 0 || status.Behind > 0:
		branchColor = colorBranchDiverged
	}
	parts[1] = branchColor(parts[1])

	parts[2] = colorTimeValue(parts[2])
	parts[3] = choosePRColor(status.PRStatus)(parts[3])
}

func choosePRColor(pr string) func(a ...interface{}) string {
	pr = strings.ToLower(pr)
	switch {
	case strings.Contains(pr, "unavailable") || strings.Contains(pr, "multiple"):
		return colorPRError
	case strings.Contains(pr, "merged"):
		return colorPRMerged
	case strings.Contains(pr, "pending"), strings.Contains(pr, "open"), strings.Contains(pr, "draft"):
		return colorPRPending
	case strings.Contains(pr, "none"):
		return colorPRNone
	default:
		return colorPROther
	}
}

func printStatuses(w io.Writer, statuses []*worktreeStatus, now time.Time, layout columnLayout) {
	for _, status := range statuses {
		fmt.Fprintln(w, formatStatusLine(status, now, layout))
	}
}

func formatStatusLines(statuses []*worktreeStatus, now time.Time, layout columnLayout) []string {
	lines := make([]string, 0, len(statuses))
	for _, status := range statuses {
		lines = append(lines, formatStatusLine(status, now, layout))
	}
	return lines
}

func fetchPullRequestStatuses(ctx context.Context, statuses []*worktreeStatus, layout columnLayout, renderer *statusRenderer, now time.Time) error {
	if len(statuses) == 0 {
		return nil
	}

	type prResult struct {
		status *worktreeStatus
		pr     string
		err    error
	}

	results := make(chan prResult, len(statuses))
	var wg sync.WaitGroup
	for _, status := range statuses {
		status := status
		wg.Add(1)
		go func() {
			defer wg.Done()
			prStatus, err := queryPullRequestStatus(ctx, status.Path, status.Branch)
			if errors.Is(err, context.Canceled) {
				return
			}
			results <- prResult{status: status, pr: prStatus, err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var combined error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-results:
			if !ok {
				if renderer != nil {
					renderer.Render(statuses, layout, now)
				}
				return combined
			}
			if res.err != nil {
				res.status.PRStatus = "(PR: unavailable)"
				combined = errors.Join(combined, fmt.Errorf("%s: %w", res.status.Name, res.err))
			} else {
				res.status.PRStatus = res.pr
			}
			if renderer != nil {
				renderer.Render(statuses, layout, now)
			}
		}
	}
}

func queryPullRequestStatus(ctx context.Context, dir, branch string) (string, error) {
	if branch == "" {
		return "(PR: none)", nil
	}
	cmd := exec.CommandContext(
		ctx,
		"gh",
		"pr",
		"list",
		"--head", branch,
		"--state", "all",
		"--limit", "2",
		"--json", "number,state,isDraft",
	)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gh pr list: %s", msg)
	}

	var pulls []struct {
		Number  int    `json:"number"`
		State   string `json:"state"`
		IsDraft bool   `json:"isDraft"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &pulls); err != nil {
		return "", err
	}
	switch len(pulls) {
	case 0:
		return "(PR: none)", nil
	case 1:
		pr := pulls[0]
		state := strings.ToLower(pr.State)
		if pr.IsDraft && state == "open" {
			state = "draft"
		}
		return fmt.Sprintf("(PR: %s #%d)", state, pr.Number), nil
	default:
		return "(PR: multiple)", nil
	}
}

func isWithin(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

func currentTimeOverride() time.Time {
	if override := os.Getenv("WT_NOW"); override != "" {
		if t, err := time.Parse(time.RFC3339, override); err == nil {
			return t
		}
	}
	return time.Now()
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
