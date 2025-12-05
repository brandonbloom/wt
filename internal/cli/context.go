package cli

import (
	"os"

	"github.com/brandonbloom/wt/internal/project"
)

func loadProjectFromWD() (*project.Project, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return project.Discover(wd)
}
