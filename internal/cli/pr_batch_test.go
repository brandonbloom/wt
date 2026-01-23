package cli

import (
	"testing"
	"time"
)

func TestParsePullRequestsGraphQLResponse(t *testing.T) {
	data := []byte(`{
  "data": {
    "repository": {
      "pr0": {
        "nodes": [
          {
            "number": 42,
            "state": "OPEN",
            "isDraft": false,
            "updatedAt": "2000-01-02T00:00:00Z",
            "url": "https://example.com/pr/42",
            "headRefName": "demo-branch"
          }
        ]
      },
      "pr1": {
        "nodes": [
          {
            "number": 99,
            "state": "MERGED",
            "isDraft": false,
            "updatedAt": "2000-01-02T00:00:00Z",
            "url": "https://example.com/pr/99",
            "headRefName": "merged-branch"
          }
        ]
      }
    }
  }
}`)

	aliasToBranch := map[string]string{
		"pr0": "demo-branch",
		"pr1": "merged-branch",
	}

	got, err := parsePullRequestsGraphQLResponse(data, aliasToBranch)
	if err != nil {
		t.Fatalf("parsePullRequestsGraphQLResponse returned error: %v", err)
	}

	if len(got["demo-branch"]) != 1 || got["demo-branch"][0].Number != 42 {
		t.Fatalf("demo-branch = %#v, want PR #42", got["demo-branch"])
	}
	if len(got["merged-branch"]) != 1 || got["merged-branch"][0].Number != 99 {
		t.Fatalf("merged-branch = %#v, want PR #99", got["merged-branch"])
	}

	wantTime, _ := time.Parse(time.RFC3339, "2000-01-02T00:00:00Z")
	if got["demo-branch"][0].UpdatedAt != wantTime {
		t.Fatalf("updatedAt = %v, want %v", got["demo-branch"][0].UpdatedAt, wantTime)
	}
}
