package editor

import "testing"

func TestWrapStartsBreaksAtSpaces(t *testing.T) {
	line := []rune("the quick brown fox jumps")
	starts := wrapStarts(line, 10, 8)
	if len(starts) < 3 {
		t.Fatalf("starts %v", starts)
	}
	// Every row must fit and must not start with the leftover of a split word.
	for i, s := range starts {
		end := len(line)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		if w := displayCol(line[s:end], end-s, 8); w > 10 {
			t.Fatalf("row %d width %d > 10: %q", i, w, string(line[s:end]))
		}
	}
	if string(line[starts[0]:starts[1]]) != "the quick " {
		t.Fatalf("first row %q", string(line[starts[0]:starts[1]]))
	}
}

func TestWrapStartsHardBreaksLongWord(t *testing.T) {
	line := []rune("aaaaaaaaaaaaaaaaaaaa")
	starts := wrapStarts(line, 5, 8)
	if len(starts) != 4 {
		t.Fatalf("starts %v want 4 rows", starts)
	}
}

func TestWrapStartsEmptyLine(t *testing.T) {
	if got := wrapStarts(nil, 10, 8); len(got) != 1 || got[0] != 0 {
		t.Fatalf("empty line rows %v", got)
	}
}

func TestVisibleRowsWrapped(t *testing.T) {
	tb := newTestTab("aaaa bbbb cccc\nshort")
	tb.cfg = &Config{TabWidth: 8, Wrap: true}
	e := &Editor{tabs: []*Tab{tb}, cfg: tb.cfg, width: 20, height: 10}
	rows := e.visibleRows(tb, 5, 8)
	if len(rows) < 4 {
		t.Fatalf("rows %v", rows)
	}
	if !rows[0].first || rows[1].first {
		t.Fatal("only the first row of a line carries the gutter number")
	}
	// The last row must be the second buffer line.
	last := rows[len(rows)-1]
	if last.line != 1 {
		t.Fatalf("last row line = %d want 1", last.line)
	}
}

func TestCursorRowFindsWrappedCursor(t *testing.T) {
	tb := newTestTab("aaaa bbbb cccc")
	tb.cfg = &Config{TabWidth: 8, Wrap: true}
	e := &Editor{tabs: []*Tab{tb}, cfg: tb.cfg, width: 20, height: 10}
	rows := e.visibleRows(tb, 5, 8)
	tb.cur = Pos{0, 12} // inside the third chunk
	ri := cursorRow(rows, tb.cur)
	if ri < 0 {
		t.Fatalf("cursor row not found in %v", rows)
	}
	if tb.cur.Col < rows[ri].start || tb.cur.Col > rows[ri].end {
		t.Fatalf("row %v does not contain col %d", rows[ri], tb.cur.Col)
	}
}

func TestWrapEnsureVisibleKeepsCursorOnScreen(t *testing.T) {
	tb := newTestTab("aaaa bbbb cccc dddd eeee ffff\nnext")
	tb.cfg = &Config{TabWidth: 8, Wrap: true}
	e := &Editor{tabs: []*Tab{tb}, cfg: tb.cfg, width: 20, height: 6}
	tb.cur = Pos{1, 4}
	e.ensureVisible(5, 3)
	rows := e.visibleRows(tb, 5, 3)
	if cursorRow(rows, tb.cur) < 0 {
		t.Fatalf("cursor off screen: top=%d sub=%d rows=%v", tb.top, tb.topSub, rows)
	}
}

func TestNoWrapKeepsOneRowPerLine(t *testing.T) {
	tb := newTestTab("a very long line that would wrap\nsecond")
	tb.cfg = DefaultConfig()
	e := &Editor{tabs: []*Tab{tb}, cfg: tb.cfg, width: 10, height: 10}
	rows := e.visibleRows(tb, 5, 5)
	if len(rows) != 2 {
		t.Fatalf("rows %v want one per line", rows)
	}
}

func TestResizeRefollowsCursor(t *testing.T) {
	tb := newTestTab("1\n2\n3\n4\n5\n6\n7\n8\n9\n10")
	tb.cfg = DefaultConfig()
	e := &Editor{tabs: []*Tab{tb}, cfg: tb.cfg}
	tb.cur = Pos{9, 0}
	e.ensureVisible(20, 10) // cursor visible: top 0
	if tb.top != 0 {
		t.Fatalf("top = %d", tb.top)
	}
	// The window shrinks; the cursor would fall off the bottom.
	e.ensureVisible(20, 3)
	if tb.cur.Line < tb.top || tb.cur.Line >= tb.top+3 {
		t.Fatalf("after resize cursor line %d not in view [%d,%d)", tb.cur.Line, tb.top, tb.top+3)
	}
}
