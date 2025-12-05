package project

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/brandonbloom/wt/internal/config"
)

var (
	// ErrNotFound indicates that .wt could not be discovered.
	ErrNotFound = errors.New("run `wt init` to create a project in this directory")
	// ErrDefaultWorktreeMissing indicates neither main nor master exist.
	ErrDefaultWorktreeMissing = errors.New("default worktree missing; expected a main/ or master/ directory")
	// ErrDefaultWorktreeConflict indicates both default names exist simultaneously.
	ErrDefaultWorktreeConflict = errors.New("ambiguous default worktree; found both main/ and master/")
)

// Project encapsulates a wt-enabled repository discovered on disk.
type Project struct {
	Root                string
	ConfigPath          string
	Config              config.Config
	DefaultWorktree     string
	DefaultWorktreePath string
}

// Discover walks upward from start until it finds a .wt directory.
func Discover(start string) (*Project, error) {
	root, err := locateRoot(start)
	if err != nil {
		return nil, err
	}
	return Load(root)
}

// Load constructs a Project from a known root directory.
func Load(root string) (*Project, error) {
	defaultName, defaultPath, err := resolveDefaultWorktree(root)
	if err != nil {
		return nil, err
	}

	cfgPath := filepath.Join(root, ".wt", "config.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}

	return &Project{
		Root:                root,
		ConfigPath:          cfgPath,
		Config:              cfg,
		DefaultWorktree:     defaultName,
		DefaultWorktreePath: defaultPath,
	}, nil
}

func locateRoot(start string) (string, error) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if isDir(filepath.Join(cur, ".wt")) {
			return cur, nil
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return "", ErrNotFound
}

func resolveDefaultWorktree(root string) (string, string, error) {
	mainPath := filepath.Join(root, "main")
	masterPath := filepath.Join(root, "master")

	mainOK := isWorktree(mainPath)
	masterOK := isWorktree(masterPath)

	switch {
	case mainOK && masterOK:
		return "", "", ErrDefaultWorktreeConflict
	case !mainOK && !masterOK:
		return "", "", ErrDefaultWorktreeMissing
	case mainOK:
		return "main", mainPath, nil
	default:
		return "master", masterPath, nil
	}
}

// DetectDefaultWorktree reports which default worktree directory (main/master)
// exists under root, along with its absolute path, even when .wt is missing.
func DetectDefaultWorktree(root string) (string, string, error) {
	return resolveDefaultWorktree(root)
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func isWorktree(path string) bool {
	if !isDir(path) {
		return false
	}
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// Worktree describes a git worktree living under the project root.
type Worktree struct {
	Name string
	Path string
}

// ListWorktrees enumerates all git worktrees immediately under the root.
func ListWorktrees(root string) ([]Worktree, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var result []Worktree
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == ".wt" {
			continue
		}
		path := filepath.Join(root, name)
		if !isWorktree(path) {
			continue
		}
		result = append(result, Worktree{Name: name, Path: path})
	}
	sortWorktrees(result)
	return result, nil
}

func sortWorktrees(wts []Worktree) {
	sort.Slice(wts, func(i, j int) bool {
		return wts[i].Name < wts[j].Name
	})
}

// EnsureWTDir makes sure the .wt directory exists.
func EnsureWTDir(root string) error {
	return os.MkdirAll(filepath.Join(root, ".wt"), 0o755)
}

// EnsureConfig ensures a baseline config file exists, writing when missing.
func EnsureConfig(root string, defaultBranch string) (config.Config, error) {
	if err := EnsureWTDir(root); err != nil {
		return config.Config{}, err
	}
	path := filepath.Join(root, ".wt", "config.toml")
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		cfg := config.Default(defaultBranch)
		if err := config.Save(path, cfg); err != nil {
			return config.Config{}, err
		}
		return cfg, nil
	}
	return config.Load(path)
}
