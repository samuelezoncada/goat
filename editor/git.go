package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// gitTTL is how long a branch lookup is reused. The status bar is redrawn on
// every event, so the repository is only re-read a few times a second at most,
// and an external branch switch shows up within this window.
const gitTTL = 2 * time.Second

// gitLookup caches the branch of one directory.
type gitLookup struct {
	dir    string // the directory the lookup was made for
	branch string // "" when the directory is not inside a repository
	at     time.Time
}

// branchFor returns the checked-out branch of the repository containing dir,
// the short commit for a detached HEAD, or "" when dir is not in a repository.
func (e *Editor) branchFor(dir string) string {
	if dir == "" {
		return ""
	}
	now := time.Now()
	if e.git.dir == dir && now.Sub(e.git.at) < gitTTL {
		return e.git.branch
	}
	branch := gitBranch(dir)
	e.git = gitLookup{dir: dir, branch: branch, at: now}
	return branch
}

// gitContextDir is the directory whose repository the status bar reports: the
// active file's directory, else the project root, else the working directory.
func (e *Editor) gitContextDir() string {
	if t := e.active(); t != nil && t.path != "" {
		return filepath.Dir(t.path)
	}
	if e.root != "" {
		return e.root
	}
	if e.browser != nil && e.browser.root != "" {
		return e.browser.root
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

// gitBranch reads HEAD from the repository containing dir. It is a plain file
// read rather than a `git` invocation, so it costs nothing to call while
// drawing.
func gitBranch(dir string) string {
	if dir == "" {
		return "" // an empty path would resolve to the working directory
	}
	gitDir := findGitDir(dir)
	if gitDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	return parseHEAD(data)
}

// parseHEAD turns the contents of .git/HEAD into a display name: the branch
// name for a symbolic ref, the abbreviated commit for a detached HEAD.
func parseHEAD(data []byte) string {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return ""
	}
	if rest, ok := strings.CutPrefix(s, "ref:"); ok {
		ref := strings.TrimSpace(rest)
		// refs/heads/feature/x -> feature/x; anything else keeps its full ref.
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			return name
		}
		return ref
	}
	if isHex(s) && len(s) >= 7 {
		return s[:7] // detached HEAD
	}
	return ""
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return len(s) > 0
}

// maxGitWalk bounds the walk up the directory tree looking for a repository.
const maxGitWalk = 64

// findGitDir walks up from dir to the repository's git directory. It follows
// the `gitdir:` pointer that git writes for worktrees and submodules, where
// .git is a file rather than a directory.
func findGitDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for i := 0; i < maxGitWalk; i++ {
		candidate := filepath.Join(abs, ".git")
		if fi, err := os.Stat(candidate); err == nil {
			if fi.IsDir() {
				return candidate
			}
			if resolved := readGitFile(candidate, abs); resolved != "" {
				return resolved
			}
			return ""
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "" // reached the filesystem root
		}
		abs = parent
	}
	return ""
}

// readGitFile resolves a .git file ("gitdir: <path>") to its target, which may
// be relative to the directory holding the file.
func readGitFile(path, base string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4096 {
		return ""
	}
	line := string(bytes.TrimSpace(data))
	target, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return ""
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(base, target)
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		return ""
	}
	return target
}
