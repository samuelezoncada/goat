package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRepo creates a directory holding a .git directory with the given HEAD.
func fakeRepo(t *testing.T, head string) string {
	t.Helper()
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(head), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParseHEAD(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ref: refs/heads/main\n", "main"},
		{"ref: refs/heads/feature/nested-name\n", "feature/nested-name"},
		{"ref: refs/heads/main", "main"},
		{"  ref:   refs/heads/spaced  \n", "spaced"},
		{"ref: refs/remotes/origin/main\n", "refs/remotes/origin/main"},
		{"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678\n", "a1b2c3d"},
		{"", ""},
		{"\n", ""},
		{"garbage that is not a ref\n", ""},
		{"abc\n", ""}, // too short to be a commit
	}
	for _, c := range cases {
		if got := parseHEAD([]byte(c.in)); got != c.want {
			t.Errorf("parseHEAD(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestGitBranchFromRepo(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/my-branch\n")
	if got := gitBranch(root); got != "my-branch" {
		t.Fatalf("branch = %q want my-branch", got)
	}
}

func TestGitBranchWalksUp(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/trunk\n")
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitBranch(deep); got != "trunk" {
		t.Fatalf("branch from a subdirectory = %q want trunk", got)
	}
}

func TestGitBranchDetachedHead(t *testing.T) {
	root := fakeRepo(t, "0123456789abcdef0123456789abcdef01234567\n")
	if got := gitBranch(root); got != "0123456" {
		t.Fatalf("detached HEAD = %q want the short commit", got)
	}
}

// TestGitBranchWorktreeIndirection covers a linked worktree or a submodule,
// where .git is a file pointing at the real git directory.
func TestGitBranchWorktreeIndirection(t *testing.T) {
	base := t.TempDir()
	realGit := filepath.Join(base, "store", "worktrees", "wt")
	if err := os.MkdirAll(realGit, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(realGit, "HEAD"), []byte("ref: refs/heads/wt-branch\n"), 0o644)

	work := filepath.Join(base, "work")
	os.MkdirAll(work, 0o755)
	os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: "+realGit+"\n"), 0o644)
	if got := gitBranch(work); got != "wt-branch" {
		t.Fatalf("worktree branch = %q want wt-branch", got)
	}

	// The pointer may also be relative to the directory holding the file.
	rel := filepath.Join(base, "relwork")
	os.MkdirAll(rel, 0o755)
	relPath, err := filepath.Rel(rel, realGit)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(rel, ".git"), []byte("gitdir: "+relPath+"\n"), 0o644)
	if got := gitBranch(rel); got != "wt-branch" {
		t.Fatalf("relative gitdir branch = %q", got)
	}
}

func TestGitBranchOutsideRepo(t *testing.T) {
	// A temp dir with no .git anywhere up to the root.
	dir := t.TempDir()
	if got := gitBranch(dir); got != "" {
		// /tmp is not usually inside a repository; skip if this machine differs.
		t.Skipf("temp dir is inside a repository (%q); nothing to assert", got)
	}
	if got := gitBranch(""); got != "" {
		t.Fatalf("empty dir = %q", got)
	}
}

func TestGitBranchBrokenRepo(t *testing.T) {
	root := t.TempDir()
	// .git exists but holds no HEAD.
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	if got := gitBranch(root); got != "" {
		t.Fatalf("missing HEAD = %q want empty", got)
	}
	// .git is a file with nothing useful in it.
	other := t.TempDir()
	os.WriteFile(filepath.Join(other, ".git"), []byte("not a pointer\n"), 0o644)
	if got := gitBranch(other); got != "" {
		t.Fatalf("bogus .git file = %q want empty", got)
	}
}

// TestBranchForCaches keeps the status bar from re-reading HEAD on every frame.
func TestBranchForCaches(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/first\n")
	e := &Editor{cfg: DefaultConfig()}
	if got := e.branchFor(root); got != "first" {
		t.Fatalf("branch = %q", got)
	}
	// Change HEAD: the cached value is reused inside the TTL.
	os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/second\n"), 0o644)
	if got := e.branchFor(root); got != "first" {
		t.Fatalf("branch = %q, the lookup should be cached", got)
	}
	// Once the entry ages out, the new branch shows up.
	e.git.at = time.Now().Add(-2 * gitTTL)
	if got := e.branchFor(root); got != "second" {
		t.Fatalf("branch = %q want second after the TTL", got)
	}
	// A different directory is not answered from the cache.
	other := fakeRepo(t, "ref: refs/heads/elsewhere\n")
	if got := e.branchFor(other); got != "elsewhere" {
		t.Fatalf("branch = %q for another directory", got)
	}
}

func TestGitContextDirPrefersActiveFile(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/ctx\n")
	path := filepath.Join(root, "sub", "f.txt")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("x\n"), 0o644)

	e := &Editor{cfg: DefaultConfig()}
	e.openPath(path)
	if got := e.gitContextDir(); got != filepath.Dir(path) {
		t.Fatalf("context dir = %q want %q", got, filepath.Dir(path))
	}
	if got := e.branchFor(e.gitContextDir()); got != "ctx" {
		t.Fatalf("branch = %q want ctx", got)
	}

	// With an unnamed buffer, the project root is used instead.
	e2 := &Editor{cfg: DefaultConfig(), root: root}
	e2.newTab()
	if got := e2.gitContextDir(); got != root {
		t.Fatalf("context dir = %q want the project root %q", got, root)
	}
}

