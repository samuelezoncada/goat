package editor

import (
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"

	"goat/syntax"
)

func TestCursorCell(t *testing.T) {
	if ch, w := cursorCell([]rune("abc"), 1); ch != 'b' || w != 1 {
		t.Fatalf("normal char = %q/%d", ch, w)
	}
	if ch, w := cursorCell([]rune("a\tb"), 1); ch != '\t' || w != tabStop-1 {
		t.Fatalf("tab = %q/%d", ch, w)
	}
	if ch, w := cursorCell([]rune("世b"), 0); ch != '世' || w != 2 {
		t.Fatalf("wide char = %q/%d", ch, w)
	}
	if ch, w := cursorCell([]rune("ab"), 5); ch != ' ' || w != 1 {
		t.Fatalf("past EOL = %q/%d", ch, w)
	}
	if ch, w := cursorCell([]rune("ab"), -1); ch != ' ' || w != 1 {
		t.Fatalf("negative col = %q/%d", ch, w)
	}
}

func TestDrawCursorBlock(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "a.go")
	tb := openTabAt(t, path, "package x\nfunc Foo() {}\n")

	e := &Editor{screen: scr, theme: syntax.DefaultTheme(), width: 80, height: 24}
	e.browser = NewBrowser(e)
	e.allocFrame()
	e.tabs = []*Tab{tb}
	e.cur = 0
	tb.cur = Pos{Line: 1, Col: 5}

	e.drawEditor()
	scr.Show()

	x := e.gutterWidth() + displayCol(tb.line(1), 5) - tb.left
	y := e.mainTop() + 1 - tb.top
	cells, w, _ := scr.GetContents()
	cell := cells[y*w+x]
	if string(cell.Runes) != "F" {
		t.Fatalf("cursor cell runes = %q want F", cell.Runes)
	}
	if cell.Style != blockCursorStyle {
		t.Fatalf("cursor cell style = %+v want block cursor style", cell.Style)
	}
}
