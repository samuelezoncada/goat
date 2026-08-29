package editor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// browserWithEditor builds a browser plus the editor it belongs to, so tests
// can drive prompts opened by the file operations.
func browserWithEditor(t *testing.T, root string) (*Editor, *Browser) {
	t.Helper()
	e := &Editor{cfg: DefaultConfig(), width: 60, height: 20}
	b := &Browser{e: e, expanded: map[string]bool{}, showHidden: true, root: root}
	e.browser = b
	b.rebuild()
	return e, b
}

func TestBrowserHiddenToggle(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "visible.txt"), "")
	mustWrite(t, filepath.Join(root, ".hidden"), "")
	_, b := browserWithEditor(t, root)
	if len(b.entries) != 2 {
		t.Fatalf("entries %v", b.entries)
	}
	b.toggleHidden()
	for _, en := range b.entries {
		if strings.HasPrefix(en.name, ".") {
			t.Fatalf("hidden entry still listed: %s", en.name)
		}
	}
	b.toggleHidden()
	if len(b.entries) != 2 {
		t.Fatalf("toggling back should restore hidden files: %v", b.entries)
	}
}

// TestBrowserSurfacesReadError: an unreadable directory used to look empty.
func TestBrowserSurfacesReadError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for this user")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	mustMkdir(t, locked)
	mustWrite(t, filepath.Join(locked, "secret"), "")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	_, b := browserWithEditor(t, root)
	b.sel = 0
	b.expand()
	if b.err == "" {
		t.Fatal("an unreadable directory should report why it is empty")
	}
}

func TestBrowserSymlinkedDirIsADir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	mustMkdir(t, target)
	mustWrite(t, filepath.Join(target, "inner.txt"), "")
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Skip(err)
	}
	_, b := browserWithEditor(t, root)
	var link *Entry
	for i := range b.entries {
		if b.entries[i].name == "link" {
			link = &b.entries[i]
		}
	}
	if link == nil {
		t.Fatal("symlink not listed")
	}
	if !link.isDir || !link.symlink {
		t.Fatalf("symlinked dir: isDir=%v symlink=%v", link.isDir, link.symlink)
	}
	// It can be expanded like a directory.
	for i, en := range b.entries {
		if en.name == "link" {
			b.sel = i
		}
	}
	b.expand()
	found := false
	for _, en := range b.entries {
		if en.name == "inner.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expanding a symlinked dir should list its contents: %v", b.entries)
	}
}

func TestBrowserRootUpFromTopLevel(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	mustMkdir(t, sub)
	mustWrite(t, filepath.Join(sub, "a.txt"), "")
	_, b := browserWithEditor(t, sub)
	if b.root != sub {
		t.Fatalf("root %q", b.root)
	}
	b.sel = 0
	b.collapseOrUp() // a file at depth 0: re-root one level up
	if b.root != root {
		t.Fatalf("root = %q want %q", b.root, root)
	}
	// The old root is selected and expanded, so nothing is lost.
	if !b.expanded[sub] {
		t.Fatal("previous root should stay expanded")
	}
}

func TestBrowserJumpToPrefix(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"alpha", "beta", "gamma"} {
		mustWrite(t, filepath.Join(root, n), "")
	}
	_, b := browserWithEditor(t, root)
	b.sel = 0
	b.jumpToPrefix('g')
	if b.entries[b.sel].name != "gamma" {
		t.Fatalf("selected %q", b.entries[b.sel].name)
	}
	b.jumpToPrefix('b')
	if b.entries[b.sel].name != "beta" {
		t.Fatalf("wrapping search selected %q", b.entries[b.sel].name)
	}
}

func TestBrowserNewFileAndRenameAndDelete(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.txt"), "")
	e, b := browserWithEditor(t, root)

	// New file
	b.newFile()
	e.prompt.input = []rune("made.txt")
	e.promptKey(keyEvent(tcellKeyEnter))
	if _, err := os.Stat(filepath.Join(root, "made.txt")); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Rename it
	b.selectPath(filepath.Join(root, "made.txt"))
	b.rename()
	e.prompt.input = []rune("renamed.txt")
	e.promptKey(keyEvent(tcellKeyEnter))
	if _, err := os.Stat(filepath.Join(root, "renamed.txt")); err != nil {
		t.Fatalf("rename failed: %v", err)
	}

	// Delete needs an explicit y
	b.selectPath(filepath.Join(root, "renamed.txt"))
	b.remove()
	e.prompt.input = []rune("n")
	e.promptKey(keyEvent(tcellKeyEnter))
	if _, err := os.Stat(filepath.Join(root, "renamed.txt")); err != nil {
		t.Fatal("answering n must not delete the file")
	}
	b.selectPath(filepath.Join(root, "renamed.txt"))
	b.remove()
	e.prompt.input = []rune("y")
	e.promptKey(keyEvent(tcellKeyEnter))
	if _, err := os.Stat(filepath.Join(root, "renamed.txt")); !os.IsNotExist(err) {
		t.Fatal("file should be deleted")
	}
}

func TestBrowserNewDir(t *testing.T) {
	root := t.TempDir()
	e, b := browserWithEditor(t, root)
	b.newDir()
	e.prompt.input = []rune("sub")
	e.promptKey(keyEvent(tcellKeyEnter))
	fi, err := os.Stat(filepath.Join(root, "sub"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("dir not created: %v", err)
	}
}

func TestBrowserRenameFollowsOpenTab(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "old.txt")
	mustWrite(t, path, "content\n")
	e, b := browserWithEditor(t, root)
	tb, err := OpenTab(path)
	if err != nil {
		t.Fatal(err)
	}
	e.tabs = []*Tab{tb}
	b.selectPath(path)
	b.rename()
	e.prompt.input = []rune("new.txt")
	e.promptKey(keyEvent(tcellKeyEnter))
	if filepath.Base(tb.path) != "new.txt" || tb.name != "new.txt" {
		t.Fatalf("open tab still points at %q", tb.path)
	}
}

func TestBrowserDeleteNonEmptyDirRefused(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "full")
	mustMkdir(t, sub)
	mustWrite(t, filepath.Join(sub, "x"), "")
	e, b := browserWithEditor(t, root)
	b.selectPath(sub)
	b.remove()
	e.prompt.input = []rune("y")
	e.promptKey(keyEvent(tcellKeyEnter))
	if _, err := os.Stat(sub); err != nil {
		t.Fatal("a non-empty directory must not be removed")
	}
	if !strings.Contains(e.msg, "Error deleting") {
		t.Fatalf("expected an error message, got %q", e.msg)
	}
}
