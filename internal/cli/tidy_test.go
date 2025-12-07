package cli

import (
	"strings"
	"testing"
)

func TestDescribePRSummarySuppressesWhenNoUniqueCommits(t *testing.T) {
	cand := &tidyCandidate{
		UniqueAhead: 0,
		Dirty:       false,
		HasStash:    false,
		PRs: []pullRequestInfo{
			{Number: 107, State: "MERGED"},
		},
	}
	if got := describePRSummary(cand); got != "none" {
		t.Fatalf("expected summary to be suppressed, got %q", got)
	}
}

func TestDescribePRSummaryShowsPRsWhenUniqueCommitsRemain(t *testing.T) {
	cand := &tidyCandidate{
		UniqueAhead: 1,
		PRs: []pullRequestInfo{
			{Number: 92, State: "OPEN"},
		},
	}
	got := describePRSummary(cand)
	if !strings.Contains(got, "#92") {
		t.Fatalf("expected summary to mention PR #92, got %q", got)
	}
}
