package editor

import "testing"

func TestSearchForward(t *testing.T) {
	tb := newTestTab("foo bar baz\nqux foo")
	e := &Editor{tabs: []*Tab{tb}}
	tb.cur = Pos{0, 0}
	e.search.caseSens = true
	e.search.text = "foo"
	if !e.doSearch("foo", 1, true) {
		t.Fatal("not found")
	}
	if tb.cur.Line != 0 || tb.cur.Col != 0 {
		t.Fatalf("cursor %v", tb.cur)
	}
	if !e.doSearch("foo", 1, false) {
		t.Fatal("repeat not found")
	}
	if tb.cur.Line != 1 || tb.cur.Col != 4 {
		t.Fatalf("cursor %v", tb.cur)
	}
}

func TestSearchWraps(t *testing.T) {
	tb := newTestTab("aa\nbb\naa")
	e := &Editor{tabs: []*Tab{tb}}
	tb.cur = Pos{2, 1}
	e.search.caseSens = true
	if !e.doSearch("aa", 1, true) {
		t.Fatal("wrap search not found")
	}
	if tb.cur.Line != 0 || tb.cur.Col != 0 {
		t.Fatalf("cursor %v", tb.cur)
	}
}

func TestSearchWrapSameLineFromEnd(t *testing.T) {
	tb := newTestTab("foo foo")
	e := &Editor{tabs: []*Tab{tb}}
	e.search.caseSens = true
	e.search.text = "foo"
	tb.cur = Pos{0, 7} // cursor past the last char
	if !e.doSearch("foo", 1, true) {
		t.Fatal("search from end should wrap to line start")
	}
	if tb.cur.Line != 0 || tb.cur.Col != 0 {
		t.Fatalf("cursor %v", tb.cur)
	}
	// next should advance to the second occurrence on the same line
	e.searchNext()
	if tb.cur.Line != 0 || tb.cur.Col != 4 {
		t.Fatalf("next cursor %v", tb.cur)
	}
	// and next again wraps around to the first
	e.searchNext()
	if tb.cur.Line != 0 || tb.cur.Col != 0 {
		t.Fatalf("wrap-next cursor %v", tb.cur)
	}
}

func TestSearchBackward(t *testing.T) {
	tb := newTestTab("aa bb\ncc aa")
	e := &Editor{tabs: []*Tab{tb}}
	tb.cur = Pos{0, 5}
	e.search.caseSens = true
	if !e.doSearch("aa", -1, true) {
		t.Fatal("backward not found")
	}
	if tb.cur.Line != 0 || tb.cur.Col != 0 {
		t.Fatalf("cursor %v", tb.cur)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	tb := newTestTab("Foo")
	e := &Editor{tabs: []*Tab{tb}}
	e.search.caseSens = false
	if !e.doSearch("foo", 1, true) {
		t.Fatal("case-insensitive match failed")
	}
	if tb.cur.Col != 0 {
		t.Fatalf("cursor %v", tb.cur)
	}
}

func TestSearchMissing(t *testing.T) {
	tb := newTestTab("hello")
	e := &Editor{tabs: []*Tab{tb}}
	if e.doSearch("zzz", 1, true) {
		t.Fatal("should not match")
	}
}

func TestReplaceAll(t *testing.T) {
	tb := newTestTab("foo foo foo")
	e := &Editor{tabs: []*Tab{tb}}
	e.search.text = "foo"
	e.replaceTo = "bar"
	tb.cur = Pos{0, 0}
	e.replaceAll()
	if got := joinLines(t, tb); got != "bar bar bar" {
		t.Fatalf("got %q", got)
	}
	// undo should restore
	for tb.edits.CanUndo() {
		tb.undo()
	}
	if got := joinLines(t, tb); got != "foo foo foo" {
		t.Fatalf("after undo got %q", got)
	}
}

func TestJustify(t *testing.T) {
	tb := newTestTab("hello\nworld\n\nrest")
	e := &Editor{tabs: []*Tab{tb}}
	tb.cur = Pos{0, 0}
	e.justify()
	want := "helloworld\n\nrest"
	if got := joinLines(t, tb); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// undo (twice: the tail-append and the line removal) restores the paragraph
	for tb.edits.CanUndo() {
		tb.undo()
	}
	if got := joinLines(t, tb); got != "hello\nworld\n\nrest" {
		t.Fatalf("after undo got %q", got)
	}
}
