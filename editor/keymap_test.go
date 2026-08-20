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
