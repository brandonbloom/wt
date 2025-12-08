package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/brandonbloom/wt/internal/timefmt"
)

type ciState int

const (
	ciStateUnknown ciState = iota
	ciStateSuccess
	ciStatePending
	ciStateFailure
	ciStateWarning
	ciStateError
)

type ciRunSummary struct {
	Name        string
	Status      string
	Conclusion  string
	URL         string
	StartedAt   time.Time
	CompletedAt time.Time
}

type ciResult struct {
	State   ciState
	Failure *ciRunSummary
	Message string
}

type ciTarget struct {
	Ref    string
	Branch string
	Head   string
}

type ciFetchOptions struct {
	Repo       *githubRepo
	RepoErr    error
	RemoteName string
	Workdir    string
}

type ciRequest struct {
	target   ciTarget
	statuses []*worktreeStatus
}

type ciFetchResult struct {
	req    *ciRequest
	result ciResult
	err    error
}

func fetchCIStatuses(ctx context.Context, opts ciFetchOptions, statuses []*worktreeStatus, now time.Time, onUpdate func(*worktreeStatus)) error {
	if len(statuses) == 0 {
		return nil
	}

	if opts.Repo == nil {
		msg := "CI: ? remote unavailable"
		if opts.RepoErr != nil {
			msg = fmt.Sprintf("CI: ? %s", singleLineError(opts.RepoErr))
		} else if opts.RemoteName != "" {
			msg = fmt.Sprintf("CI: ? remote %s missing", opts.RemoteName)
		}
		for _, status := range statuses {
			setCIError(status, msg, ciStateError)
			if onUpdate != nil {
				onUpdate(status)
			}
		}
		if opts.RepoErr != nil {
			return opts.RepoErr
		}
		return nil
	}

	keyed := make(map[string]*ciRequest)
	for _, status := range statuses {
		target, err := determineCITarget(status)
		if err != nil {
			setCIError(status, fmt.Sprintf("CI: ? %s", err.Error()), ciStateError)
			if onUpdate != nil {
				onUpdate(status)
			}
			continue
		}
		key := fmt.Sprintf("%s|%s|%s", opts.Repo.slug(), target.Ref, target.Branch)
		req := keyed[key]
		if req == nil {
			req = &ciRequest{target: target}
			keyed[key] = req
		}
		req.statuses = append(req.statuses, status)
	}

	if len(keyed) == 0 {
		return nil
	}

	results := make(chan ciFetchResult, len(keyed))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)

	for _, req := range keyed {
		req := req
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := fetchCITarget(ctx, opts, req.target)
			if errors.Is(err, context.Canceled) {
				return
			}
			results <- ciFetchResult{req: req, result: res, err: err}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var combined error
	for res := range results {
		if res.err != nil {
			combined = errors.Join(combined, res.err)
			msg := fmt.Sprintf("CI: ? %s", singleLineError(res.err))
			for _, status := range res.req.statuses {
				setCIError(status, msg, ciStateError)
				if onUpdate != nil {
					onUpdate(status)
				}
			}
			continue
		}
		for _, status := range res.req.statuses {
			applyCIResult(status, res.result, now)
			if onUpdate != nil {
				onUpdate(status)
			}
		}
	}
	return combined
}

func determineCITarget(status *worktreeStatus) (ciTarget, error) {
	if status == nil {
		return ciTarget{}, fmt.Errorf("status missing")
	}
	open := openPullRequests(status.PullRequests)
	if len(open) == 1 {
		return ciTarget{
			Ref:    fmt.Sprintf("refs/pull/%d/merge", open[0].Number),
			Branch: status.Branch,
			Head:   status.HeadHash,
		}, nil
	}
	if status.HeadHash == "" {
		return ciTarget{}, fmt.Errorf("commit unknown")
	}
	return ciTarget{
		Ref:    status.HeadHash,
		Branch: status.Branch,
		Head:   status.HeadHash,
	}, nil
}

func fetchCITarget(ctx context.Context, opts ciFetchOptions, target ciTarget) (ciResult, error) {
	path := fmt.Sprintf(
		"repos/%s/commits/%s/check-runs",
		opts.Repo.slug(),
		url.PathEscape(target.Ref),
	)
	data, err := runGhJSON(ctx, opts.Workdir, "api", path)
	if err != nil {
		return ciResult{}, err
	}
	var resp ghCheckRunsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ciResult{}, err
	}
	if len(resp.CheckRuns) > 0 {
		return summarizeCheckRuns(resp), nil
	}
	if target.Branch == "" {
		return ciResult{State: ciStateUnknown}, nil
	}
	fallback, err := fetchWorkflowFallback(ctx, opts, target)
	if err != nil {
		return ciResult{}, err
	}
	return fallback, nil
}

func fetchWorkflowFallback(ctx context.Context, opts ciFetchOptions, target ciTarget) (ciResult, error) {
	args := []string{
		"run", "list",
		"--branch", target.Branch,
		"--limit", "5",
		"--json", "name,status,conclusion,headSha,url,createdAt,updatedAt",
		"--repo", opts.Repo.slug(),
	}
	data, err := runGhJSON(ctx, opts.Workdir, args...)
	if err != nil {
		return ciResult{}, err
	}
	var runs []ghWorkflowRun
	if err := json.Unmarshal(data, &runs); err != nil {
		return ciResult{}, err
	}
	return summarizeWorkflowRuns(runs, target.Head), nil
}

