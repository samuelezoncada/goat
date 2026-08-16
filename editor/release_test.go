package editor

import (
	"testing"
)

func TestNewTextStripsCR(t *testing.T) {
	text := newText([]byte("package x\r\nfunc Foo() {}\r\nbar\r"))
	if text.LineCount() != 3 {
		t.Fatalf("lines = %d want 3", text.LineCount())
	}
	if got := string(text.Line(0)); got != "package x" {
		t.Fatalf("line 0 = %q want %q", got, "package x")
	}
	if got := string(text.Line(1)); got != "func Foo() {}" {
		t.Fatalf("line 1 = %q", got)
	}
	if got := string(text.Line(2)); got != "bar" {
		t.Fatalf("line 2 = %q", got)
	}
	// pure LF input is untouched
	text2 := newText([]byte("a\nb\n"))
	if string(text2.Line(1)) != "b" {
		t.Fatalf("lf line = %q", text2.Line(1))
	}
}

func TestUndoCap(t *testing.T) {
	var s UndoStack
	for i := 0; i < maxUndo+100; i++ {
		s.push(&op{kind: opDelete, line: i, col: 0, text: []rune("x")})
	}
	if len(s.undoS) != maxUndo {
		t.Fatalf("undo size = %d want %d", len(s.undoS), maxUndo)
	}
	// the newest op is preserved (undo pops the most recent first)
	top := s.undoS[len(s.undoS)-1]
	if top.line != maxUndo+99 {
		t.Fatalf("top op line = %d want %d", top.line, maxUndo+99)
	}
	// redo still works after the cap
	if o := s.undo(); o == nil {
		t.Fatal("undo should still return ops")
	}
}
