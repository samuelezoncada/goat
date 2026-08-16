package editor

import (
	"fmt"

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

type frameCell struct {
	r rune
	s tcell.Style
}

// Editor owns the screen, tab list, and top-level state.
type Editor struct {
	screen      tcell.Screen
	tabs        []*Tab
	cur         int
	mode        Mode
	focus       Focus
	msg         string
	prompt      *Prompt
	picker      *Picker
	results     *Results
	symProvider SymbolProvider
	symLast     symLookup
	symPending  *symbolEvent // lookup queued until the index build finishes
	symBuilding bool         // an index build is in flight
	symRebuild  bool         // a save happened during a build; rebuild after
	root        string       // project root for the file picker (abs), "" = launch cwd
	recent      []string
	browser     *Browser
	theme       *syntax.Theme
	clip        []rune
	search      Search
	replaceTo   string
	exitPending bool
	exitIdx     int
	pasteActive bool
	pasteBuf    []rune
	mouseDown   bool  // left mouse button held
	mouseDrag   bool  // the cursor moved since press (click vs drag)
	mouseAnchor Pos   // position where the drag started
	mouseLast   int64 // ms timestamp of the last text click (double-click)
	mouseLine   int   // line of the last text click
	helpTop     int   // scroll offset for the help page
	width       int
	height      int
	frame       [][]frameCell
	running     bool
	tabRects    []tabRect
}

// New initializes the terminal and returns an Editor.
func New() (*Editor, error) {
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
		theme:  syntax.DefaultTheme(),
		width:  w,
		height: h,
	}
	e.browser = NewBrowser(e)
	e.allocFrame()
	return e, nil
}

// Close restores the terminal and stops highlighter goroutines.
func (e *Editor) Close() {
	for _, t := range e.tabs {
		t.close()
	}
	e.screen.Fini()
}

// Run drives the event loop until exit.
func (e *Editor) Run() error {
	defer e.Close()
	e.running = true
	for e.running {
		e.draw()
		ev := e.screen.PollEvent()
		e.handle(ev)
	}
	return nil
}

func (e *Editor) active() *Tab {
	if e.cur < 0 || e.cur >= len(e.tabs) {
		return nil
	}
	return e.tabs[e.cur]
}

func (e *Editor) quit() { e.running = false }

// focusText returns keyboard focus to the buffer and hides the browser.
func (e *Editor) focusText() {
	e.focus = FocusText
	if e.browser != nil {
		e.browser.open = false
	}
}

func (e *Editor) statusf(format string, args ...any) {
	e.msg = fmt.Sprintf(format, args...)
}

func (e *Editor) clearMsg() { e.msg = "" }

// --- layout --------------------------------------------------------------

func (e *Editor) mainTop() int { return tabBarH }
func (e *Editor) mainHeight() int {
	return e.height - tabBarH - statusH
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
