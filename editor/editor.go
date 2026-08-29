package editor

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"

	"goat/syntax"
)

type Mode int

const (
	ModeNormal Mode = iota
	ModePrompt
	ModeHelp
	ModePicker
	ModeResults
)

type Focus int

const (
	FocusText Focus = iota
	FocusBrowser
)

const (
	tabBarH   = 1
	statusH   = 2
	gutterMin = 4
)

// signalEvent carries a terminating signal into the event loop, so the
// buffers are inspected on the goroutine that owns them.
type signalEvent struct{ sig os.Signal }

// Editor owns the screen, tab list, and top-level state.
type Editor struct {
	screen       tcell.Screen
	cfg          *Config
	tabs         []*Tab
	cur          int
	mode         Mode
	focus        Focus
	msg          string
	msgErr       bool // msg reports a failure, not just information
	prompt       *Prompt
	promptStack  []*Prompt // prompts interrupted by another prompt
	history      map[string][]string
	picker       *Picker
	results      *Results
	symProvider  SymbolProvider
	symLast      symLookup
	symPending   *symbolEvent  // lookup queued until the index build finishes
	symBuilding  bool          // an index build is in flight
	symRebuild   bool          // a save happened during a build; rebuild after
	symSeq       uint64        // generation counter, to drop stale async results
	symCancel    chan struct{} // closed to abort an in-flight usages scan
	jumps        []Loc         // jump-back stack for Alt+D
	root         string        // project root for the file picker (abs), "" = launch cwd
	recent       []string
	fileIndex    *fileIndex // cached picker index, shared across ^P invocations
	browser      *Browser
	git          gitLookup // cached git branch shown in the status bar
	theme        *syntax.Theme
	clip         []rune
	search       Search
	replaceTo    string
	replaceScope *[4]int // when set, replace-all is limited to this region
	exitPending  bool
	exitIdx      int
	pasteActive  bool
	pasteBuf     []rune
	mouseDown    bool  // left mouse button held
	mouseDrag    bool  // the cursor moved since press (click vs drag)
	mouseAnchor  Pos   // position where the drag started
	mouseLast    int64 // ms timestamp of the last text click (double-click)
	mouseLine    int   // line of the last text click
	helpTop      int   // scroll offset for the help page
	width        int
	height       int
	frame        [][]frameCell
	running      bool
	tabRects     []tabRect
	rows         []row // display rows of the last frame, for mouse mapping
	rowsGutter   int
	rowsTextW    int
	wakePending  atomic.Bool
	closeOnce    sync.Once
}

// New initializes the terminal and returns an Editor.
func New() (*Editor, error) { return NewWithConfig(LoadConfig()) }

// NewWithConfig initializes the terminal with explicit settings.
func NewWithConfig(cfg *Config) (*Editor, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	s.EnableMouse(tcell.MouseButtonEvents | tcell.MouseMotionEvents)
	s.EnablePaste()
	w, h := s.Size()
	e := &Editor{
		screen: s,
		cfg:    cfg,
		theme:  syntax.ThemeByName(cfg.Theme),
		width:  w,
		height: h,
	}
	e.browser = NewBrowser(e)
	e.allocFrame()
	return e, nil
}

// Close restores the terminal and stops highlighter goroutines. Safe to call
// more than once.
func (e *Editor) Close() {
	e.closeOnce.Do(func() {
		for _, t := range e.tabs {
			t.close()
		}
		if e.screen != nil {
			e.screen.Fini()
		}
	})
}

