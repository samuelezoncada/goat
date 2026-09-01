package editor

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"goat/syntax"
)

func TestAltScroll(t *testing.T) {
	tb := newTestTab("0\n1\n2\n3\n4\n5\n6\n7")
	tb.top = 0
	e := &Editor{tabs: []*Tab{tb}, width: 80, height: 30, theme: syntax.DefaultTheme()}
	e.cur = 0

	altDown := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModAlt)
	altUp := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt)

	e.handleNormalKey(altDown)
	if tb.top != 1 {
		t.Fatalf("Alt+Down top = %d want 1", tb.top)
	}
	if tb.cur.Line != 0 {
		t.Fatalf("Alt+Down moved cursor to line %d", tb.cur.Line)
	}
	e.handleNormalKey(altUp)
	if tb.top != 0 {
		t.Fatalf("Alt+Up top = %d want 0", tb.top)
	}
	// Alt+Up clamps at the top
	e.handleNormalKey(altUp)
	if tb.top != 0 {
		t.Fatalf("Alt+Up clamp top = %d want 0", tb.top)
	}
	// Alt+Down clamps at the last line
	for i := 0; i < tb.lineCount()+3; i++ {
		e.handleNormalKey(altDown)
	}
	if tb.top != tb.lineCount()-1 {
		t.Fatalf("Alt+Down clamp top = %d want %d", tb.top, tb.lineCount()-1)
	}
	// cursor stays put throughout
	if tb.cur.Line != 0 {
		t.Fatalf("cursor moved to line %d", tb.cur.Line)
	}
}

func TestCmdArrowMovement(t *testing.T) {
	tb := newTestTab("0\n1\n2\n3\n4")
	tb.cur = Pos{2, 1}
	tb.destCol = 1
	e := &Editor{tabs: []*Tab{tb}, width: 80, height: 30, theme: syntax.DefaultTheme()}
	e.cur = 0

	// ⌘↑ moves to the start of the buffer.
	e.handleNormalKey(cmdKeyEvent(tcell.KeyUp))
	if tb.cur.Line != 0 || tb.cur.Col != 0 {
		t.Fatalf("⌘↑ cursor = %+v want {0 0}", tb.cur)
	}
	// ⌘↓ moves to the end of the buffer.
	e.handleNormalKey(cmdKeyEvent(tcell.KeyDown))
	if tb.cur.Line != 4 || tb.cur.Col != 1 {
		t.Fatalf("⌘↓ cursor = %+v want {4 1}", tb.cur)
	}
	// ⌘← moves to home.
	tb.cur = Pos{2, 3}
	tb.destCol = 3
	e.handleNormalKey(cmdKeyEvent(tcell.KeyLeft))
	if tb.cur.Line != 2 || tb.cur.Col != 0 {
		t.Fatalf("⌘← cursor = %+v want {2 0}", tb.cur)
	}
	// ⌘→ moves to end of line.
	tb.cur = Pos{2, 0}
	tb.destCol = 0
	e.handleNormalKey(cmdKeyEvent(tcell.KeyRight))
	if tb.cur.Line != 2 || tb.cur.Col != 1 {
		t.Fatalf("⌘→ cursor = %+v want {2 1}", tb.cur)
	}
	// ⌘⇧↓ extends the selection to the end of the buffer.
	tb.cur = Pos{1, 0}
	tb.destCol = 0
	tb.mark = &Pos{0, 0}
	e.handleNormalKey(cmdShiftKeyEvent(tcell.KeyDown))
	if tb.cur.Line != 4 || !tb.hasSelection() {
		t.Fatalf("⌘⇧↓ cursor = %+v, selection = %v", tb.cur, tb.mark)
	}
}

func TestCmdRuneActions(t *testing.T) {
	tb := newTestTab("hello")
	tb.cur = Pos{0, 2}
	tb.destCol = 2
	e := &Editor{tabs: []*Tab{tb}, width: 80, height: 30, theme: syntax.DefaultTheme()}
	e.cur = 0

	// ⌘A selects the whole buffer.
	e.handleNormalKey(cmdRuneEvent('a'))
	if !tb.hasSelection() || tb.cur.Line != 0 || tb.cur.Col != 5 {
		t.Fatalf("⌘A cur = %+v mark = %v", tb.cur, tb.mark)
	}

	// ⌘Z undoes an edit; ⇧⌘Z redoes it.
	tb.cur = Pos{0, 5}
	tb.destCol = 5
	tb.mark = nil
	tb.insertRunes(0, 5, []rune("!"))
	e.handleNormalKey(cmdRuneEvent('z'))
	if got := joinLines(t, tb); got != "hello" {
		t.Fatalf("⌘Z text = %q want hello", got)
	}
	e.handleNormalKey(cmdShiftRuneEvent('z'))
	if got := joinLines(t, tb); got != "hello!" {
		t.Fatalf("⇧⌘Z text = %q want hello!", got)
	}

	// ⌘V pastes the clipboard.
	e.handleNormalKey(cmdRuneEvent('z'))
	if got := joinLines(t, tb); got != "hello" {
		t.Fatalf("undo for ⌘V text = %q want hello", got)
	}
	tb.cur = Pos{0, 5}
	tb.destCol = 5
	tb.mark = nil
	e.clip = []rune(" world")
	e.handleNormalKey(cmdRuneEvent('v'))
	if got := joinLines(t, tb); got != "hello world" {
		t.Fatalf("⌘V text = %q want hello world", got)
	}
}

func TestCmdTabKeys(t *testing.T) {
	a := newTestTab("aaa")
	b := newTestTab("bbb")
	e := &Editor{tabs: []*Tab{a, b}, width: 80, height: 30, theme: syntax.DefaultTheme()}
	e.cur = 0

	// ⌘2 goes to the second tab.
	e.handleNormalKey(cmdRuneEvent('2'))
	if e.cur != 1 {
		t.Fatalf("⌘2 cur = %d want 1", e.cur)
	}
	// ⌘T opens a new tab.
	e.handleNormalKey(cmdRuneEvent('t'))
	if len(e.tabs) != 3 {
		t.Fatalf("⌘T tabs = %d want 3", len(e.tabs))
	}
	// ⌘W closes it.
	e.handleNormalKey(cmdRuneEvent('w'))
	if len(e.tabs) != 2 {
		t.Fatalf("⌘W tabs = %d want 2", len(e.tabs))
	}
}
