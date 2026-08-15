package editor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// Loc identifies a position in a file.
type Loc struct {
	File string
	Line int // 0-based
	Col  int // 0-based rune column
}

// SymbolProvider resolves definitions and usages of a symbol. A provider may
// be built lazily (ctags index, language server) and is built in the
// background so the UI never blocks.
type SymbolProvider interface {
	Ready() bool
	Build() error
	Definitions(sym string) []Loc
	Usages(sym string) []Loc
}

// symView distinguishes the two result views shown for a symbol.
type symView int

const (
	viewDefs symView = iota
	viewUsages
)

// symLookup remembers the last Alt+D lookup so a second press toggles between
// definitions and usages.
type symLookup struct {
	sym  string
	file string
	view symView
}

// symbolEvent carries async symbol results from a worker to the event loop.
type symbolEvent struct {
	sym  string
	view symView
	locs []Loc
}

// buildDone signals that a background index build finished. The editor then
// runs any lookup that was queued while the index was being built.
type buildDone struct {
	err error
}

// symbolRoot returns the project root used for symbol indexing.
func (e *Editor) symbolRoot() string {
	if e.root != "" {
		return e.root
	}
	if e.browser != nil && e.browser.root != "" {
		return e.browser.root
	}
	cwd, _ := os.Getwd()
	return cwd
}

// wordAtCursor returns the identifier spanning the cursor, or "" if the cursor
// is not on a word character.
func wordAtCursor(t *Tab) string {
	line := t.line(t.cur.Line)
	col := t.cur.Col
	start, end := col, col
	for start > 0 && isWord(line[start-1]) {
		start--
	}
	for end < len(line) && isWord(line[end]) {
		end++
	}
	if start >= end {
		return ""
	}
	return string(line[start:end])
}

// gotoSymbol implements Alt+D: definitions on the first press, usages on the
// next press for the same symbol in the same file.
func (e *Editor) gotoSymbol() {
	t := e.active()
	if t == nil {
		return
	}
	if e.results != nil {
		e.toggleResults()
		return
	}
	word := wordAtCursor(t)
	if word == "" {
		e.statusf("No symbol under cursor")
		return
	}
	if e.symLast.sym == word && e.symLast.file == t.path {
		e.symLast.view = 1 - e.symLast.view
	} else {
		e.symLast = symLookup{sym: word, file: t.path, view: viewDefs}
	}
	e.runLookup(word, e.symLast.view)
}

// ensureSymbolProvider lazily creates the symbol provider for the project.
func (e *Editor) ensureSymbolProvider() SymbolProvider {
	if e.symProvider == nil {
		e.symProvider = newCtagsIndex(e.symbolRoot())
	}
	return e.symProvider
}

// runLookup resolves a symbol. If the index is still building, the lookup is
// queued and run automatically once the build finishes, so the "Building
// symbol index..." message never lingers after the build completes.
func (e *Editor) runLookup(sym string, view symView) {
	p := e.ensureSymbolProvider()
	if !p.Ready() {
		e.statusf("Building symbol index...")
		e.symPending = &symbolEvent{sym: sym, view: view}
		e.startBuild()
		return
	}
	e.symPending = nil
	if view == viewUsages {
		e.statusf("Searching usages of %q...", sym)
		go func() {
			locs := p.Usages(sym)
			e.screen.PostEvent(tcell.NewEventInterrupt(symbolEvent{sym: sym, view: view, locs: locs}))
		}()
		return
	}
	e.routeResults(sym, view, p.Definitions(sym))
}

// startBuild kicks off a background index build if none is in flight. A build
// requested while one is already running is coalesced into a follow-up build.
func (e *Editor) startBuild() {
	if e.symBuilding {
		e.symRebuild = true
		return
	}
	e.symBuilding = true
	go func() {
		err := e.symProvider.Build()
		e.screen.PostEvent(tcell.NewEventInterrupt(buildDone{err: err}))
	}()
}

// onBuildDone is the event-loop half of a finished background build.
func (e *Editor) onBuildDone(bd buildDone) {
	e.symBuilding = false
	if bd.err != nil {
		if e.symPending != nil {
			e.statusf("Symbol lookup failed: %v", bd.err)
			e.symPending = nil
		}
		e.symRebuild = false
		return
	}
	if e.symRebuild {
		e.symRebuild = false
		e.startBuild()
		return
	}
	if e.symPending != nil {
		pending := *e.symPending
		e.symPending = nil
		e.runLookup(pending.sym, pending.view)
	}
}

// onSymbolEvent is the event-loop half of async symbol results.
func (e *Editor) onSymbolEvent(se symbolEvent) {
	if e.symLast.sym != se.sym {
		return // stale result for a superseded lookup
	}
	e.routeResults(se.sym, se.view, se.locs)
}

// routeResults jumps directly for a single hit, shows the overlay otherwise.
func (e *Editor) routeResults(sym string, view symView, locs []Loc) {
	e.symLast.view = view
	switch {
	case len(locs) == 0:
		e.statusf("No %s found for %q", viewNoun(view), sym)
	case len(locs) == 1:
		e.closeResults()
		e.jumpTo(sym, locs[0])
	default:
		defs := map[string]bool{}
		if view == viewUsages {
			for _, d := range e.symProvider.Definitions(sym) {
				defs[locKey(d)] = true
			}
		}
		e.openResults(sym, view, locs, defs)
	}
}

// jumpTo moves the cursor to loc, opening the file if needed.
func (e *Editor) jumpTo(sym string, loc Loc) {
	loc = e.resolveCol(sym, loc)
	e.symLast.file = loc.File
	t := e.active()
	if t != nil && t.path != "" && samePath(t.path, loc.File) {
		e.setCursor(loc.Line, loc.Col)
		e.statusf("")
		return
	}
	e.openPath(loc.File)
	e.setCursor(loc.Line, loc.Col)
}

// resolveCol fills in a rune column for a definition, which ctags does not
// report reliably: it word-scans the target line for the symbol.
func (e *Editor) resolveCol(sym string, loc Loc) Loc {
	if loc.Col > 0 || sym == "" {
		return loc
	}
	data, err := os.ReadFile(loc.File)
	if err != nil {
		return loc
	}
	lines := strings.Split(string(data), "\n")
	if loc.Line < 0 || loc.Line >= len(lines) {
		return loc
	}
	if c := findWordCol(sym, []rune(lines[loc.Line])); c >= 0 {
		loc.Col = c
	}
	return loc
}

// findWordCol returns the rune column of the first whole-word occurrence of
// sym in line, or -1 if absent.
func findWordCol(sym string, line []rune) int {
	for i := 0; i < len(line); {
		if isWord(line[i]) {
			j := i
			for j < len(line) && isWord(line[j]) {
				j++
			}
			if string(line[i:j]) == sym {
				return i
			}
			i = j
		} else {
			i++
		}
	}
	return -1
}

func samePath(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa != "" && bb != "" && aa == bb
}

func locKey(loc Loc) string { return sprintf("%s:%d", loc.File, loc.Line) }

func viewNoun(v symView) string {
	if v == viewUsages {
		return "usage"
	}
	return "definition"
}

func viewLabel(v symView) string {
	if v == viewUsages {
		return "Usages"
	}
	return "Definitions"
}
