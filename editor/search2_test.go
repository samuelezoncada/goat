package editor

import "testing"

// TestReplaceUndoRestoresMatchedCase: a case-insensitive replace used to
// record the typed pattern, so undo rewrote the file with the wrong casing.
func TestReplaceUndoRestoresMatchedCase(t *testing.T) {
	tb := newTestTab("FOO and Foo and foo")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.text = "foo"
	e.search.caseSens = false
	e.replaceTo = "bar"
	e.replaceAll()
	if got := joinLines(t, tb); got != "bar and bar and bar" {
		t.Fatalf("replaceAll got %q", got)
	}
	tb.undo()
	if got := joinLines(t, tb); got != "FOO and Foo and foo" {
		t.Fatalf("undo must restore the original casing, got %q", got)
	}
}

// TestReplaceAllCoversWholeBuffer: replace-all used to start at the cursor and
// silently skip everything above it.
func TestReplaceAllCoversWholeBuffer(t *testing.T) {
	tb := newTestTab("x\nx\nx\nx")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	tb.cur = Pos{2, 0} // cursor in the middle
	e.search.text = "x"
	e.search.caseSens = true
	e.replaceTo = "y"
	e.replaceAll()
	if got := joinLines(t, tb); got != "y\ny\ny\ny" {
		t.Fatalf("got %q want every line replaced", got)
	}
}

func TestReplaceAllGrowingReplacementTerminates(t *testing.T) {
	tb := newTestTab("aaa")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.text = "a"
	e.search.caseSens = true
	e.replaceTo = "aa"
	e.replaceAll()
	if got := joinLines(t, tb); got != "aaaaaa" {
		t.Fatalf("got %q want aaaaaa", got)
	}
}

func TestSearchWholeWord(t *testing.T) {
	tb := newTestTab("foobar\nfoo bar")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.wholeWord = true
	e.search.caseSens = true
	tb.cur = Pos{0, 0}
	if !e.doSearch("foo", 1, true) {
		t.Fatal("whole-word search should find the standalone foo")
	}
	if tb.cur.Line != 1 || tb.cur.Col != 0 {
		t.Fatalf("landed at %v, want line 1 (foobar must not match)", tb.cur)
	}
}

func TestSearchRegex(t *testing.T) {
	tb := newTestTab("id=12\nid=345")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.regex = true
	e.search.caseSens = true
	tb.cur = Pos{0, 0}
	if !e.doSearch(`id=\d{3}`, 1, true) {
		t.Fatal("regex search failed")
	}
	if tb.cur.Line != 1 {
		t.Fatalf("landed at %v want line 1", tb.cur)
	}
}

func TestReplaceRegexWithCaptures(t *testing.T) {
	tb := newTestTab("name: bob\nname: alice")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.regex = true
	e.search.caseSens = true
	e.search.text = `name: (\w+)`
	e.replaceTo = "user=$1"
	e.replaceAll()
	if got := joinLines(t, tb); got != "user=bob\nuser=alice" {
		t.Fatalf("got %q", got)
	}
	tb.undo()
	if got := joinLines(t, tb); got != "name: bob\nname: alice" {
		t.Fatalf("undo got %q", got)
	}
}

func TestBadRegexReported(t *testing.T) {
	tb := newTestTab("abc")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.regex = true
	e.search.text = "a(" // unbalanced
	if e.doSearch("a(", 1, true) {
		t.Fatal("an invalid pattern must not report a match")
	}
	if e.msg == "" {
		t.Fatal("an invalid pattern should be reported to the user")
	}
}

func TestSearchHighlightsAllMatches(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.search.text = "ab"
	e.search.caseSens = true
	e.search.highlight = true
	ms := e.lineMatches([]rune("ab cab ab"))
	if len(ms) != 3 {
		t.Fatalf("found %d matches want 3: %v", len(ms), ms)
	}
	e.search.highlight = false
	if ms := e.lineMatches([]rune("ab")); ms != nil {
		t.Fatal("no highlighting when it is switched off")
	}
}

func TestReplaceEmptyMatchTerminates(t *testing.T) {
	tb := newTestTab("abc")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.regex = true
	e.search.caseSens = true
	e.search.text = "x*" // matches the empty string everywhere
	e.replaceTo = "-"
	e.replaceAll() // must not hang
	if tb.lineCount() != 1 {
		t.Fatalf("lines %d", tb.lineCount())
	}
}

func TestSearchCancelRestoresCursor(t *testing.T) {
	tb := newTestTab("alpha\nbeta\ngamma")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	tb.cur = Pos{0, 1}
	e.beginSearch()
	e.promptInsert([]rune("gamma")) // live search jumps to line 2
	if tb.cur.Line != 2 {
		t.Fatalf("live search should have moved to line 2, at %v", tb.cur)
	}
	p := e.prompt
	cancel := p.onCancel
	e.cancelPrompt()
	if cancel != nil {
		cancel()
	}
	if tb.cur != (Pos{0, 1}) {
		t.Fatalf("cancel should restore the cursor, at %v", tb.cur)
	}
}

