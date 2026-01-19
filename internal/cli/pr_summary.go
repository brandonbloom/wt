package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type pullRequestInfo struct {
	Number    int
	State     string
	IsDraft   bool
	UpdatedAt time.Time
	URL       string
}

func (pr pullRequestInfo) Open() bool {
	state := strings.ToLower(pr.State)
	return state == "open"
}

func queryPullRequests(ctx context.Context, dir, branch string) ([]pullRequestInfo, error) {
	if branch == "" {
		return nil, nil
	}
	cmd := exec.CommandContext(
		ctx,
		"gh",
		"pr",
		"list",
		"--head", branch,
		"--state", "all",
		"--limit", "5",
		"--json", "number,state,isDraft,updatedAt,url",
	)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh pr list: %s", msg)
	}

	var raw []struct {
		Number    int    `json:"number"`
		State     string `json:"state"`
		IsDraft   bool   `json:"isDraft"`
		UpdatedAt string `json:"updatedAt"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &raw); err != nil {
		return nil, err
	}

	prs := make([]pullRequestInfo, 0, len(raw))
	for _, pr := range raw {
		t, _ := time.Parse(time.RFC3339, pr.UpdatedAt)
		prs = append(prs, pullRequestInfo{
			Number:    pr.Number,
			State:     pr.State,
			IsDraft:   pr.IsDraft,
			UpdatedAt: t,
			URL:       pr.URL,
		})
	}
	return prs, nil
}

type prSummary struct {
	Operation string
	Column    string
	Reason    string
}

type prContext struct {
	HasPendingWork   bool
	HasUniqueCommits bool
}

func summarizePullRequestState(ctx prContext, prs []pullRequestInfo, workflow workflowExpectations) prSummary {
	if !ctx.HasPendingWork {
		return prSummary{Column: ""}
	}
	active := openPullRequests(prs)
	if len(active) > 0 {
		if len(active) == 1 {
			label := formatSinglePR(active[0])
			text := "PR " + label
			return prSummary{Operation: text, Column: text}
		}
		text := formatMultiplePRs(active)
		return prSummary{Operation: text, Column: text}
	}
	if !ctx.HasUniqueCommits {
		return prSummary{Column: ""}
	}
	if len(prs) == 0 {
		if !workflow.PRsExpected {
			return prSummary{Column: "", Reason: ""}
		}
		return prSummary{
			Column: "No PR",
			Reason: "No PR",
		}
	}
	pr := prs[0]
	state := formatPRState(pr)
	text := fmt.Sprintf("PR #%d %s; unpublished commits", pr.Number, state)
	return prSummary{
		Operation: text,
		Column:    text,
		Reason:    text,
	}
}

func openPullRequests(prs []pullRequestInfo) []pullRequestInfo {
	var open []pullRequestInfo
	for _, pr := range prs {
		if pr.Open() {
			open = append(open, pr)
		}
	}
	return open
}

func formatSinglePR(pr pullRequestInfo) string {
	state := formatPRState(pr)
	return fmt.Sprintf("#%d %s", pr.Number, state)
}

func formatMultiplePRs(prs []pullRequestInfo) string {
	nums := make([]string, 0, len(prs))
	for i, pr := range prs {
		if i >= 3 {
			break
		}
		nums = append(nums, fmt.Sprintf("#%d", pr.Number))
	}
	return fmt.Sprintf("PR %s multiple", strings.Join(nums, ", "))
}

func formatPRState(pr pullRequestInfo) string {
	state := strings.ToLower(pr.State)
	if pr.IsDraft && state == "open" {
		return "draft"
	}
	return state
}
