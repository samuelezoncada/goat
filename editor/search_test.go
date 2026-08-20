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

// TestReplaceAllContainingNeedle guards against the infinite loop where a
// replacement that contains the search text re-finds itself after a wrap.
func TestReplaceAllContainingNeedle(t *testing.T) {
	tb := newTestTab("cat")
	e := &Editor{tabs: []*Tab{tb}}
	e.search.text = "a"
	e.replaceTo = "aa"
	tb.cur = Pos{0, 0}
	e.replaceAll()
	if got := joinLines(t, tb); got != "caat" {
		t.Fatalf("got %q", got)
	}
	tb2 := newTestTab("aa")
	e2 := &Editor{tabs: []*Tab{tb2}}
	e2.search.text = "aa"
	e2.replaceTo = "a"
	tb2.cur = Pos{0, 0}
	e2.replaceAll()
	if got := joinLines(t, tb2); got != "a" {
		t.Fatalf("got %q", got)
	}
}

// TestReplaceAllDoesNotWrap ensures replace-all processes each occurrence once
// and does not loop back to matches already replaced before the cursor.
func TestReplaceAllDoesNotWrap(t *testing.T) {
	tb := newTestTab("foo foo foo")
	e := &Editor{tabs: []*Tab{tb}}
	e.search.text = "foo"
	e.replaceTo = "x"
	tb.cur = Pos{0, 0}
	e.replaceAll()
	if got := joinLines(t, tb); got != "x x x" {
		t.Fatalf("got %q", got)
	}
}

// TestSearchNonASCII guards against byte-vs-rune column confusion: the search
// cursor position must be a rune column even on lines with multi-byte runes.
func TestSearchNonASCII(t *testing.T) {
	tb := newTestTab("café bar baz")
	e := &Editor{tabs: []*Tab{tb}}
	e.search.caseSens = true
	e.search.text = "bar"
	tb.cur = Pos{0, 0}
	if !e.doSearch("bar", 1, true) {
		t.Fatal("not found")
	}
	if tb.cur.Col != 5 {
		t.Fatalf("cursor col=%d want 5 (rune col of 'bar' after 'café ')", tb.cur.Col)
	}
	// reverse search across the same line: from the end finds the second one
	tb2 := newTestTab("héllo wörld héllo")
	e2 := &Editor{tabs: []*Tab{tb2}}
	e2.search.caseSens = true
	e2.search.text = "héllo"
	tb2.cur = Pos{0, 17}
	if !e2.doSearch("héllo", -1, true) {
		t.Fatal("backward not found")
	}
	if tb2.cur.Col != 12 {
		t.Fatalf("backward cursor col=%d want 12", tb2.cur.Col)
	}
}

func TestSearchCaseInsensitiveNonASCII(t *testing.T) {
	tb := newTestTab("CAFÉ Bar")
	e := &Editor{tabs: []*Tab{tb}}
	e.search.caseSens = false
	e.search.text = "café"
	tb.cur = Pos{0, 0}
	if !e.doSearch("café", 1, true) {
		t.Fatal("not found")
	}
	if tb.cur.Col != 0 {
		t.Fatalf("col=%d want 0", tb.cur.Col)
	}
	e.search.text = "bar"
	if !e.doSearch("bar", 1, false) {
		t.Fatal("bar not found")
	}
	if tb.cur.Col != 5 {
		t.Fatalf("bar col=%d want 5", tb.cur.Col)
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
