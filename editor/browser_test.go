package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"

	"goat/syntax"
)

// newBrowserAt builds a browser rooted at base with a fresh expansion set.
func newBrowserAt(base string) *Browser {
	e := &Editor{cfg: DefaultConfig()}
	b := NewBrowser(e)
	b.root = base
	b.expanded = map[string]bool{}
	b.rebuild()
	return b
}

func names(b *Browser) []string {
	out := make([]string, 0, len(b.entries))
	for _, en := range b.entries {
		out = append(out, en.name)
	}
	return out
}

func TestBrowserTreeFlatten(t *testing.T) {
	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "src", "pkg"))
	mustWrite(t, filepath.Join(base, "a.go"), "")
	mustWrite(t, filepath.Join(base, ".hidden"), "")
	mustWrite(t, filepath.Join(base, "src", "c.go"), "")
	mustWrite(t, filepath.Join(base, "src", "pkg", "b.go"), "")

	b := newBrowserAt(base)
	// dirs before files, hidden after visible, nothing expanded
	if got := names(b); len(got) != 3 || got[0] != "src" || got[1] != "a.go" || got[2] != ".hidden" {
		t.Fatalf("flat list = %v", got)
	}
	if b.entries[0].depth != 0 {
		t.Fatalf("src depth = %d", b.entries[0].depth)
	}

	// expand src: its children appear inline at depth 1
	b.expanded[filepath.Join(base, "src")] = true
	b.rebuild()
	got := names(b)
	want := []string{"src", "pkg", "c.go", "a.go", ".hidden"}
	if len(got) != len(want) {
		t.Fatalf("expanded list = %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expanded list = %v want %v", got, want)
		}
	}
	if b.entries[1].depth != 1 || b.entries[2].depth != 1 {
		t.Fatalf("child depths = %d,%d want 1,1", b.entries[1].depth, b.entries[2].depth)
	}
}

func TestBrowserToggleExpand(t *testing.T) {
	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "src"))
	mustWrite(t, filepath.Join(base, "a.go"), "")
	mustWrite(t, filepath.Join(base, "src", "b.go"), "")

	b := newBrowserAt(base)
	idx := 0 // src is the first entry
	if b.entries[idx].name != "src" || !b.entries[idx].isDir {
		t.Fatalf("entry 0 = %+v", b.entries[idx])
	}
	b.sel = idx
	b.enter() // expand
	if len(b.entries) != 3 {
		t.Fatalf("after expand entries = %v", names(b))
	}
	b.enter() // collapse
	if len(b.entries) != 2 {
		t.Fatalf("after collapse entries = %v", names(b))
	}
}

func TestBrowserCollapseOrUp(t *testing.T) {
	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "src", "pkg"))
	mustWrite(t, filepath.Join(base, "src", "pkg", "a.go"), "")

	b := newBrowserAt(base)
	b.expanded[filepath.Join(base, "src")] = true
	b.expanded[filepath.Join(base, "src", "pkg")] = true
	b.rebuild()
	// list: src, pkg/, a.go
	if len(b.entries) != 3 {
		t.Fatalf("entries = %v", names(b))
	}

	// select a.go (a file) -> collapseOrUp selects the parent (pkg) row
	b.sel = 2
	b.collapseOrUp()
	if b.entries[b.sel].name != "pkg" {
		t.Fatalf("parent jump: sel=%d entries=%v", b.sel, names(b))
	}

	// select pkg (an expanded dir) -> collapseOrUp collapses it
	b.sel = 1
	b.collapseOrUp()
	if len(b.entries) != 2 {
		t.Fatalf("after collapse entries = %v", names(b))
	}

	// at depth 0 a collapsed dir is a no-op (stays selected)
	b.sel = 0
	b.collapseOrUp()
	if b.entries[b.sel].name != "src" {
		t.Fatalf("depth-0 collapse moved selection to %d", b.sel)
	}
}

func TestBrowserHeavyDirsListedCollapsed(t *testing.T) {
	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "node_modules", "dep"))
	mustWrite(t, filepath.Join(base, "node_modules", "dep", "i.js"), "")
	mustWrite(t, filepath.Join(base, "main.go"), "")

	b := newBrowserAt(base)
	got := names(b)
	// node_modules appears like any dir (dirs first), not walked into
	if len(got) != 2 || got[0] != "node_modules" || got[1] != "main.go" {
		t.Fatalf("entries = %v", got)
	}
}

func TestBrowserOpenDirSetsRoot(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	e := &Editor{}
	b := NewBrowser(e)
	b.OpenDir(sub)
	if b.root != sub {
		t.Fatalf("root = %s want %s", b.root, sub)
	}
	if !b.open || e.focus != FocusBrowser {
		t.Fatalf("browser not opened/focused")
	}
}

func TestBrowserHiddenWhileEditing(t *testing.T) {
	e := &Editor{}
	e.browser = NewBrowser(e)
	e.browser.open = true
	e.focus = FocusBrowser
	e.focusText()
	if e.browser.open {
		t.Fatal("browser should hide when focus returns to text")
	}
	if e.focus != FocusText {
		t.Fatalf("focus=%v want FocusText", e.focus)
	}
}

func TestDrawBrowserHidesWhenTextFocused(t *testing.T) {
	e := &Editor{}
	e.browser = NewBrowser(e)
	e.browser.open = true
	e.focus = FocusText
	e.drawBrowser()
	if e.browser.open {
		t.Fatal("drawBrowser must hide browser when text is focused")
	}
}

func TestBrowserEnterOpensFile(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "f.go")
	mustWrite(t, path, "package f\n")
	e := &Editor{tabs: []*Tab{NewTab()}}
	b := NewBrowser(e)
	e.browser = b
	b.root = base
	b.expanded = map[string]bool{}
	b.rebuild()

	b.sel = 0
	if b.entries[b.sel].name != "f.go" {
		t.Fatalf("entry 0 = %+v", b.entries[b.sel])
	}
	b.enter()
	if e.browser.open || e.focus != FocusText {
		t.Fatal("browser should close after opening a file")
	}
	if len(e.tabs) != 1 || e.tabs[0].name != "f.go" {
		t.Fatalf("file not opened: %+v", e.tabs)
	}
	e.tabs[0].close()
}

func TestBrowserRenderSmoke(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "src"))
	mustWrite(t, filepath.Join(base, "a.go"), "")
	mustWrite(t, filepath.Join(base, "src", "b.go"), "")

	e := &Editor{screen: scr, theme: syntax.DefaultTheme(), width: 80, height: 24}
	e.allocFrame()
	e.browser = NewBrowser(e)
	e.browser.root = base
	e.browser.expanded = map[string]bool{}
	e.browser.open = true
	e.focus = FocusBrowser
	e.browser.rebuild()
	e.browser.sel = 0

	e.drawBrowser()
	scr.Show()

	cells, w, _ := scr.GetContents()
	// first row below the header: "▸ src/" with the marker at column 1
	if string(cells[(e.mainTop()+1)*w+1].Runes) != "\u25b8" {
		t.Fatalf("row marker = %q want ▸", cells[(e.mainTop()+1)*w+1].Runes)
	}
}