func TestLiveSearchMeasuresFromStart(t *testing.T) {
	tb := newTestTab("aaa\nbbb\naab")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.caseSens = true
	tb.cur = Pos{0, 0}
	e.beginSearch()
	e.promptInsert([]rune("aab")) // matches on line 2
	if tb.cur.Line != 2 {
		t.Fatalf("at %v want line 2", tb.cur)
	}
	// Deleting a character must not walk forward from the previous hit.
	e.promptKey(keyEvent(tcell_KeyBackspace))
	if tb.cur.Line != 0 {
		t.Fatalf("after backspace at %v, want the first match again", tb.cur)
	}
}

func TestReplaceAllWithinSelection(t *testing.T) {
	tb := newTestTab("a a\na a\na a")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.caseSens = true
	// Select from line 1 col 0 to line 1 col 3 (the middle line only).
	tb.mark = &Pos{1, 0}
	tb.cur = Pos{1, 3}
	e.beginReplace()
	e.prompt.input = []rune("a")
	e.promptKey(keyEvent(tcellKeyEnter)) // "Replace Where Is"
	e.prompt.input = []rune("Z")
	e.promptKey(keyEvent(tcellKeyEnter)) // "Replace with"
	e.prompt.input = []rune("a")
	e.promptKey(keyEvent(tcellKeyEnter)) // answer: all

	if got := joinLines(t, tb); got != "a a\nZ Z\na a" {
		t.Fatalf("got %q, only the selected line should change", got)
	}
	tb.undo()
	if got := joinLines(t, tb); got != "a a\na a\na a" {
		t.Fatalf("undo got %q", got)
	}
}

func TestReplaceInSelectionPartialLine(t *testing.T) {
	tb := newTestTab("xx xx xx")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.caseSens = true
	tb.mark = &Pos{0, 0}
	tb.cur = Pos{0, 5} // covers the first two "xx"
	e.search.text = "xx"
	e.replaceTo = "y"
	sel := tb.selRange()
	e.replaceScope = &sel
	e.replaceAll()
	if got := joinLines(t, tb); got != "y y xx" {
		t.Fatalf("got %q want %q", got, "y y xx")
	}
}

func TestReplaceAllUnscopedStillCoversEverything(t *testing.T) {
	tb := newTestTab("a a\na a")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.caseSens = true
	e.search.text = "a"
	e.replaceTo = "bb"
	e.replaceAll()
	if got := joinLines(t, tb); got != "bb bb\nbb bb" {
		t.Fatalf("got %q", got)
	}
}

func TestUndoGroupsReplaceAll(t *testing.T) {
	tb := newTestTab("a a a\na a\na")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.search.text = "a"
	e.search.caseSens = true
	e.replaceTo = "bb"
	before := joinLines(t, tb)
	e.replaceAll()
	if got := joinLines(t, tb); got != "bb bb bb\nbb bb\nbb" {
		t.Fatalf("replaceAll got %q", got)
	}
	tb.undo()
	if got := joinLines(t, tb); got != before {
		t.Fatalf("one undo should revert the whole replace-all, got %q", got)
	}
	tb.redo()
	if got := joinLines(t, tb); got != "bb bb bb\nbb bb\nbb" {
		t.Fatalf("one redo should re-apply it, got %q", got)
	}
}

func TestRenderHighlightsSearchMatches(t *testing.T) {
	s := simScreen(t, 40, 8)
	tb := newTestTab("foo bar foo")
	e := editorOnScreen(t, s, tb)
	e.search.text = "foo"
	e.search.caseSens = true
	e.search.highlight = true
	e.draw()

	g := e.gutterWidth()
	y := e.mainTop()
	// Sample cells away from column 0, which carries the block cursor.
	_, _, matchStyle, _ := s.GetContent(g+1, y)  // inside the first "foo"
	_, _, plainStyle, _ := s.GetContent(g+4, y)  // inside "bar"
	_, _, match2Style, _ := s.GetContent(g+9, y) // inside the second "foo"
	if matchStyle == plainStyle {
		t.Fatal("search hits should be styled differently from ordinary text")
	}
	if match2Style != matchStyle {
		t.Fatal("every hit on the line should be highlighted, not just the first")
	}
	// Switching highlighting off restores the plain style.
	e.search.highlight = false
	e.frame = nil
	e.allocFrame()
	e.draw()
	_, _, afterStyle, _ := s.GetContent(g+1, y)
	if afterStyle == matchStyle {
		t.Fatal("clearing the highlight should restore the normal style")
	}
}

func TestPromptSearchOptionToggles(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.beginPrompt("Search: ", "", nil, func(string) {})
	e.promptKey(altRuneEvent('r'))
	if !e.search.regex {
		t.Fatal("Alt+R should toggle regex mode")
	}
	e.promptKey(altRuneEvent('u'))
	if !e.search.wholeWord {
		t.Fatal("Alt+U should toggle whole-word mode")
	}
	e.promptKey(altRuneEvent('c'))
	if !e.search.caseSens {
		t.Fatal("Alt+C should toggle case sensitivity")
	}
}
