package editor

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"goat/syntax"
)

func TestHelpRendersTitle(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	scr.SetSize(80, 24)
	e := &Editor{screen: scr, theme: syntax.DefaultTheme(), width: 80, height: 24}
	e.allocFrame()
	e.drawHelp()
	scr.Show()

	cells, w, _ := scr.GetContents()
	if string(cells[1*w+2].Runes) != "g" {
		t.Fatalf("title cell = %q want g", cells[1*w+2].Runes)
	}
	// footer is the last row
	if !containsRunes(cells[(e.height-1)*w:e.height*w], []rune("close help")) {
		t.Fatal("footer not rendered")
	}
}

func TestHelpRendersSingleColumnNarrow(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	scr.SetSize(30, 20)
	e := &Editor{screen: scr, theme: syntax.DefaultTheme(), width: 30, height: 20}
	e.allocFrame()
	e.drawHelp() // must not panic on narrow terminals
	scr.Show()
}

func TestHelpKeyScroll(t *testing.T) {
	e := &Editor{width: 80, height: 24}
	e.openHelp()
	if e.mode != ModeHelp || e.helpTop != 0 {
		t.Fatalf("openHelp: mode=%v top=%d", e.mode, e.helpTop)
	}

	viewH := e.height - 3
	total := helpLineCount(true)

	e.helpKey(tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone))
	if e.helpTop != viewH {
		t.Fatalf("after PgDn top = %d want %d", e.helpTop, viewH)
	}
	e.helpKey(tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone))
	if e.helpTop != total-viewH {
		t.Fatalf("PgDn should clamp to %d, got %d", total-viewH, e.helpTop)
	}
	e.helpKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if e.helpTop != total-viewH {
		t.Fatalf("Down should clamp, got %d", e.helpTop)
	}
	e.helpKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if e.helpTop != total-viewH-1 {
		t.Fatalf("after Up top = %d want %d", e.helpTop, total-viewH-1)
	}

	// any other key closes help and resets the scroll
	e.helpKey(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
	if e.mode != ModeNormal || e.helpTop != 0 {
		t.Fatalf("Esc should close help: mode=%v top=%d", e.mode, e.helpTop)
	}
}

func containsRunes(cells []tcell.SimCell, sub []rune) bool {
	for i := range cells {
		if len(cells[i].Runes) > 0 && cells[i].Runes[0] == sub[0] {
			match := true
			for j := 1; j < len(sub); j++ {
				c := cells[i+j]
				if len(c.Runes) == 0 || c.Runes[0] != sub[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
