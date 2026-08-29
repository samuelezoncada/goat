package editor

import (
	"path/filepath"
	"testing"
)

func newTestTab(content string) *Tab {
	t := &Tab{text: newText([]byte(content))}
	if len(content) > 0 && content[len(content)-1] == '\n' {
		t.trailingNL = true
	}
	return t
}

func joinLines(t *testing.T, tb *Tab) string {
	t.Helper()
	out := ""
	for i := 0; i < tb.lineCount(); i++ {
		if i > 0 {
			out += "\n"
		}
		out += string(tb.line(i))
	}
	return out
}

func TestInsertRunesSingleLineCursor(t *testing.T) {
	tb := newTestTab("hello")
	tb.insertRunes(0, 2, []rune("XY"))
	if tb.cur.Line != 0 || tb.cur.Col != 4 {
		t.Fatalf("cursor = %+v want 0,4", tb.cur)
	}
}

func TestInsertRunesMultilineCursor(t *testing.T) {
	tb := newTestTab("ab")
	tb.insertRunes(0, 1, []rune("X\nYZ"))
	if tb.cur.Line != 1 || tb.cur.Col != 2 {
		t.Fatalf("cursor = %+v want 1,2", tb.cur)
	}
	if got := joinLines(t, tb); got != "aX\nYZb" {
		t.Fatalf("got %q", got)
	}
	// typing after the paste must not panic (cursor must be in range)
	tb.insertRune('q')
	if got := joinLines(t, tb); got != "aX\nYZqb" {
		t.Fatalf("after typing got %q", got)
	}
}

func TestInsertRunesMultilineCursorTrailingNewline(t *testing.T) {
	tb := newTestTab("")
	tb.insertRunes(0, 0, []rune("a\nb\n"))
	if tb.cur.Line != 2 || tb.cur.Col != 0 {
		t.Fatalf("cursor = %+v want 2,0", tb.cur)
	}
}

func TestInsertRunesEmptyIsNoOp(t *testing.T) {
	tb := newTestTab("hello")
	tb.insertRunes(0, 0, nil)
	if tb.dirty {
		t.Fatal("empty insert should not mark the tab dirty")
	}
	if tb.edits.CanUndo() {
		t.Fatal("empty insert should not record an undo op")
	}
}

func TestInsertTextSingleLine(t *testing.T) {
	tb := newTestTab("hello")
	insertText(tb.text, 0, 2, []rune("XY"))
	if got := joinLines(t, tb); got != "heXYllo" {
		t.Fatalf("got %q", got)
	}
}

