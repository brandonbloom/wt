package cli

import (
	"fmt"
	"path/filepath"

	"github.com/brandonbloom/wt/internal/gitutil"
	"github.com/brandonbloom/wt/internal/project"
)

type githubRepo struct {
	Owner  string
	Name   string
	Remote string
}

func (r githubRepo) slug() string {
	return fmt.Sprintf("%s/%s", r.Owner, r.Name)
}

func resolveGitHubRepo(proj *project.Project) (*githubRepo, error) {
	if proj == nil {
		return nil, fmt.Errorf("project not loaded")
	}
	remote := proj.Config.CIRemote()
	workdir := proj.DefaultWorktreePath
	if workdir == "" {
		workdir = filepath.Join(proj.Root, proj.DefaultWorktree)
	}
	url, err := gitutil.RemoteURL(workdir, remote)
	if err != nil {
		return nil, fmt.Errorf("git remote %s: %w", remote, err)
	}
	owner, name, err := gitutil.ParseGitHubRemote(url)
	if err != nil {
		return nil, err
	}
	return &githubRepo{
		Owner:  owner,
		Name:   name,
		Remote: remote,
	}, nil
}