// Run drives the event loop until exit. A panic is turned into an emergency
// save plus a restored terminal, so a crash never leaves the terminal unusable
// or the work unrecoverable.
func (e *Editor) Run() (err error) {
	defer func() {
		if r := recover(); r != nil {
			saved := e.emergencySave()
			stack := debug.Stack()
			e.Close()
			fmt.Fprintf(os.Stderr, "goat: internal error: %v\n\n%s\n", r, stack)
			for _, s := range saved {
				fmt.Fprintf(os.Stderr, "goat: unsaved buffer written to %s\n", s)
			}
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	defer e.Close()
	e.installSignals()
	e.reportConfigWarnings()
	e.running = true
	for e.running {
		e.draw()
		ev := e.screen.PollEvent()
		if ev == nil {
			break // screen finalized
		}
		e.handle(ev)
	}
	return err
}

// reportConfigWarnings surfaces config-file problems once the CLI has finished
// opening files, so the message is not immediately overwritten by them.
func (e *Editor) reportConfigWarnings() {
	cfg := e.config()
	if len(cfg.Warnings) == 0 || e.msgErr {
		return // never bury a real error (e.g. a file that failed to open)
	}
	name := filepath.Base(cfg.Path)
	if len(cfg.Warnings) == 1 {
		e.statusf("%s: %s", name, cfg.Warnings[0])
		return
	}
	e.statusf("%s: %s (+%d more)", name, cfg.Warnings[0], len(cfg.Warnings)-1)
}

// installSignals routes a terminating signal into the event loop so buffers
// can be saved and the terminal restored before exiting.
func (e *Editor) installSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		for sig := range ch {
			if e.screen != nil {
				e.screen.PostEvent(tcell.NewEventInterrupt(signalEvent{sig: sig}))
			}
		}
	}()
}

// emergencySave writes every modified buffer next to its file with a .save
// suffix and returns the paths written.
func (e *Editor) emergencySave() []string {
	var out []string
	for i, t := range e.tabs {
		if t == nil || !t.dirty {
			continue
		}
		path := t.path + ".save"
		if t.path == "" {
			path = sprintf("goat-%d.save", i+1)
			if abs, err := filepath.Abs(path); err == nil {
				path = abs
			}
		}
		if err := os.WriteFile(path, t.bytes(), 0600); err == nil {
			out = append(out, path)
		}
	}
	return out
}

// wakeup asks the event loop to redraw (called from worker goroutines).
func (e *Editor) wakeup() { e.post(nil) }

// post delivers a payload to the event loop. Safe to call from a worker
// goroutine, and a no-op when there is no screen (tests).
func (e *Editor) post(data any) {
	if e.screen == nil {
		return
	}
	_ = e.screen.PostEvent(tcell.NewEventInterrupt(data))
}

// scheduleWake redraws once after d, coalescing overlapping requests. Used to
// finish a pending highlight snapshot when typing stops.
func (e *Editor) scheduleWake(d time.Duration) {
	if e.wakePending.Swap(true) {
		return
	}
	time.AfterFunc(d, func() {
		e.wakePending.Store(false)
		e.wakeup()
	})
}

func (e *Editor) active() *Tab {
	if e.cur < 0 || e.cur >= len(e.tabs) {
		return nil
	}
	return e.tabs[e.cur]
}

func (e *Editor) config() *Config {
	if e.cfg == nil {
		e.cfg = DefaultConfig()
	}
	return e.cfg
}

func (e *Editor) quit() { e.running = false }

// focusText returns keyboard focus to the buffer and hides the browser.
func (e *Editor) focusText() {
	e.focus = FocusText
	if e.browser != nil {
		e.browser.open = false
	}
}

// statusf sets an informational status message.
func (e *Editor) statusf(format string, args ...any) {
	e.msg = fmt.Sprintf(format, args...)
	e.msgErr = false
}

// errorf sets a status message that reports a failure. Errors outrank
// informational messages, so a startup notice cannot bury one.
func (e *Editor) errorf(format string, args ...any) {
	e.msg = fmt.Sprintf(format, args...)
	e.msgErr = true
}

func (e *Editor) clearMsg() { e.msg, e.msgErr = "", false }

// --- layout --------------------------------------------------------------

func (e *Editor) mainTop() int { return tabBarH }
func (e *Editor) mainHeight() int {
	h := e.height - tabBarH - statusH
	if h < 1 {
		h = 1
	}
	return h
}

// editorLeft returns the left edge of the text pane. The browser covers the
// whole screen while focused, so the text pane always starts at column 0.
func (e *Editor) editorLeft() int { return 0 }

func (e *Editor) editorWidth() int {
	w := e.width - e.editorLeft()
	if w < 1 {
		w = 1
	}
	return w
}

func (e *Editor) gutterWidth() int {
	n := 1
	if t := e.active(); t != nil {
		n = digits(t.lineCount())
	}
	if n < gutterMin-2 {
		n = gutterMin - 2
	}
	return n + 2
}

func digits(n int) int {
	d := 1
	for n >= 10 {
		n /= 10
		d++
	}
	return d
}
