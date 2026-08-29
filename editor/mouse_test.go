package editor

import (
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// mouseEditor builds an editor with one tab and a fixed viewport for mouse tests.
func mouseEditor(t *testing.T, content string) (*Editor, *Tab) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.go")
	tb := openTabAt(t, path, content)
	e := &Editor{tabs: []*Tab{tb}, width: 100, height: 30}
	e.cur = 0
	tb.top = 0
	tb.left = 0
	return e, tb
}

// cellXY returns the terminal cell for a (line, col) in the text pane.
func cellXY(e *Editor, t *Tab, line, col int) (int, int) {
	return e.gutterWidth() + displayCol(t.line(line), col, 8), e.mainTop() + line - t.top
}

func TestMouseClickMovesCursorNoSelection(t *testing.T) {
	e, tb := mouseEditor(t, "package x\nfunc Foo() {}\nbar\n")
	x, y := cellXY(e, tb, 2, 1)
	e.handleMouse(tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone))
	e.handleMouse(tcell.NewEventMouse(x, y, tcell.ButtonNone, tcell.ModNone))
	if tb.cur.Line != 2 || tb.cur.Col != 1 {
		t.Fatalf("cursor = %+v want 2,1", tb.cur)
	}
	if tb.mark != nil {
		t.Fatalf("plain click must not leave a selection: %+v", tb.mark)
	}
}

func TestMouseDragSelects(t *testing.T) {
	e, tb := mouseEditor(t, "package x\nfunc Foo() {}\nbar\n")
	x1, y1 := cellXY(e, tb, 0, 1)
	x2, y2 := cellXY(e, tb, 2, 2)
	e.handleMouse(tcell.NewEventMouse(x1, y1, tcell.Button1, tcell.ModNone))    // press
	e.handleMouse(tcell.NewEventMouse(x2, y2, tcell.Button1, tcell.ModNone))    // drag
	e.handleMouse(tcell.NewEventMouse(x2, y2, tcell.ButtonNone, tcell.ModNone)) // release
	if tb.mark == nil {
		t.Fatal("drag should leave a selection")
	}
	if *tb.mark != (Pos{0, 1}) {
		t.Fatalf("mark = %+v want 0,1", tb.mark)
	}
	if tb.cur.Line != 2 || tb.cur.Col != 2 {
		t.Fatalf("cur = %+v want 2,2", tb.cur)
	}
	sel := e.selection()
	if sel[0] != 0 || sel[1] != 1 || sel[2] != 2 || sel[3] != 2 {
		t.Fatalf("selection = %v want 0,1..2,2", sel)
	}
}

func TestMouseClickClearsSelection(t *testing.T) {
	e, tb := mouseEditor(t, "package x\nfunc Foo() {}\nbar\n")
	tb.mark = &Pos{0, 0}
	tb.cur = Pos{2, 2}
	x, y := cellXY(e, tb, 1, 1)
	e.handleMouse(tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone))
	e.handleMouse(tcell.NewEventMouse(x, y, tcell.ButtonNone, tcell.ModNone))
	if tb.mark != nil {
		t.Fatal("click should clear a previous selection")
	}
	if tb.cur.Line != 1 || tb.cur.Col != 1 {
		t.Fatalf("cursor = %+v want 1,1", tb.cur)
	}
}

func TestDoubleClickSelectsWord(t *testing.T) {
	e, tb := mouseEditor(t, "package x\nfunc Foo() {}\nbar\n")
	x, y := cellXY(e, tb, 1, 6) // inside "Foo"
	e.handleMouse(tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone))
	e.handleMouse(tcell.NewEventMouse(x, y, tcell.ButtonNone, tcell.ModNone))
	e.handleMouse(tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone)) // second click
	e.handleMouse(tcell.NewEventMouse(x, y, tcell.ButtonNone, tcell.ModNone))
	if tb.mark == nil {
		t.Fatal("double-click should select the word")
	}
	sel := e.selection()
	line := []rune(tb.line(1))
	start, end := wordRange(line, 6)
	if sel[1] != start || sel[3] != end || sel[0] != 1 || sel[2] != 1 {
		t.Fatalf("selection = %v want word 1:%d..1:%d", sel, start, end)
	}
}

func TestDoubleClickNeedsSameLine(t *testing.T) {
	e, tb := mouseEditor(t, "package x\nfunc Foo() {}\nbar\n")
	x1, y1 := cellXY(e, tb, 1, 6)
	x2, y2 := cellXY(e, tb, 2, 1)
	e.handleMouse(tcell.NewEventMouse(x1, y1, tcell.Button1, tcell.ModNone))
	e.handleMouse(tcell.NewEventMouse(x1, y1, tcell.ButtonNone, tcell.ModNone))
	e.handleMouse(tcell.NewEventMouse(x2, y2, tcell.Button1, tcell.ModNone)) // different line
	e.handleMouse(tcell.NewEventMouse(x2, y2, tcell.ButtonNone, tcell.ModNone))
	if tb.mark != nil {
		t.Fatal("clicks on different lines must not select a word")
	}
}