func TestInsertTextMultiline(t *testing.T) {
	tb := newTestTab("ab")
	insertText(tb.text, 0, 1, []rune("X\nYZ"))
	want := "aX\nYZb"
	if got := joinLines(t, tb); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestInsertTextTrailingNewline(t *testing.T) {
	tb := newTestTab("abc")
	insertText(tb.text, 0, 3, []rune("\n"))
	want := "abc\n"
	if got := joinLines(t, tb); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDeleteTextWithinLine(t *testing.T) {
	tb := newTestTab("hello")
	rem := deleteText(tb.text, 0, 1, 3)
	if string(rem) != "ell" {
		t.Fatalf("removed %q", rem)
	}
	if got := joinLines(t, tb); got != "ho" {
		t.Fatalf("got %q", got)
	}
}

func TestDeleteTextAcrossLines(t *testing.T) {
	tb := newTestTab("ab\ncd\nef")
	// delete "b\nc" => "a" + "d\nef"
	rem := deleteText(tb.text, 0, 1, 3)
	if string(rem) != "b\nc" {
		t.Fatalf("removed %q", rem)
	}
	if got := joinLines(t, tb); got != "ad\nef" {
		t.Fatalf("got %q", got)
	}
}

func TestTypingAndBackspace(t *testing.T) {
	tb := newTestTab("")
	tb.insertRune('a')
	tb.insertRune('b')
	if got := joinLines(t, tb); got != "ab" {
		t.Fatalf("got %q", got)
	}
	tb.backspace()
	if got := joinLines(t, tb); got != "a" {
		t.Fatalf("got %q", got)
	}
}

func TestNewlineSplitsLine(t *testing.T) {
	tb := newTestTab("  hello world")
	tb.cur = Pos{0, 7}
	tb.insertNewline()
	// indent of the split line is copied, then the remainder (" world") follows
	want := "  hello\n   world"
	if got := joinLines(t, tb); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if tb.cur.Line != 1 || tb.cur.Col != 2 {
		t.Fatalf("cursor %v", tb.cur)
	}
}

func TestNewlineAutoIndent(t *testing.T) {
	tb := newTestTab("    foo")
	tb.cur = Pos{0, 7}
	tb.insertNewline()
	if got := string(tb.line(1)); got != "    " {
		t.Fatalf("indented line %q", got)
	}
}

func TestUndoRedoTyping(t *testing.T) {
	tb := newTestTab("")
	for _, r := range "abc" {
		tb.insertRune(r)
	}
	if got := joinLines(t, tb); got != "abc" {
		t.Fatalf("got %q", got)
	}
	tb.undo()
	if got := joinLines(t, tb); got != "" {
		t.Fatalf("after undo got %q", got)
	}
	tb.redo()
	if got := joinLines(t, tb); got != "abc" {
		t.Fatalf("after redo got %q", got)
	}
}

func TestUndoNewline(t *testing.T) {
	tb := newTestTab("a")
	tb.cur = Pos{0, 1}
	tb.insertNewline()
	tb.insertRune('b')
	tb.undo() // remove 'b'
	tb.undo() // join back to "a"
	if got := joinLines(t, tb); got != "a" {
		t.Fatalf("got %q", got)
	}
}

func TestUndoNewlineAutoIndent(t *testing.T) {
	tb := newTestTab("    foo")
	tb.cur = Pos{0, 7}
	tb.insertNewline()
	if got := string(tb.line(1)); got != "    " {
		t.Fatalf("indented line %q", got)
	}
	tb.undo()
	want, got := "    foo", joinLines(t, tb)
	if got != want {
		t.Fatalf("after undo got %q want %q (stray indent left)", got, want)
	}
	if tb.cur.Line != 0 || tb.cur.Col != 7 {
		t.Fatalf("cursor %v", tb.cur)
	}
}

func TestUndoRedoDirty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	tb := newTestTab("")
	tb.insertRune('a')
	tb.insertRune('b')
	if err := tb.saveTo(path); err != nil {
		t.Fatal(err)
	}
	if tb.dirty {
		t.Fatal("saved tab should not be dirty")
	}
	tb.undo()
	if !tb.dirty {
		t.Fatal("undo away from saved content should mark dirty")
	}
	tb.redo()
	if tb.dirty {
		t.Fatal("redo back to saved content should clear dirty")
	}
	// edit past the saved content, then undo back to it
	tb.insertRune('c')
	tb.undo()
	if tb.dirty {
		t.Fatal("undo back to saved content should clear dirty")
	}
	// a fresh edit after that marks it dirty again
	tb.insertRune('d')
	if !tb.dirty {
		t.Fatal("editing should mark dirty")
	}
}

func TestCutLine(t *testing.T) {
	tb := newTestTab("one\ntwo\nthree")
	e := &Editor{clip: nil}
	tb.cur = Pos{1, 0}
	tb.mark = nil
	// emulate cut: reuse logic by extracting region via deleteRegion on the whole line
	removed := tb.deleteRegion(1, 0, 2, 0)
	if string(removed) != "two\n" {
		t.Fatalf("removed %q", removed)
	}
	_ = e
	want := "one\nthree"
	if got := joinLines(t, tb); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDisplayCol(t *testing.T) {
	if c := displayCol([]rune("a\tb"), 2, 8); c != 8 {
		t.Fatalf("tab width got %d want 8", c)
	}
	if c := displayCol([]rune("ab"), 2, 8); c != 2 {
		t.Fatalf("got %d", c)
	}
}
