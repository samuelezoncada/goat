package editor

import (
	"strings"
	"testing"
)

// TestUndoOwnsItsText guards against a recorded op aliasing a line's backing
// array: a later edit that appends in place would otherwise rewrite history.
func TestUndoOwnsItsText(t *testing.T) {
	tb := newTestTab("hello")
	// Give the line spare capacity, so an in-place append is possible.
	line := make([]rune, 5, 64)
	copy(line, []rune("hello"))
	tb.text.Set(0, line)

	tb.cur = Pos{0, 5}
	tb.backspace() // records a delete of "o"
	recorded := string(tb.edits.undoS[len(tb.edits.undoS)-1].text)
	// Type over the same region a few times, which reuses the array.
	for _, r := range "XYZW" {
		tb.insertRune(r)
	}
	if got := string(tb.edits.undoS[0].text); got != recorded {
		t.Fatalf("recorded undo text changed from %q to %q", recorded, got)
	}
	// Undoing everything must restore the original content exactly.
	for tb.edits.CanUndo() {
		tb.undo()
	}
	if got := joinLines(t, tb); got != "hello" {
		t.Fatalf("after undo got %q want %q", got, "hello")
	}
}

func TestUndoGroupsJustify(t *testing.T) {
	tb := newTestTab("alpha\nbeta\ngamma\n\ntail")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig(), width: 80, height: 24}
	tb.cur = Pos{0, 0}
	before := joinLines(t, tb)
	e.justify()
	if joinLines(t, tb) == before {
		t.Fatal("justify did nothing")
	}
	tb.undo() // one step, not one per line
	if got := joinLines(t, tb); got != before {
		t.Fatalf("one undo should restore the paragraph, got %q", got)
	}
	tb.redo()
	if joinLines(t, tb) == before {
		t.Fatal("one redo should re-apply the whole justify")
	}
}

func TestUndoCoalescesBackspaceRun(t *testing.T) {
	tb := newTestTab("hello world")
	tb.cur = Pos{0, 11}
	for i := 0; i < 5; i++ {
		tb.backspace()
	}
	if got := joinLines(t, tb); got != "hello " {
		t.Fatalf("after backspaces %q", got)
	}
	if n := len(tb.edits.undoS); n != 1 {
		t.Fatalf("backspace run recorded %d ops, want 1", n)
	}
	tb.undo()
	if got := joinLines(t, tb); got != "hello world" {
		t.Fatalf("one undo should restore the run, got %q", got)
	}
}

func TestUndoCoalescesDeleteForwardRun(t *testing.T) {
	tb := newTestTab("hello world")
	tb.cur = Pos{0, 0}
	for i := 0; i < 6; i++ {
		tb.deleteForward()
	}
	if got := joinLines(t, tb); got != "world" {
		t.Fatalf("after deletes %q", got)
	}
	if n := len(tb.edits.undoS); n != 1 {
		t.Fatalf("delete run recorded %d ops, want 1", n)
	}
	tb.undo()
	if got := joinLines(t, tb); got != "hello world" {
		t.Fatalf("one undo got %q", got)
	}
}

func TestSaveSealsCoalescing(t *testing.T) {
	dir := t.TempDir()
	tb := newTestTab("")
	tb.insertRune('a')
	if err := tb.saveTo(dir + "/f.txt"); err != nil {
		t.Fatal(err)
	}
	tb.insertRune('b')
	tb.undo()
	if got := joinLines(t, tb); got != "a" {
		t.Fatalf("undo after save should stop at the saved content, got %q", got)
	}
	if tb.dirty {
		t.Fatal("back at saved content, so not dirty")
	}
}

func TestUndoCapDropsWholeGroups(t *testing.T) {
	var s UndoStack
	// One group, then enough single ops to push past the cap.
	s.begin()
	for i := 0; i < 5; i++ {
		s.push(&op{kind: opDelete, line: i, text: []rune("x")})
	}
	s.end()
	for i := 0; i < maxUndo+10; i++ {
		s.push(&op{kind: opDelete, line: 1000 + i, text: []rune("y")})
	}
	if len(s.undoS) > maxUndo {
		t.Fatalf("stack grew to %d, cap is %d", len(s.undoS), maxUndo)
	}
	// No orphaned fragment of the dropped group may remain.
	for _, o := range s.undoS {
		if o.group != 0 && o.line < 1000 {
			t.Fatal("a group was split by trimming")
		}
	}
}

func TestUndoStackTrimKeepsNewest(t *testing.T) {
	var s UndoStack
	for i := 0; i < maxUndo+100; i++ {
		s.push(&op{kind: opDelete, line: i, col: 0, text: []rune("x")})
	}
	if len(s.undoS) != maxUndo {
		t.Fatalf("size %d want %d", len(s.undoS), maxUndo)
	}
	if top := s.undoS[len(s.undoS)-1]; top.line != maxUndo+99 {
		t.Fatalf("newest op lost: line %d", top.line)
	}
	if oldest := s.undoS[0]; oldest.line != 100 {
		t.Fatalf("oldest kept op = %d want 100", oldest.line)
	}
}

func TestIndentDedentSelectionIsOneAction(t *testing.T) {
	tb := newTestTab("one\ntwo\nthree")
	tb.cfg = DefaultConfig()
	tb.mark = &Pos{0, 0}
	tb.cur = Pos{2, 5}
	tb.insertTab()
	want := "\tone\n\ttwo\n\tthree"
	if got := joinLines(t, tb); got != want {
		t.Fatalf("indent got %q want %q", got, want)
	}
	tb.undo()
	if got := joinLines(t, tb); got != "one\ntwo\nthree" {
		t.Fatalf("one undo should revert the block indent, got %q", got)
	}
	// Dedent removes it again.
	tb.redo()
	tb.mark = &Pos{0, 0}
	tb.cur = Pos{2, 5}
	tb.dedent()
	if got := joinLines(t, tb); got != "one\ntwo\nthree" {
		t.Fatalf("dedent got %q", got)
	}
}

func TestIndentWithSpacesConfig(t *testing.T) {
	tb := newTestTab("x")
	tb.cfg = &Config{TabWidth: 4, ExpandTab: true, AutoIndent: true}
	tb.cur = Pos{0, 0}
	tb.insertTab()
	if got := joinLines(t, tb); got != "    x" {
		t.Fatalf("expandtab got %q want 4 spaces", got)
	}
	// From column 4 the next stop is 4 more spaces.
	tb.cur = Pos{0, 4}
	tb.insertTab()
	if got := joinLines(t, tb); !strings.HasPrefix(got, "        x") {
		t.Fatalf("second tab got %q", got)
	}
}

func TestDedentSpaces(t *testing.T) {
	tb := newTestTab("      deep")
	tb.cfg = &Config{TabWidth: 4, ExpandTab: true}
	tb.cur = Pos{0, 0}
	tb.dedent()
	if got := joinLines(t, tb); got != "  deep" {
		t.Fatalf("dedent got %q want %q", got, "  deep")
	}
}
