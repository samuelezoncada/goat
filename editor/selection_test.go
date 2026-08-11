package editor

import "testing"

func TestSelectionCopyCut(t *testing.T) {
	tb := newTestTab("hello world")
	e := &Editor{tabs: []*Tab{tb}}
	tb.cur = Pos{0, 0}
	tb.selectMove(tb.moveRight) // mark (0,0), cur (0,1)
	tb.selectMove(tb.moveRight) // cur (0,2)
	e.copySelection()
	if string(e.clip) != "he" {
		t.Fatalf("clip %q", e.clip)
	}
	e.cut()
	if got := joinLines(t, tb); got != "llo world" {
		t.Fatalf("after cut got %q", got)
	}
}

func TestCopyMultiline(t *testing.T) {
	tb := newTestTab("ab\ncd\nef")
	e := &Editor{tabs: []*Tab{tb}}
	tb.cur = Pos{0, 1}
	tb.mark = &Pos{2, 1}
	e.copySelection()
	if string(e.clip) != "b\ncd\ne" {
		t.Fatalf("clip %q", e.clip)
	}
	// pasting it back into the untouched buffer reinserts the copied text;
	// the split line's remainder ("b") follows the last inserted line
	tb.cur = Pos{0, 1}
	e.uncut()
	if got := joinLines(t, tb); got != "ab\ncd\neb\ncd\nef" {
		t.Fatalf("after paste got %q", got)
	}
}

func TestSelectMoveClearsWithoutShift(t *testing.T) {
	tb := newTestTab("hello world")
	e := &Editor{tabs: []*Tab{tb}}
	tb.cur = Pos{0, 0}
	tb.selectMove(tb.moveRight) // mark set
	if tb.mark == nil {
		t.Fatal("mark should be set")
	}
	// plain movement clears the selection
	e.selectMove(tb.moveRight, false)
	if tb.mark != nil {
		t.Fatal("mark should be cleared on plain movement")
	}
}

func TestToggleMark(t *testing.T) {
	tb := newTestTab("hello")
	tb.cur = Pos{0, 2}
	tb.toggleMark()
	if tb.mark == nil || tb.mark.Col != 2 {
		t.Fatalf("mark %v", tb.mark)
	}
	tb.toggleMark()
	if tb.mark != nil {
		t.Fatal("mark should be cleared")
	}
}

func TestWordMoveExtendsSelection(t *testing.T) {
	tb := newTestTab("foo bar baz")
	tb.cur = Pos{0, 7} // on the space before "baz"
	tb.selectMove(tb.wordLeft)
	if tb.mark == nil || tb.mark.Col != 7 {
		t.Fatalf("mark %v", tb.mark)
	}
	if tb.cur.Col != 4 {
		t.Fatalf("cursor %v, want start of 'bar'", tb.cur.Col)
	}
	tb.selectMove(tb.wordLeft)
	if tb.cur.Col != 0 {
		t.Fatalf("cursor %v, want 0", tb.cur.Col)
	}
}

func TestManualScrollNotOverridden(t *testing.T) {
	tb := newTestTab("one\ntwo\nthree\nfour\nfive\nsix")
	tb.cur = Pos{0, 0}
	tb.lastScroll = tb.cur
	e := &Editor{tabs: []*Tab{tb}}

	// mouse wheel scrolled down: top = 2
	tb.top = 2
	e.ensureVisible(50, 3)
	if tb.top != 2 {
		t.Fatalf("manual scroll overridden: top=%d", tb.top)
	}

	// cursor moved onto a visible line: no forced jump
	tb.cur = Pos{4, 0}
	e.ensureVisible(50, 3)
	if tb.top != 2 {
		t.Fatalf("cursor on visible line moved view: top=%d", tb.top)
	}
	// cursor moved off-screen below: follow
	tb.cur = Pos{5, 0}
	e.ensureVisible(50, 3)
	if tb.top != 3 {
		t.Fatalf("cursor follow failed: top=%d want 3", tb.top)
	}
}
