package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestOpenPathDedupes: opening the same file twice used to create two buffers
// that could diverge and overwrite each other.
func TestOpenPathDedupes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	os.WriteFile(path, []byte("one\n"), 0o644)
	e := &Editor{cfg: DefaultConfig()}
	e.openPath(path)
	e.openPath(path)
	if len(e.tabs) != 1 {
		t.Fatalf("tabs = %d want 1", len(e.tabs))
	}
	// Also via a different spelling of the same path.
	e.openPath(filepath.Join(dir, ".", "a.txt"))
	if len(e.tabs) != 1 {
		t.Fatalf("tabs = %d after a relative spelling", len(e.tabs))
	}
	if e.cur != 0 {
		t.Fatalf("cur = %d, should focus the existing tab", e.cur)
	}
}

// TestOpenTabClosesReplacedHighlighter: replacing the pristine buffer leaked
// its highlighter goroutine.
func TestOpenTabClosesReplacedHighlighter(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.newTab()
	replaced := e.tabs[0]
	if replaced.hl == nil {
		t.Fatal("a fresh buffer should have a highlighter to leak")
	}
	e.openTab(newTabWith(e.config()))
	if len(e.tabs) != 1 {
		t.Fatalf("tabs = %d, the pristine buffer should be replaced", len(e.tabs))
	}
	if replaced.hl != nil {
		t.Fatal("the replaced tab's highlighter must be closed and cleared")
	}
}

func TestGotoTab(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	for i := 0; i < 3; i++ {
		e.tabs = append(e.tabs, newTabWith(e.config()))
	}
	e.gotoTab(2)
	if e.cur != 2 {
		t.Fatalf("cur = %d", e.cur)
	}
	e.gotoTab(9) // out of range: no change
	if e.cur != 2 {
		t.Fatalf("cur = %d after an out-of-range jump", e.cur)
	}
}

func TestAltDigitSwitchesTab(t *testing.T) {
	e := &Editor{cfg: DefaultConfig(), width: 40, height: 12}
	e.browser = NewBrowser(e)
	for i := 0; i < 3; i++ {
		e.tabs = append(e.tabs, newTabWith(e.config()))
	}
	e.handle(altRuneEvent('3'))
	if e.cur != 2 {
		t.Fatalf("Alt+3 selected tab %d want index 2", e.cur)
	}
}

func TestSaveOfNewFileSetsName(t *testing.T) {
	dir := t.TempDir()
	e := &Editor{cfg: DefaultConfig()}
	e.newTab()
	tb := e.tabs[0]
	tb.insertRune('x')
	path := filepath.Join(dir, "fresh.txt")
	if !e.writeTab(tb, path) {
		t.Fatalf("write failed: %s", e.msg)
	}
	if tb.name != "fresh.txt" || tb.dirty {
		t.Fatalf("name=%q dirty=%v", tb.name, tb.dirty)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "x" {
		t.Fatalf("content %q", got)
	}
}

// TestOverwritePromptOnExternalChange: saving over a file that changed on disk
// must ask first.
func TestOverwritePromptOnExternalChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("mine\n"), 0o644)
	e := &Editor{cfg: DefaultConfig()}
	e.openPath(path)
	tb := e.tabs[0]
	tb.cur = Pos{0, 4}
	tb.insertRune('!')
	// Someone else rewrites it.
	tb.diskSize = 999 // simulate a differing stat without racing the clock
	e.writeTabChecked(tb, path, nil)
	if e.prompt == nil {
		t.Fatal("an external change should raise a confirmation prompt")
	}
	// Answering no leaves the file alone.
	e.prompt.input = []rune("n")
	e.promptKey(keyEvent(tcellKeyEnter))
	got, _ := os.ReadFile(path)
	if string(got) != "mine\n" {
		t.Fatalf("file overwritten despite declining: %q", got)
	}
	// Answering yes writes it.
	tb.diskSize = 999
	e.writeTabChecked(tb, path, nil)
	e.prompt.input = []rune("y")
	e.promptKey(keyEvent(tcellKeyEnter))
	got, _ = os.ReadFile(path)
	if string(got) != "mine!\n" {
		t.Fatalf("content %q", got)
	}
}

func TestExitCancelledByEscape(t *testing.T) {
	e := &Editor{cfg: DefaultConfig(), running: true}
	tb := newTabWith(e.config())
	tb.insertRune('x')
	e.tabs = []*Tab{tb}
	e.exit()
	if e.prompt == nil {
		t.Fatal("a dirty buffer should prompt on exit")
	}
	// Esc dismisses the prompt: the exit sequence must unwind, not linger.
	e.promptKey(keyEvent(tcellKeyEsc))
	if e.exitPending {
		t.Fatal("exitPending should be cleared when the prompt is dismissed")
	}
	if !e.running {
		t.Fatal("dismissing the prompt must not quit")
	}
}

func TestReadOnlyFlagFromCLI(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.newTab()
	e.SetReadOnly(true)
	if !e.tabs[0].readOnly {
		t.Fatal("--view should mark buffers read-only")
	}
}

// TestCtrlQQuits covers the exit dispatch itself: some terminals swallow ^Q as
// flow control, so the binding is verified here rather than through a pty.
func TestCtrlQQuits(t *testing.T) {
	e := &Editor{cfg: DefaultConfig(), running: true, width: 40, height: 12}
	e.browser = NewBrowser(e)
	e.newTab() // clean buffer: no save prompt
	e.handle(keyEvent(tcell.KeyCtrlQ))
	if e.running {
		t.Fatal("^Q should quit a session with no unsaved work")
	}
	if e.prompt != nil {
		t.Fatal("no prompt expected for a clean buffer")
	}
}

func TestAltQQuits(t *testing.T) {
	e := &Editor{cfg: DefaultConfig(), running: true, width: 40, height: 12}
	e.browser = NewBrowser(e)
	e.newTab()
	e.handle(altRuneEvent('q'))
	if e.running {
		t.Fatal("Alt+Q should quit")
	}
}

func TestCtrlQPromptsWhenDirty(t *testing.T) {
	e := &Editor{cfg: DefaultConfig(), running: true, width: 40, height: 12}
	e.browser = NewBrowser(e)
	e.newTab()
	e.tabs[0].insertRune('x')
	e.handle(keyEvent(tcell.KeyCtrlQ))
	if !e.running {
		t.Fatal("must not quit before asking about unsaved work")
	}
	if e.prompt == nil {
		t.Fatal("expected a save prompt")
	}
}

// TestOpenErrorSurvivesFallbackTab: opening a binary file must leave its error
// on screen, not have it wiped by the empty buffer created afterwards.
func TestOpenErrorSurvivesFallbackTab(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "a.bin")
	os.WriteFile(bin, []byte{'a', 0, 'b'}, 0o644)
	e := &Editor{cfg: DefaultConfig()}
	e.openPath(bin)
	if len(e.tabs) != 0 {
		t.Fatalf("a refused file should not open a tab: %d", len(e.tabs))
	}
	msg := e.msg
	if msg == "" {
		t.Fatal("no error reported")
	}
	e.newTab() // what main() does when nothing opened
	if e.msg != msg {
		t.Fatalf("error message lost: %q -> %q", msg, e.msg)
	}
	if !strings.Contains(e.msg, "binary") {
		t.Fatalf("message %q should explain why", e.msg)
	}
}
