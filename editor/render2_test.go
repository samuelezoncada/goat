package editor

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"goat/syntax"
)

// simScreen renders into a grid so tests can assert what the user sees.
func simScreen(t *testing.T, w, h int) tcell.SimulationScreen {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	s.SetSize(w, h)
	t.Cleanup(s.Fini)
	return s
}

func editorOnScreen(t *testing.T, s tcell.SimulationScreen, tb *Tab) *Editor {
	t.Helper()
	w, h := s.Size()
	e := &Editor{screen: s, cfg: DefaultConfig(), theme: syntax.DefaultTheme(), width: w, height: h}
	if tb != nil {
		tb.cfg = e.cfg
		e.tabs = []*Tab{tb}
	}
	e.browser = NewBrowser(e)
	e.allocFrame()
	return e
}

// screenLine returns the rendered text of one screen row.
func screenLine(s tcell.SimulationScreen, y int) string {
	w, _ := s.Size()
	var b strings.Builder
	for x := 0; x < w; x++ {
		r, comb, _, _ := s.GetContent(x, y)
		b.WriteRune(r)
		for _, c := range comb {
			b.WriteRune(c)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

func TestRenderCombiningRuneStaysInOneCell(t *testing.T) {
	s := simScreen(t, 40, 8)
	tb := newTestTab("éx")
	e := editorOnScreen(t, s, tb)
	e.draw()
	line := screenLine(s, e.mainTop())
	if !strings.Contains(line, "éx") {
		t.Fatalf("row = %q, want the combining pair drawn in one cell followed by x", line)
	}
	// The 'x' must sit immediately after the base rune's single cell.
	gutter := e.gutterWidth()
	r, _, _, _ := s.GetContent(gutter+1, e.mainTop())
	if r != 'x' {
		t.Fatalf("cell after the cluster = %q want x", r)
	}
}

func TestRenderRawByteMarker(t *testing.T) {
	s := simScreen(t, 40, 8)
	tb := &Tab{text: newText([]byte{'a', 0xFF, 'b'}), cfg: DefaultConfig()}
	e := editorOnScreen(t, s, tb)
	e.draw()
	line := screenLine(s, e.mainTop())
	if !strings.Contains(line, "a·b") {
		t.Fatalf("row = %q, want the invalid byte shown as a marker", line)
	}
}

// TestRenderHorizontalScrollSkipsLeftPart checks the renderer draws from the
// scroll offset rather than walking the whole line.
func TestRenderHorizontalScroll(t *testing.T) {
	s := simScreen(t, 30, 8)
	long := strings.Repeat("ab", 100) // 200 cells
	tb := newTestTab(long)
	e := editorOnScreen(t, s, tb)
	tb.cur = Pos{0, 150}
	e.draw()
	row := screenLine(s, e.mainTop())
	if strings.HasPrefix(strings.TrimSpace(row), "1  ab") && tb.left == 0 {
		t.Fatalf("view did not scroll: left=%d row=%q", tb.left, row)
	}
	if tb.left == 0 {
		t.Fatal("cursor beyond the pane should scroll horizontally")
	}
	// The cursor's cell must be inside the pane.
	x := e.gutterWidth() + displayCol(tb.line(0), tb.cur.Col, 8) - tb.left
	if x < e.gutterWidth() || x >= e.width {
		t.Fatalf("cursor x = %d outside the pane", x)
	}
}

func TestRenderWrapShowsWholeLine(t *testing.T) {
	s := simScreen(t, 24, 10)
	tb := newTestTab("alpha beta gamma delta")
	e := editorOnScreen(t, s, tb)
	e.cfg.Wrap = true
	tb.cfg = e.cfg
	e.draw()
	var all strings.Builder
	for y := e.mainTop(); y < e.mainTop()+e.mainHeight(); y++ {
		all.WriteString(screenLine(s, y))
		all.WriteString("\n")
	}
	for _, word := range []string{"alpha", "beta", "gamma", "delta"} {
		if !strings.Contains(all.String(), word) {
			t.Fatalf("wrapped view is missing %q:\n%s", word, all.String())
		}
	}
}

func TestRenderTabExpansion(t *testing.T) {
	s := simScreen(t, 40, 8)
	tb := newTestTab("a\tb")
	e := editorOnScreen(t, s, tb)
	e.cfg.TabWidth = 4 // the tab shares the editor's config
	e.draw()
	g := e.gutterWidth()
	r, _, _, _ := s.GetContent(g+4, e.mainTop())
	if r != 'b' {
		t.Fatalf("with tabwidth 4, b should be at cell 4, found %q", r)
	}
}

func TestStatusShowsPositionAndEncoding(t *testing.T) {
	tb := &Tab{text: newText([]byte("a\tb")), cfg: &Config{TabWidth: 8}, trailingNL: true}
	tb.cur = Pos{0, 2}
	got := tb.statusRight()
	if !strings.Contains(got, "Ln 1/1") {
		t.Fatalf("status %q missing the line counter", got)
	}
	if !strings.Contains(got, "Col 3 (9)") {
		t.Fatalf("status %q should show both rune and display columns", got)
	}
	if !strings.Contains(got, "UTF-8") || !strings.Contains(got, "LF") {
		t.Fatalf("status %q missing encoding/EOL", got)
	}
	tb.rawBytes = true
	if !strings.Contains(tb.statusRight(), "UTF-8?") {
		t.Fatal("a non-UTF-8 buffer should be flagged in the status bar")
	}
	tb.trailingNL = false
	if !strings.Contains(tb.statusRight(), "no final newline") {
		t.Fatal("a missing final newline should be visible")
	}
}

func TestStatusHintsComeFromBindings(t *testing.T) {
	s := simScreen(t, 200, 8)
	e := editorOnScreen(t, s, newTestTab("x"))
	e.draw()
	hints := screenLine(s, e.height-1)
	for _, want := range []string{"^Q", "^O", "^W", "^G"} {
		if !strings.Contains(hints, want) {
			t.Fatalf("hint bar %q missing %q", hints, want)
		}
	}
}

func TestDrawDoesNotPanicOnTinyScreen(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {2, 3}, {5, 4}, {80, 4}} {
		s := simScreen(t, size[0], size[1])
		e := editorOnScreen(t, s, newTestTab("hello\nworld"))
		e.draw()
		e.mode = ModeHelp
		e.draw()
		e.mode = ModeNormal
		e.focus = FocusBrowser
		e.browser.open = true
		e.draw()
	}
}

func TestReadOnlyUndoBlocked(t *testing.T) {
	s := simScreen(t, 40, 8)
	tb := newTestTab("hello")
	e := editorOnScreen(t, s, tb)
	tb.cur = Pos{0, 5}
	tb.insertRune('!')
	tb.readOnly = true
	e.handle(keyEvent(tcell.KeyCtrlZ))
	if got := joinLines(t, tb); got != "hello!" {
		t.Fatalf("undo must not modify a read-only buffer, got %q", got)
	}
	// Once it is writable again, undo works.
	tb.readOnly = false
	e.handle(keyEvent(tcell.KeyCtrlZ))
	if got := joinLines(t, tb); got != "hello" {
		t.Fatalf("undo should work on a writable buffer, got %q", got)
	}
}

func TestStatusMessageClearsOnNextKey(t *testing.T) {
	s := simScreen(t, 60, 10)
	tb := newTestTab("hello")
	e := editorOnScreen(t, s, tb)
	e.statusf("Wrote 1 line")
	if e.msg == "" {
		t.Fatal("message not set")
	}
	e.handle(keyEvent(tcell.KeyRight))
	if e.msg != "" {
		t.Fatalf("msg = %q, a keystroke should clear it", e.msg)
	}
	// An action that reports something still gets to show its message.
	e.handle(altRuneEvent('c'))
	if e.msg == "" {
		t.Fatal("Alt+C should report the new case sensitivity")
	}
}

func TestStatusMessageWinsOverPositionOnNarrowScreen(t *testing.T) {
	s := simScreen(t, 60, 8)
	tb := newTestTab("x")
	e := editorOnScreen(t, s, tb)
	e.statusf("Error reading a.bin: binary file (contains NUL bytes)")
	e.draw()
	row := screenLine(s, e.height-2)
	if !strings.Contains(row, "NUL bytes") {
		t.Fatalf("status row %q truncated the message", row)
	}
	// With no message, the position fields are shown.
	e.clearMsg()
	e.frame = nil
	e.allocFrame()
	e.draw()
	row = screenLine(s, e.height-2)
	if !strings.Contains(row, "Ln 1/1") {
		t.Fatalf("status row %q should show the position", row)
	}
}