// TestGitBranchAgreesWithGit compares the HEAD parser against real git output
// for a normal branch, a detached HEAD and a linked worktree.
func TestGitBranchAgreesWithGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	repo := base + "/repo"
	run(base, "init", "-q", "-b", "feature/xyz", repo)
	run(repo, "config", "user.email", "t@e.st")
	run(repo, "config", "user.name", "T")
	run(repo, "commit", "-q", "--allow-empty", "-m", "init")

	if got := gitBranch(repo); got != "feature/xyz" {
		t.Fatalf("branch = %q want feature/xyz", got)
	}

	// A linked worktree: .git is a file pointing into the main repo.
	wt := base + "/wt"
	run(repo, "worktree", "add", "-q", "-b", "wt-branch", wt)
	if got := gitBranch(wt); got != "wt-branch" {
		t.Fatalf("worktree branch = %q want wt-branch", got)
	}

	// Detached HEAD: goat shows the abbreviated commit.
	sha := run(repo, "rev-parse", "HEAD")
	run(repo, "checkout", "-q", "--detach")
	got := gitBranch(repo)
	if got != sha[:7] {
		t.Fatalf("detached HEAD = %q want %q", got, sha[:7])
	}
}

// --- status bar rendering ------------------------------------------------

func TestStatusShowsBranch(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/feature/status\n")
	path := filepath.Join(root, "f.txt")
	os.WriteFile(path, []byte("hello\n"), 0o644)

	s := simScreen(t, 100, 8)
	e := editorOnScreen(t, s, nil)
	e.openPath(path)
	e.clearMsg() // drop the "Read 1 line" notice so the filename shows
	e.draw()

	row := screenLine(s, e.height-2)
	if !strings.Contains(row, "git:feature/status") {
		t.Fatalf("status row %q does not show the branch", row)
	}
	// The path may be elided from the front, but never the filename itself.
	if !strings.Contains(row, "f.txt") {
		t.Fatalf("status row %q lost the filename", row)
	}
}

func TestElideLeftKeepsTheEnd(t *testing.T) {
	if got := elideLeft("/very/long/path/to/file.go", 12); got != "…/to/file.go" {
		t.Fatalf("elideLeft = %q", got)
	}
	if got := elideLeft("short", 12); got != "short" {
		t.Fatalf("no elision needed: %q", got)
	}
	if got := elideLeft("abc", 0); got != "" {
		t.Fatalf("zero width: %q", got)
	}
	if got := len([]rune(elideLeft("/a/b/c/d/e", 4))); got != 4 {
		t.Fatalf("elided width = %d want 4", got)
	}
}

func TestStatusNoBranchOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	if gitBranch(dir) != "" {
		t.Skip("temp dir is inside a repository")
	}
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("hello\n"), 0o644)

	s := simScreen(t, 100, 8)
	e := editorOnScreen(t, s, nil)
	e.openPath(path)
	e.clearMsg()
	e.draw()
	if row := screenLine(s, e.height-2); strings.Contains(row, "git:") {
		t.Fatalf("status row %q shows a branch outside a repository", row)
	}
}

