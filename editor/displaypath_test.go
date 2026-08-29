package editor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDisplayPathRelativeToOpenedFolder: launching goat on a folder should show
// file names starting from that folder, not from the filesystem root.
func TestDisplayPathRelativeToOpenedFolder(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "internal", "service")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(deep, "handler.go")
	os.WriteFile(path, []byte("package service\n"), 0o644)

	e := &Editor{cfg: DefaultConfig()}
	e.SetRoot(root) // what main() does for a directory argument
	e.openPath(path)

	want := filepath.Join("internal", "service", "handler.go")
	if got := e.displayPath(e.active()); got != want {
		t.Fatalf("displayPath = %q want %q", got, want)
	}
	// The absolute path is still available for anything that needs it.
	if !filepath.IsAbs(e.active().displayName()) {
		t.Fatal("displayName should stay absolute")
	}
}

func TestDisplayPathFileAtRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	os.WriteFile(path, []byte("package main\n"), 0o644)

	e := &Editor{cfg: DefaultConfig()}
	e.SetRoot(root)
	e.openPath(path)
	if got := e.displayPath(e.active()); got != "main.go" {
		t.Fatalf("displayPath = %q want main.go", got)
	}
}

// TestDisplayPathOutsideRootStaysAbsolute: a file opened from elsewhere must
// not be shown as "../../..".
func TestDisplayPathOutsideRootStaysAbsolute(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	other := filepath.Join(base, "elsewhere")
	os.MkdirAll(root, 0o755)
	os.MkdirAll(other, 0o755)
	path := filepath.Join(other, "notes.txt")
	os.WriteFile(path, []byte("x\n"), 0o644)

	e := &Editor{cfg: DefaultConfig()}
	e.SetRoot(root)
	e.openPath(path)
	got := e.displayPath(e.active())
	if !filepath.IsAbs(got) {
		t.Fatalf("displayPath = %q, a file outside the project should stay absolute", got)
	}
	if strings.Contains(got, "..") {
		t.Fatalf("displayPath = %q should not walk up", got)
	}
}

// TestDisplayPathUsesBrowserRootWhenNoFolderGiven covers `goat file.go`, where
// the launch directory is the implicit project root.
func TestDisplayPathUsesBrowserRootWhenNoFolderGiven(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "pkg")
	os.MkdirAll(sub, 0o755)
	path := filepath.Join(sub, "a.go")
	os.WriteFile(path, []byte("package pkg\n"), 0o644)

	e := &Editor{cfg: DefaultConfig()}
	e.browser = &Browser{e: e, expanded: map[string]bool{}, root: root}
	e.openPath(path)
	want := filepath.Join("pkg", "a.go")
	if got := e.displayPath(e.active()); got != want {
		t.Fatalf("displayPath = %q want %q", got, want)
	}
}

func TestDisplayPathUnnamedBuffer(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.newTab()
	if got := e.displayPath(e.active()); got != "New Buffer" {
		t.Fatalf("displayPath = %q", got)
	}
	if got := e.displayPath(nil); got != "" {
		t.Fatalf("nil tab = %q", got)
	}
}

func TestDisplayPathDifferentVolumeStaysAbsolute(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("volume-relative paths are a Windows concern")
	}
	e := &Editor{cfg: DefaultConfig(), root: `C:\project`}
	tb := &Tab{path: `D:\other\file.txt`, name: "file.txt"}
	if got := e.displayPath(tb); got != `D:\other\file.txt` {
		t.Fatalf("displayPath = %q", got)
	}
}

// TestSubdirectoryRootShowsShortPaths: opening a subfolder of a repository
// scopes the names to that subfolder.
func TestSubdirectoryRootShowsShortPaths(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	os.MkdirAll(filepath.Join(src, "ui"), 0o755)
	path := filepath.Join(src, "ui", "view.go")
	os.WriteFile(path, []byte("package ui\n"), 0o644)

	e := &Editor{cfg: DefaultConfig()}
	e.SetRoot(src) // goat ./src
	e.openPath(path)
	want := filepath.Join("ui", "view.go")
	if got := e.displayPath(e.active()); got != want {
		t.Fatalf("displayPath = %q want %q", got, want)
	}
}

func TestStatusShowsRelativePath(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "editor")
	os.MkdirAll(deep, 0o755)
	path := filepath.Join(deep, "render.go")
	os.WriteFile(path, []byte("package editor\n"), 0o644)

	s := simScreen(t, 100, 8)
	e := editorOnScreen(t, s, nil)
	e.SetRoot(root)
	e.openPath(path)
	e.clearMsg()
	e.draw()
	row := screenLine(s, e.height-2)
	want := filepath.Join("editor", "render.go")
	if !strings.Contains(row, want) {
		t.Fatalf("status row %q should show %q", row, want)
	}
	if strings.Contains(row, root) {
		t.Fatalf("status row %q still shows the absolute prefix", row)
	}
}

// TestOpenExistingTabMessageIsRelative keeps the "already open" notice in the
// same form as the status bar.
func TestOpenExistingTabMessageIsRelative(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sub", "f.txt")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("x\n"), 0o644)

	e := &Editor{cfg: DefaultConfig()}
	e.SetRoot(root)
	e.openPath(path)
	e.openPath(path) // second open: switches and reports
	want := filepath.Join("sub", "f.txt")
	if e.msg != want {
		t.Fatalf("message = %q want %q", e.msg, want)
	}
}

// TestSetRootResetsPickerIndex guards the invariant that changing the project
// root invalidates anything cached against the old one.
func TestSetRootResetsPickerIndex(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.fileIndex = &fileIndex{root: "/old", ready: true}
	e.SetRoot(t.TempDir())
	if e.fileIndex != nil {
		t.Fatal("changing the root should drop the cached file index")
	}
}