func runGhJSON(ctx context.Context, workdir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

type ghCheckRunsResponse struct {
	TotalCount int          `json:"total_count"`
	CheckRuns  []ghCheckRun `json:"check_runs"`
}

type ghCheckRun struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	HTMLURL     string `json:"html_url"`
	DetailsURL  string `json:"details_url"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

type ghWorkflowRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"headSha"`
	URL        string `json:"url"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

func summarizeCheckRuns(resp ghCheckRunsResponse) ciResult {
	var failure *ciRunSummary
	hasPending := false
	hasSuccess := false
	hasWarning := false

	for _, run := range resp.CheckRuns {
		summary := ciRunSummary{
			Name:        strings.TrimSpace(run.Name),
			Status:      strings.ToLower(run.Status),
			Conclusion:  strings.ToLower(run.Conclusion),
			URL:         firstNonEmpty(run.HTMLURL, run.DetailsURL),
			StartedAt:   parseTime(run.StartedAt),
			CompletedAt: parseTime(run.CompletedAt),
		}
		switch summary.Status {
		case "queued", "in_progress":
			hasPending = true
			continue
		}
		switch summary.Conclusion {
		case "success":
			hasSuccess = true
		case "", "neutral", "skipped":
			hasWarning = true
		default:
			if failure == nil {
				failure = &summary
			}
		}
	}

	switch {
	case failure != nil:
		return ciResult{State: ciStateFailure, Failure: failure}
	case hasPending:
		return ciResult{State: ciStatePending}
	case hasSuccess:
		return ciResult{State: ciStateSuccess}
	case hasWarning:
		return ciResult{State: ciStateWarning}
	default:
		return ciResult{State: ciStateUnknown}
	}
}

func summarizeWorkflowRuns(runs []ghWorkflowRun, head string) ciResult {
	var failure *ciRunSummary
	hasPending := false
	hasSuccess := false

	for _, run := range runs {
		if head != "" && !strings.EqualFold(run.HeadSHA, head) {
			continue
		}
		summary := ciRunSummary{
			Name:        strings.TrimSpace(run.Name),
			Status:      strings.ToLower(run.Status),
			Conclusion:  strings.ToLower(run.Conclusion),
			URL:         run.URL,
			StartedAt:   parseTime(run.CreatedAt),
			CompletedAt: parseTime(run.UpdatedAt),
		}
		switch summary.Status {
		case "queued", "in_progress":
			hasPending = true
			continue
		}
		switch summary.Conclusion {
		case "success":
			hasSuccess = true
		case "failure", "cancelled", "timed_out", "action_required", "startup_failure", "stale":
			if failure == nil {
				failure = &summary
			}
		}
	}

	switch {
	case failure != nil:
		return ciResult{State: ciStateFailure, Failure: failure}
	case hasPending:
		return ciResult{State: ciStatePending}
	case hasSuccess:
		return ciResult{State: ciStateSuccess}
	default:
		return ciResult{State: ciStateUnknown}
	}
}

func setCIError(status *worktreeStatus, label string, state ciState) {
	if status == nil {
		return
	}
	status.CIStatus = label
	status.CIState = state
	status.CIDetail = nil
}

func applyCIResult(status *worktreeStatus, res ciResult, now time.Time) {
	if status == nil {
		return
	}
	status.CIState = res.State
	status.CIDetail = status.CIDetail[:0]
	switch res.State {
	case ciStateFailure:
		if res.Failure != nil {
			status.CIDetail = append(status.CIDetail, *res.Failure)
		}
		status.CIStatus = formatCILabel(res, now)
	case ciStatePending:
		status.CIStatus = "CI◷"
	case ciStateSuccess:
		status.CIStatus = "CI✓"
	case ciStateWarning:
		status.CIStatus = "CI!"
	case ciStateError:
		status.CIStatus = formatErrorLabel(res.Message)
	case ciStateUnknown:
		if strings.TrimSpace(res.Message) == "" {
			status.CIStatus = ""
		} else {
			status.CIStatus = formatErrorLabel(res.Message)
		}
	default:
		status.CIStatus = "CI?"
	}
}

func formatCILabel(res ciResult, now time.Time) string {
	if res.State != ciStateFailure || res.Failure == nil {
		if res.State == ciStateFailure {
			return "CI✗"
		}
		return formatErrorLabel(res.Message)
	}
	label := "CI✗"
	name := strings.TrimSpace(res.Failure.Name)
	if name != "" {
		label = fmt.Sprintf("CI✗ %s", name)
	}
	if !res.Failure.CompletedAt.IsZero() {
		label = fmt.Sprintf("%s (%s)", label, timefmt.Relative(res.Failure.CompletedAt, now))
	}
	return label
}

func parseTime(val string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(val))
	if err != nil {
		return time.Time{}
	}
	return t
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func formatErrorLabel(msg string) string {
	if strings.TrimSpace(msg) == "" {
		return "CI?"
	}
	if strings.HasPrefix(strings.TrimSpace(msg), "CI") {
		return msg
	}
	return fmt.Sprintf("CI? %s", msg)
}
