package editor

import (
	"testing"

	"github.com/gdamore/tcell/v2"
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
