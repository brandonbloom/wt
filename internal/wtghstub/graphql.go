// GraphQL implements the minimal `gh api graphql` behavior needed by `wt` status/tidy
// during transcript tests. It supports `-f/-F bN=<branch>` parameters and emits
// `repository.prN.nodes` entries derived from `WT_GH_STATE_FILE`.
package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func handleGraphQL(stateFile string, args []string) (string, error) {
	branchByIdx := map[int]string{}
	re := regexp.MustCompile(`^b(\d+)$`)
	for i := 0; i < len(args); i++ {
		if args[i] != "-f" && args[i] != "-F" {
			continue
		}
		if i+1 >= len(args) {
			break
		}
		kv := args[i+1]
		i++
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		m := re.FindStringSubmatch(key)
		if len(m) != 2 {
			continue
		}
		idx, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		branchByIdx[idx] = val
	}

	type gqlPRNode struct {
		Number      int    `json:"number"`
		State       string `json:"state"`
		IsDraft     bool   `json:"isDraft"`
		UpdatedAt   string `json:"updatedAt"`
		URL         string `json:"url"`
		HeadRefName string `json:"headRefName"`
	}
	type gqlPRConn struct {
		Nodes []gqlPRNode `json:"nodes"`
	}

	prs := loadPRs(stateFile)
	indices := make([]int, 0, len(branchByIdx))
	for idx := range branchByIdx {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	repo := map[string]any{}
	for _, idx := range indices {
		branch := branchByIdx[idx]
		nodes := make([]gqlPRNode, 0, 5)
		for _, pr := range prs {
			if pr.Branch != branch {
				continue
			}
			nodes = append(nodes, gqlPRNode{
				Number:      pr.Number,
				State:       pr.State,
				IsDraft:     pr.IsDraft,
				UpdatedAt:   pr.UpdatedAt,
				URL:         pr.URL,
				HeadRefName: pr.Branch,
			})
			if len(nodes) >= 5 {
				break
			}
		}
		repo[fmt.Sprintf("pr%d", idx)] = gqlPRConn{Nodes: nodes}
	}

	payload := map[string]any{
		"data": map[string]any{
			"repository": repo,
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
