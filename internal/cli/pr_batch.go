package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/trace"
	"sort"
	"strings"
	"time"
)

type prGraphQLNode struct {
	Number      int    `json:"number"`
	State       string `json:"state"`
	IsDraft     bool   `json:"isDraft"`
	UpdatedAt   string `json:"updatedAt"`
	URL         string `json:"url"`
	HeadRefName string `json:"headRefName"`
}

type prGraphQLConnection struct {
	Nodes []prGraphQLNode `json:"nodes"`
}

func buildPullRequestsGraphQLQuery(branches []string) (string, map[string]string, map[string]string) {
	unique := make([]string, 0, len(branches))
	seen := make(map[string]bool, len(branches))
	for _, branch := range branches {
		branch = strings.TrimSpace(branch)
		if branch == "" || seen[branch] {
			continue
		}
		seen[branch] = true
		unique = append(unique, branch)
	}

	sort.Strings(unique)

	aliasToBranch := make(map[string]string, len(unique))
	varValues := make(map[string]string, len(unique)+2)
	varValues["owner"] = ""
	varValues["name"] = ""

	var b strings.Builder
	b.WriteString("query(")
	b.WriteString("$owner:String!, $name:String!")
	for i := range unique {
		fmt.Fprintf(&b, ", $b%d:String!", i)
	}
	b.WriteString(") { repository(owner:$owner, name:$name) {")
	for i, branch := range unique {
		alias := fmt.Sprintf("pr%d", i)
		varName := fmt.Sprintf("b%d", i)
		aliasToBranch[alias] = branch
		varValues[varName] = branch

		fmt.Fprintf(&b, `
  %s: pullRequests(headRefName:$%s, states:[OPEN,CLOSED,MERGED], first:5, orderBy:{field:UPDATED_AT, direction:DESC}) {
    nodes { number state isDraft updatedAt url headRefName }
  }`, alias, varName)
	}
	b.WriteString("\n} }")

	return b.String(), aliasToBranch, varValues
}

func queryPullRequestsGraphQL(ctx context.Context, workdir string, repo *githubRepo, branches []string) (map[string][]pullRequestInfo, error) {
	if repo == nil {
		return nil, fmt.Errorf("repo unavailable")
	}
	region := trace.StartRegion(ctx, "gh pr batch")
	defer region.End()

	query, aliasToBranch, varValues := buildPullRequestsGraphQLQuery(branches)
	if len(aliasToBranch) == 0 {
		return map[string][]pullRequestInfo{}, nil
	}

	varValues["owner"] = repo.Owner
	varValues["name"] = repo.Name

	args := []string{"api", "graphql", "-f", "query=" + query}
	args = append(args, "-f", "owner="+varValues["owner"])
	args = append(args, "-f", "name="+varValues["name"])
	for i := 0; i < len(aliasToBranch); i++ {
		key := fmt.Sprintf("b%d", i)
		args = append(args, "-f", key+"="+varValues[key])
	}

	data, err := runGhJSON(ctx, workdir, args...)
	if err != nil {
		return nil, err
	}
	parseRegion := trace.StartRegion(ctx, "parse pr graphql json")
	out, err := parsePullRequestsGraphQLResponse(data, aliasToBranch)
	parseRegion.End()
	return out, err
}

func parsePullRequestsGraphQLResponse(data []byte, aliasToBranch map[string]string) (map[string][]pullRequestInfo, error) {
	var resp struct {
		Data struct {
			Repository map[string]prGraphQLConnection `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	out := make(map[string][]pullRequestInfo, len(aliasToBranch))
	for alias, branch := range aliasToBranch {
		conn, ok := resp.Data.Repository[alias]
		if !ok {
			out[branch] = nil
			continue
		}
		prs := make([]pullRequestInfo, 0, len(conn.Nodes))
		for _, node := range conn.Nodes {
			t, _ := time.Parse(time.RFC3339, node.UpdatedAt)
			prs = append(prs, pullRequestInfo{
				Number:    node.Number,
				State:     node.State,
				IsDraft:   node.IsDraft,
				UpdatedAt: t,
				URL:       node.URL,
			})
		}
		out[branch] = prs
	}
	return out, nil
}
