package editor

import (
	"testing"
)

// checkCur asserts the cursor is always within the buffer bounds.
func checkCur(t *testing.T, tb *Tab) {
	t.Helper()
	if tb.cur.Line < 0 || tb.cur.Line >= tb.lineCount() {
		t.Fatalf("cursor line %d out of range (lines=%d)", tb.cur.Line, tb.lineCount())
	}
	l := len(tb.line(tb.cur.Line))
	if tb.cur.Col < 0 || tb.cur.Col > l {
		t.Fatalf("cursor col %d out of range for line %d (len=%d)", tb.cur.Col, tb.cur.Line, l)
	}
}

func TestEditStressNoPanic(t *testing.T) {
	tb := newTestTab("alpha beta gamma\nline two\nline three")
	e := &Editor{tabs: []*Tab{tb}, clip: []rune("XYZ\npqr")}
	tb.cur = Pos{0, 0}

	ops := []func(){
		func() { tb.insertRune('a') },
		func() { tb.insertNewline() },
		func() { tb.insertRunes(tb.cur.Line, tb.cur.Col, []rune("multi\nline\npaste")) },
		func() { tb.backspace() },
		func() { tb.deleteForward() },
		func() { tb.undo() },
		func() { tb.redo() },
		func() { e.cut() },
		func() { e.uncut() },
		func() { e.justify() },
		func() { tb.moveLeft() },
		func() { tb.moveRight() },
		func() { tb.moveUp() },
		func() { tb.moveDown() },
		func() { tb.home() },
		func() { tb.end() },
		func() { tb.wordLeft() },
		func() { tb.wordRight() },
		func() { e.selectAll() },
		func() { e.copySelection() },
		func() { e.gotoLine() }, // opens a prompt; ignore (no-op for cursor bounds)
	}
	e.search.caseSens = true
	e.search.text = "line"
	e.replaceTo = "LINE"

	for i := 0; i < 2000; i++ {
		ops[i%len(ops)]()
		checkCur(t, tb)
		// occasionally swap the buffer to a new state to vary content
		if i%97 == 0 {
			e.replaceAll()
			checkCur(t, tb)
		}
	}
}
