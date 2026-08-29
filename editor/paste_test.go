package editor

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// pasteText feeds a bracketed paste the way tcell does: a start marker, the
// content as ordinary key events (newlines arrive as KeyEnter, tabs as
// KeyTab), then an end marker.
func pasteText(e *Editor, s string) {
	e.handle(newPasteEvent(true))
	for _, r := range s {
		switch r {
		case '\n':
			e.handle(tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
		case '\t':
			e.handle(tcell.NewEventKey(tcell.KeyTab, '\t', tcell.ModNone))
		default:
			e.handle(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
		}
	}
	e.handle(newPasteEvent(false))
}

func newPasteEvent(start bool) *tcell.EventPaste { return tcell.NewEventPaste(start) }

// TestPasteKeepsNewlines is the regression test for multi-line pastes losing
// their line breaks: only KeyRune events used to be collected.
func TestPasteKeepsNewlines(t *testing.T) {
	tb := newTestTab("")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	pasteText(e, "one\ntwo\n\tthree")
	want := "one\ntwo\n\tthree"
	if got := joinLines(t, tb); got != want {
		t.Fatalf("paste got %q want %q", got, want)
	}
	if tb.lineCount() != 3 {
		t.Fatalf("lines = %d want 3", tb.lineCount())
	}
	// The whole paste undoes in one step.
	tb.undo()
	if got := joinLines(t, tb); got != "" {
		t.Fatalf("after undo got %q", got)
	}
}

// TestPasteIntoPromptDoesNotTouchBuffer covers pasting while a prompt is open:
// the text belongs to the prompt, not to the document behind it.
func TestPasteIntoPromptDoesNotTouchBuffer(t *testing.T) {
	tb := newTestTab("body")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.beginPrompt("Search: ", "", nil, func(string) {})
	pasteText(e, "needle")
	if got := joinLines(t, tb); got != "body" {
		t.Fatalf("buffer changed by a paste meant for the prompt: %q", got)
	}
	if e.prompt == nil || string(e.prompt.input) != "needle" {
		t.Fatalf("prompt input = %q", string(e.prompt.input))
	}
}

// TestMouseIgnoredDuringPrompt covers a click landing in the buffer (or on a
// tab's close button) while a prompt is waiting for an answer.
func TestMouseIgnoredDuringPrompt(t *testing.T) {
	tb := newTestTab("hello\nworld")
	tb.cur = Pos{0, 0}
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig(), width: 40, height: 10}
	answered := false
	e.beginPrompt("Save? ", "", nil, func(string) { answered = true })
	e.handle(tcell.NewEventMouse(10, 2, tcell.Button1, tcell.ModNone))
	if tb.cur != (Pos{0, 0}) {
		t.Fatalf("cursor moved to %v while a prompt was open", tb.cur)
	}
	if e.prompt == nil {
		t.Fatal("the pending prompt was discarded by a click")
	}
	if answered {
		t.Fatal("prompt answered by a click")
	}
}

func TestMouseInPickerSelectsRow(t *testing.T) {
	e := &Editor{cfg: DefaultConfig(), width: 40, height: 12}
	e.fileIndex = &fileIndex{
		root:     ".",
		expanded: map[string]bool{},
		ready:    true,
		entries: []indexEntry{
			{path: "/a", rel: "a", relLower: "a"},
			{path: "/b", rel: "b", relLower: "b"},
			{path: "/c", rel: "c", relLower: "c"},
		},
	}
	p := &Picker{e: e}
	p.refilter()
	e.picker = p
	e.mode = ModePicker
	// Row 0 of the list is at mainTop()+1; click the third row.
	e.handle(tcell.NewEventMouse(3, e.mainTop()+3, tcell.Button1, tcell.ModNone))
	if p.sel != 2 {
		t.Fatalf("sel = %d want 2", p.sel)
	}
}

func TestHelpWheelScrolls(t *testing.T) {
	e := &Editor{cfg: DefaultConfig(), width: 80, height: 24, mode: ModeHelp}
	e.handle(tcell.NewEventMouse(1, 5, tcell.WheelDown, tcell.ModNone))
	if e.helpTop == 0 {
		t.Fatal("wheel should scroll the help page")
	}
	e.handle(tcell.NewEventMouse(1, 5, tcell.WheelUp, tcell.ModNone))
	e.handle(tcell.NewEventMouse(1, 5, tcell.WheelUp, tcell.ModNone))
	if e.helpTop != 0 {
		t.Fatalf("helpTop = %d, should clamp at 0", e.helpTop)
	}
}

func TestWheelScrollClamped(t *testing.T) {
	tb := newTestTab("a\nb\nc")
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig(), width: 40, height: 10}
	for i := 0; i < 20; i++ {
		e.handle(tcell.NewEventMouse(1, 3, tcell.WheelDown, tcell.ModNone))
	}
	if tb.top >= tb.lineCount() {
		t.Fatalf("top = %d scrolled past the end (%d lines)", tb.top, tb.lineCount())
	}
	for i := 0; i < 20; i++ {
		e.handle(tcell.NewEventMouse(1, 3, tcell.WheelUp, tcell.ModNone))
	}
	if tb.top != 0 {
		t.Fatalf("top = %d want 0", tb.top)
	}
}

func TestSignalEventSavesAndQuits(t *testing.T) {
	dir := t.TempDir()
	tb := newTestTab("work in progress")
	tb.path = dir + "/wip.txt"
	tb.dirty = true
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig(), running: true}
	e.handle(tcell.NewEventInterrupt(signalEvent{sig: fakeSignal("SIGTERM")}))
	if e.running {
		t.Fatal("a terminating signal should stop the loop")
	}
	if _, err := timeoutStat(tb.path + ".save"); err != nil {
		t.Fatalf("no emergency save written: %v", err)
	}
}

type fakeSignal string

func (f fakeSignal) String() string { return string(f) }
func (f fakeSignal) Signal()        {}

func timeoutStat(path string) (any, error) {
	deadline := time.Now().Add(time.Second)
	for {
		fi, err := statFile(path)
		if err == nil {
			return fi, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(5 * time.Millisecond)
	}
}
