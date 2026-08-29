package editor

import "testing"

// TestTypingReplacesSelection covers the expectation every editor sets: with
// text selected, typing replaces it instead of writing inside it.
func TestTypingReplacesSelection(t *testing.T) {
	tb := newTestTab("hello world")
	tb.mark = &Pos{0, 0}
	tb.cur = Pos{0, 5} // "hello" selected
	tb.insertRune('X')
	if got := joinLines(t, tb); got != "X world" {
		t.Fatalf("got %q want %q", got, "X world")
	}
	if tb.mark != nil {
		t.Fatal("typing should clear the selection")
	}
	if tb.cur != (Pos{0, 1}) {
		t.Fatalf("cursor %v want 0,1", tb.cur)
	}
	tb.undo() // one action
	if got := joinLines(t, tb); got != "hello world" {
		t.Fatalf("undo got %q", got)
	}
}

func TestBackspaceDeletesSelection(t *testing.T) {
	tb := newTestTab("hello world")
	tb.mark = &Pos{0, 5}
	tb.cur = Pos{0, 11}
	tb.backspace()
	if got := joinLines(t, tb); got != "hello" {
		t.Fatalf("got %q", got)
	}
	if tb.mark != nil {
		t.Fatal("selection should be cleared")
	}
}

func TestDeleteForwardDeletesSelection(t *testing.T) {
	tb := newTestTab("abc\ndef")
	tb.mark = &Pos{0, 1}
	tb.cur = Pos{1, 2}
	tb.deleteForward()
	if got := joinLines(t, tb); got != "af" {
		t.Fatalf("got %q want %q", got, "af")
	}
}

func TestNewlineReplacesSelection(t *testing.T) {
	tb := newTestTab("abcdef")
	tb.cfg = DefaultConfig()
	tb.mark = &Pos{0, 2}
	tb.cur = Pos{0, 4}
	tb.insertNewline()
	if got := joinLines(t, tb); got != "ab\nef" {
		t.Fatalf("got %q want %q", got, "ab\nef")
	}
	tb.undo()
	if got := joinLines(t, tb); got != "abcdef" {
		t.Fatalf("undo got %q", got)
	}
}

// TestAutoIndentNeverInventsWhitespace: splitting inside the leading
// whitespace copies at most the indent that precedes the cursor, so the total
// indentation is preserved rather than doubled.
func TestAutoIndentNeverInventsWhitespace(t *testing.T) {
	tb := newTestTab("    indented")
	tb.cfg = DefaultConfig()
	tb.cur = Pos{0, 2} // split inside the leading whitespace
	tb.insertNewline()
	// 2 spaces stay on the first line; the other 2 move down with the text,
	// and the copied indent is capped at 2 instead of adding 4 more.
	if got := joinLines(t, tb); got != "  \n    indented" {
		t.Fatalf("got %q", got)
	}
	tb2 := newTestTab("\t\tfoo")
	tb2.cfg = DefaultConfig()
	tb2.cur = Pos{0, 1}
	tb2.insertNewline()
	if got := joinLines(t, tb2); got != "\t\n\t\tfoo" {
		t.Fatalf("tabs: got %q", got)
	}
}

func TestAutoIndentOff(t *testing.T) {
	tb := newTestTab("    x")
	tb.cfg = &Config{TabWidth: 8, AutoIndent: false}
	tb.cur = Pos{0, 5}
	tb.insertNewline()
	if got := joinLines(t, tb); got != "    x\n" {
		t.Fatalf("got %q want no copied indent", got)
	}
}

// --- grapheme clusters ---------------------------------------------------

func TestClusterMovementCombining(t *testing.T) {
	// "e" + combining acute, then "x"
	tb := newTestTab("éx")
	tb.cur = Pos{0, 0}
	tb.moveRight()
	if tb.cur.Col != 2 {
		t.Fatalf("moveRight over a combining pair landed at %d want 2", tb.cur.Col)
	}
	tb.moveLeft()
	if tb.cur.Col != 0 {
		t.Fatalf("moveLeft landed at %d want 0", tb.cur.Col)
	}
}

func TestBackspaceDeletesWholeCluster(t *testing.T) {
	tb := newTestTab("aé")
	tb.cur = Pos{0, 3}
	tb.backspace()
	if got := joinLines(t, tb); got != "a" {
		t.Fatalf("backspace left %q, want the whole cluster removed", got)
	}
}

func TestDeleteForwardDeletesWholeCluster(t *testing.T) {
	tb := newTestTab("éa")
	tb.cur = Pos{0, 0}
	tb.deleteForward()
	if got := joinLines(t, tb); got != "a" {
		t.Fatalf("got %q", got)
	}
}

func TestCombiningMarkWidthIsZero(t *testing.T) {
	if w := runeWidth('́'); w != 0 {
		t.Fatalf("combining acute width = %d want 0", w)
	}
	if w := runeWidth('世'); w != 2 {
		t.Fatalf("wide rune width = %d want 2", w)
	}
	if w := runeWidth('a'); w != 1 {
		t.Fatalf("ascii width = %d want 1", w)
	}
	if w := runeWidth('\t'); w != 1 {
		t.Fatalf("tab must report a nonzero width (callers expand it): %d", w)
	}
}

