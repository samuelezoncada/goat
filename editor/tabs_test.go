package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestCloseTabCleanNoPrompt(t *testing.T) {
	tb := newTestTab("hello")
	tb.path = "/x/a.go"
	e := &Editor{tabs: []*Tab{tb}}
	e.cur = 0
	e.closeCurrentTab()
	if e.mode != ModeNormal {
		t.Fatalf("clean tab should close without prompting, mode=%v", e.mode)
	}
	if len(e.tabs) != 1 || e.tabs[0].name != "New Buffer" {
		t.Fatalf("clean tab should be replaced by empty buffer: %+v", e.tabs)
	}
}

func TestCloseTabPristineExits(t *testing.T) {
	tb := NewTab()
	e := &Editor{tabs: []*Tab{tb}}
	e.cur = 0
	e.closeCurrentTab()
	if e.running != false {
		t.Fatal("closing the pristine empty buffer should exit")
	}
}

func TestCloseTabDirtyPrompts(t *testing.T) {
	tb := newTestTab("hello")
	tb.dirty = true
	e := &Editor{tabs: []*Tab{tb}}
	e.cur = 0
	e.closeCurrentTab()
	if e.mode != ModePrompt {
		t.Fatalf("dirty tab should prompt, mode=%v", e.mode)
	}
	// answer "n" (discard) then Enter
	e.promptKey(tcell.NewEventKey(tcell.KeyRune, 'n', tcell.ModNone))
	e.promptKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if len(e.tabs) != 1 || e.tabs[0].name != "New Buffer" {
		t.Fatalf("discard should close the tab: %+v", e.tabs)
	}
}

func TestCloseTabDirtyCancelKeepsTab(t *testing.T) {
	tb := newTestTab("hello")
	tb.dirty = true
	e := &Editor{tabs: []*Tab{tb}}
	e.cur = 0
	e.closeCurrentTab()
	e.promptKey(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))
	e.promptKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if len(e.tabs) != 1 || e.tabs[0] != tb || tb.dirty == false {
		t.Fatalf("cancel should keep the tab: %+v", e.tabs)
	}
}

func TestTabBarClickClose(t *testing.T) {
	a := newTestTab("aaa")
	a.name = "a.go"
	b := newTestTab("bbb")
	b.name = "b.go"
	e := &Editor{tabs: []*Tab{a, b}}
	e.cur = 0
	e.tabRects = []tabRect{
		{index: 0, tabX0: 0, tabX1: 4, closeX0: 4, closeX1: 5},
		{index: 1, tabX0: 5, tabX1: 9, closeX0: 9, closeX1: 10},
	}
	if !e.handleTabBarClick(9) {
		t.Fatal("close click not handled")
	}
	if len(e.tabs) != 1 || e.tabs[0].name != "a.go" {
		t.Fatalf("b.go not closed: %+v", e.tabs)
	}
}

func TestTabBarClickSwitch(t *testing.T) {
	a := newTestTab("aaa")
	a.name = "a.go"
	b := newTestTab("bbb")
	b.name = "b.go"
	e := &Editor{tabs: []*Tab{a, b}}
	e.cur = 0
	e.tabRects = []tabRect{{index: 1, tabX0: 5, tabX1: 9, closeX0: 9, closeX1: 10}}
	e.handleTabBarClick(6)
	if e.cur != 1 {
		t.Fatalf("cur=%d want 1", e.cur)
	}
}

func TestBrowserClickSelectsThenDoubleClickOpens(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Editor{tabs: []*Tab{NewTab()}, width: 100, height: 30}
	e.browser = NewBrowser(e)
	e.browser.OpenDir(dir)

	idx := -1
	for i, en := range e.browser.entries {
		if en.name == "f.go" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("f.go not listed: %+v", e.browser.entries)
	}
	y := e.mainTop() + 1 + idx
	// first click selects, does not open
	e.handleMouse(tcell.NewEventMouse(5, y, tcell.Button1, tcell.ModNone))
	if !e.browser.open || e.focus != FocusBrowser {
		t.Fatalf("browser should stay open after select click")
	}
	if e.browser.sel != idx {
		t.Fatalf("sel = %d want %d", e.browser.sel, idx)
	}
	// second click within the window opens the file and closes the browser
	e.handleMouse(tcell.NewEventMouse(5, y, tcell.Button1, tcell.ModNone))
	if e.browser.open {
		t.Fatal("browser should close after double-click opening a file")
	}
	if len(e.tabs) != 1 || e.tabs[0].name != "f.go" {
		t.Fatalf("file not opened: %+v", e.tabs)
	}
	e.tabs[0].close()
}