// TestStatusBranchSurvivesNarrowScreen: the path gives way before the branch
// and the read-only flag, which are the short, high-value parts, and the
// filename survives because the path is elided from the front.
func TestStatusBranchSurvivesNarrowScreen(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/main\n")
	deep := filepath.Join(root, "very", "deeply", "nested", "directory", "tree")
	os.MkdirAll(deep, 0o755)
	path := filepath.Join(deep, "a-rather-long-filename.txt")
	os.WriteFile(path, []byte("hello\n"), 0o644)

	s := simScreen(t, 60, 8)
	e := editorOnScreen(t, s, nil)
	e.openPath(path)
	e.clearMsg()
	e.draw()
	row := screenLine(s, e.height-2)
	if !strings.Contains(row, "git:main") {
		t.Fatalf("status row %q dropped the branch instead of the path", row)
	}
	if !strings.Contains(row, "a-rather-long-filename.txt") {
		t.Fatalf("status row %q lost the filename; the path should be elided", row)
	}
}

func TestStatusMessageHidesBranch(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/main\n")
	path := filepath.Join(root, "f.txt")
	os.WriteFile(path, []byte("hello\n"), 0o644)

	s := simScreen(t, 100, 8)
	e := editorOnScreen(t, s, nil)
	e.openPath(path)
	e.statusf("Wrote 1 line")
	e.draw()
	row := screenLine(s, e.height-2)
	if !strings.Contains(row, "Wrote 1 line") {
		t.Fatalf("status row %q lost the message", row)
	}
	if strings.Contains(row, "git:") {
		t.Fatalf("status row %q should show the message alone", row)
	}
}

func TestStatusRightDegradesOnNarrowScreen(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/main\n")
	path := filepath.Join(root, "code.go")
	os.WriteFile(path, []byte("package main\n"), 0o644)

	// Wide screen: the descriptive fields are shown.
	wide := simScreen(t, 120, 8)
	e := editorOnScreen(t, wide, nil)
	e.openPath(path)
	e.clearMsg()
	e.draw()
	row := screenLine(wide, e.height-2)
	if !strings.Contains(row, "UTF-8") || !strings.Contains(row, "Ln 1/1") {
		t.Fatalf("wide status row %q should carry the full fields", row)
	}

	// Narrow screen: encoding and language give way, the position stays.
	narrow := simScreen(t, 52, 8)
	e2 := editorOnScreen(t, narrow, nil)
	e2.openPath(path)
	e2.clearMsg()
	e2.draw()
	row2 := screenLine(narrow, e2.height-2)
	if strings.Contains(row2, "UTF-8") {
		t.Fatalf("narrow status row %q should drop the encoding field", row2)
	}
	if !strings.Contains(row2, "Ln 1/1") {
		t.Fatalf("narrow status row %q should keep the position", row2)
	}
	if !strings.Contains(row2, "code.go") || !strings.Contains(row2, "git:main") {
		t.Fatalf("narrow status row %q lost the filename or branch", row2)
	}
}

func TestStatusBranchWithDirtyAndReadOnly(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/main\n")
	path := filepath.Join(root, "f.txt")
	os.WriteFile(path, []byte("hello\n"), 0o644)

	s := simScreen(t, 110, 8)
	e := editorOnScreen(t, s, nil)
	e.openPath(path)
	e.clearMsg()
	tb := e.active()
	tb.cur = Pos{0, 5}
	tb.insertRune('!')
	tb.readOnly = true
	e.draw()
	row := screenLine(s, e.height-2)
	for _, want := range []string{"*", "f.txt", "git:main", "[read-only]"} {
		if !strings.Contains(row, want) {
			t.Fatalf("status row %q missing %q", row, want)
		}
	}
	// The branch comes before the flag, next to the name it describes.
	if strings.Index(row, "git:main") > strings.Index(row, "[read-only]") {
		t.Fatalf("unexpected field order: %q", row)
	}
}

func TestStatusBranchCanBeDisabled(t *testing.T) {
	root := fakeRepo(t, "ref: refs/heads/main\n")
	path := filepath.Join(root, "f.txt")
	os.WriteFile(path, []byte("hello\n"), 0o644)

	s := simScreen(t, 100, 8)
	e := editorOnScreen(t, s, nil)
	e.cfg.GitBranch = false
	e.openPath(path)
	e.clearMsg()
	e.draw()
	if row := screenLine(s, e.height-2); strings.Contains(row, "git:") {
		t.Fatalf("gitbranch = false should hide the branch: %q", row)
	}
}