func TestDisplayColWithCombiningAndTabs(t *testing.T) {
	// "e" + acute occupies one cell, so the following char is at cell 1.
	line := []rune("éx")
	if c := displayCol(line, 2, 8); c != 1 {
		t.Fatalf("displayCol after a cluster = %d want 1", c)
	}
	if c := displayCol([]rune("a\tb"), 2, 4); c != 4 {
		t.Fatalf("tab width 4: got %d want 4", c)
	}
	if c := displayCol([]rune("世界"), 1, 8); c != 2 {
		t.Fatalf("wide rune: got %d want 2", c)
	}
}

func TestColFromDispRoundTrip(t *testing.T) {
	line := []rune("a\t世éb")
	for col := 0; col <= len(line); col++ {
		d := displayCol(line, col, 8)
		back := colFromDisp(line, d, 8)
		if back > col {
			t.Fatalf("col %d -> disp %d -> col %d (moved right)", col, d, back)
		}
	}
}

func TestFlagIsOneCluster(t *testing.T) {
	line := []rune("🇮🇹x")
	end, w := clusterEnd(line, 0)
	if end != 2 || w != 2 {
		t.Fatalf("flag cluster: end=%d w=%d want 2,2", end, w)
	}
}

func TestZWJSequenceIsOneCluster(t *testing.T) {
	line := []rune("👨‍💻x")
	end, _ := clusterEnd(line, 0)
	if end != 3 {
		t.Fatalf("ZWJ emoji cluster end=%d want 3", end)
	}
}

// --- buffer invariants ---------------------------------------------------

func TestBufferKeepsOneLine(t *testing.T) {
	txt := newText([]byte("only"))
	txt.RemoveLine(0)
	if txt.LineCount() != 1 {
		t.Fatalf("line count = %d, the buffer must keep one line", txt.LineCount())
	}
	if len(txt.Line(0)) != 0 {
		t.Fatalf("remaining line should be empty, got %q", string(txt.Line(0)))
	}
	// The invariant means insertText can still address line 0.
	insertText(txt, 0, 0, []rune("x"))
	if string(txt.Line(0)) != "x" {
		t.Fatalf("line 0 = %q", string(txt.Line(0)))
	}
}

func TestClampCursorAfterShrink(t *testing.T) {
	tb := newTestTab("hello\nworld")
	tb.cur = Pos{1, 5}
	tb.mark = &Pos{1, 4}
	// Delete the second line out from under the cursor.
	tb.text.RemoveLine(1)
	tb.clampCursor()
	if tb.cur.Line != 0 || tb.cur.Col > len(tb.line(0)) {
		t.Fatalf("cursor %v out of range", tb.cur)
	}
	if tb.mark.Line != 0 {
		t.Fatalf("mark %v out of range", *tb.mark)
	}
}

func TestCutStaleMarkDoesNotPanic(t *testing.T) {
	tb := newTestTab("short")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	tb.cur = Pos{0, 2}
	tb.mark = &Pos{9, 99} // far out of range
	e.cut()
	if got := joinLines(t, tb); got != "" && got != "sh" && got != "short" {
		t.Fatalf("unexpected buffer %q", got)
	}
}

// TestHighlightFlushIsRateLimited: the snapshot handed to the highlighter is
// what made typing expensive, so it must not happen once per keystroke.
func TestHighlightFlushIsRateLimited(t *testing.T) {
	tb := newTestTab("package main\n\nfunc main() {}\n")
	tb.cfg = DefaultConfig()
	tb.hl = newTestHighlighter()
	defer tb.close()

	now := timeZero()
	tb.invalidate(0)
	if pending := tb.flushHighlight(now); pending {
		t.Fatal("the first snapshot should go through immediately")
	}
	// A burst of edits within the interval is coalesced.
	for i := 0; i < 10; i++ {
		tb.invalidate(0)
		if !tb.flushHighlight(now) {
			t.Fatalf("edit %d triggered a second snapshot inside the interval", i)
		}
	}
	// Once the interval passes, the pending snapshot goes out.
	if pending := tb.flushHighlight(now.Add(hlInterval)); pending {
		t.Fatal("a pending snapshot should flush after the interval")
	}
	if tb.hlPending {
		t.Fatal("nothing should be pending after a flush")
	}
	// With nothing pending, there is no work and nothing to schedule.
	if tb.flushHighlight(now.Add(2 * hlInterval)) {
		t.Fatal("no snapshot is due when no edit happened")
	}
}

// TestInvalidateTracksLowestLine keeps the re-lex starting point honest.
func TestInvalidateTracksLowestLine(t *testing.T) {
	tb := newTestTab("a\nb\nc\nd")
	tb.hl = newTestHighlighter()
	defer tb.close()
	tb.invalidate(3)
	tb.invalidate(1)
	tb.invalidate(2)
	if tb.hlFrom != 1 {
		t.Fatalf("hlFrom = %d, want the lowest invalidated line", tb.hlFrom)
	}
}

func TestTabWidthAffectsDisplay(t *testing.T) {
	tb := newTestTab("\tx")
	tb.cfg = &Config{TabWidth: 4}
	if got := displayCol(tb.line(0), 1, tb.tabW()); got != 4 {
		t.Fatalf("tab width 4 -> %d", got)
	}
	tb.cfg = &Config{TabWidth: 2}
	if got := displayCol(tb.line(0), 1, tb.tabW()); got != 2 {
		t.Fatalf("tab width 2 -> %d", got)
	}
}
