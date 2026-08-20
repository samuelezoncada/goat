package editor

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"goat/syntax"
)

func TestPromptCtrlWRepeatsSearch(t *testing.T) {
	tb := newTestTab("foofoo")
	e := &Editor{tabs: []*Tab{tb}}
	e.search.text = "foo"
	e.search.dir = 1
	e.search.caseSens = true
	e.prompt = &Prompt{label: "Search: ", input: []rune("foo"), pos: 3}
	e.mode = ModePrompt
	tb.cur = Pos{0, 0}
	ev := tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModCtrl)
	e.promptKey(ev)
	if tb.cur.Col != 3 {
		t.Fatalf("expected col 3, got %d", tb.cur.Col)
	}
}

func TestPromptAltQReverseSearch(t *testing.T) {
	tb := newTestTab("foo foo bar")
	e := &Editor{tabs: []*Tab{tb}}
	e.search.text = "foo"
	e.search.dir = 1
	e.search.caseSens = true
	e.prompt = &Prompt{label: "Search: ", input: []rune("foo"), pos: 3}
	e.mode = ModePrompt
	tb.cur = Pos{0, 8} // past the second "foo"
	ev := tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModAlt)
	e.promptKey(ev)
	if tb.cur.Col != 4 {
		t.Fatalf("expected col 4, got %d", tb.cur.Col)
	}
}

func TestPromptCtrlVPaste(t *testing.T) {
	e := &Editor{clip: []rune("paste")}
	e.prompt = &Prompt{label: "X: ", input: []rune("ab"), pos: 1}
	e.mode = ModePrompt
	ev := tcell.NewEventKey(tcell.KeyCtrlV, 0, tcell.ModCtrl)
	e.promptKey(ev)
	if string(e.prompt.input) != "apasteb" || e.prompt.pos != 6 {
		t.Fatalf("input %q pos %d", string(e.prompt.input), e.prompt.pos)
	}
}

func TestPromptCtrlXCancels(t *testing.T) {
	e := &Editor{}
	e.prompt = &Prompt{label: "X: ", input: []rune("ab"), pos: 2}
	e.mode = ModePrompt
	cancelled := false
	e.prompt.onCancel = func() { cancelled = true }
	ev := tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModCtrl)
	e.promptKey(ev)
	if e.mode != ModeNormal || e.prompt != nil || !cancelled {
		t.Fatalf("prompt not cancelled: mode=%v prompt=%v cancelled=%v", e.mode, e.prompt, cancelled)
	}
}

// TestPromptLongInputScrolls guards against the caret and input scrolling
// off-screen when the prompt text is longer than the terminal width.
func TestPromptLongInputScrolls(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	scr.SetSize(30, 10)
	e := &Editor{screen: scr, theme: syntax.DefaultTheme(), width: 30, height: 10}
	e.allocFrame()
	e.beginPrompt("Search: ", "", nil, func(string) {})
	for _, r := range []rune("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		e.promptKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	e.drawPromptLine()
	scr.Show()
	cx, cy, _ := scr.GetCursor()
	if cy != e.height-2 {
		t.Fatalf("cursor row = %d want %d (cursor should stay visible)", cy, e.height-2)
	}
	if cx < 0 || cx >= e.width {
		t.Fatalf("cursor col = %d not on screen", cx)
	}
	// the caret should sit at the right edge (fully scrolled input)
	if cx != e.width-1 {
		t.Fatalf("cursor col = %d want %d", cx, e.width-1)
	}
}

func TestPromptInputWideRunesCursor(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	scr.SetSize(40, 10)
	e := &Editor{screen: scr, theme: syntax.DefaultTheme(), width: 40, height: 10}
	e.allocFrame()
	e.beginPrompt("Search: ", "", nil, func(string) {})
	for _, r := range []rune("日本abc") {
		e.promptKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	e.drawPromptLine()
	scr.Show()
	cx, _, _ := scr.GetCursor()
	// label "Search: " (8 cells) + two wide CJK runes (2 each = 4) + 3 ascii
	want := 1 + 8 + 4 + 3
	if cx != want {
		t.Fatalf("cursor col = %d want %d", cx, want)
	}
}
