package editor

import (
	"os"
	"path/filepath"
	"strings"
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
	// UpdateFile re-tags a single file after a save, so an edit does not cost
	// a full re-index of the project.
	UpdateFile(path string) error
	Definitions(sym string) []Loc
	// Usages scans for occurrences, aborting when cancel is closed so a
	// superseded lookup stops doing work.
	Usages(sym string, cancel <-chan struct{}) []Loc
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
	seq  uint64
	sym  string
	view symView
	locs []Loc
}

// buildDone signals that a background index build finished. The editor then
// runs any lookup that was queued while the index was being built.
type buildDone struct {
	err     error
	partial bool // a single-file update, not a full index build
}

// symbolRoot returns the project root used for symbol indexing.
func (e *Editor) symbolRoot() string { return e.projectRoot() }

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
		// Cancel any in-flight scan: its result is no longer wanted.
		if e.symCancel != nil {
			close(e.symCancel)
		}
		cancel := make(chan struct{})
		e.symCancel = cancel
		e.symSeq++
		seq := e.symSeq
		go func() {
			locs := p.Usages(sym, cancel)
			e.post(symbolEvent{seq: seq, sym: sym, view: view, locs: locs})
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
		e.post(buildDone{err: err})
	}()
}

// onBuildDone is the event-loop half of a finished background build.
func (e *Editor) onBuildDone(bd buildDone) {
	if bd.partial {
		// A single-file refresh: report a failure, but never restart a build.
		if bd.err != nil && e.symPending != nil {
			e.errorf("Symbol index update failed: %v", bd.err)
		}
		return
	}
	e.symBuilding = false
	if bd.err != nil {
		if e.symPending != nil {
			e.errorf("Symbol lookup failed: %v", bd.err)
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
	if se.seq != 0 && se.seq != e.symSeq {
		return // superseded by a newer lookup
	}
	if e.symLast.sym != se.sym {
		return // stale result for a superseded lookup
	}
	e.symCancel = nil
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
		if view == viewUsages && e.symProvider != nil {
			for _, d := range e.symProvider.Definitions(sym) {
				defs[locKey(d)] = true
			}
		}
		e.openResults(sym, view, locs, defs)
	}
}

// jumpTo moves the cursor to loc, opening the file if needed. The previous
// position is pushed onto the jump stack so Alt+B returns to it.
func (e *Editor) jumpTo(sym string, loc Loc) {
	loc = e.resolveCol(sym, loc)
	e.symLast.file = loc.File
	e.pushJump()
	t := e.active()
	if t != nil && t.path != "" && samePath(t.path, loc.File) {
		e.setCursor(loc.Line, loc.Col)
		e.statusf("")
		return
	}
	// openPath switches to the tab that already holds the file, if any.
	e.openPath(loc.File)
	e.setCursor(loc.Line, loc.Col)
}

// pushJump records the current position for Alt+B.
func (e *Editor) pushJump() {
	if t := e.active(); t != nil {
		e.pushJumpAt(t.cur)
	}
}

// pushJumpAt records an explicit position for Alt+B.
func (e *Editor) pushJumpAt(p Pos) {
	t := e.active()
	if t == nil {
		return
	}
	loc := Loc{File: t.path, Line: p.Line, Col: p.Col}
	if n := len(e.jumps); n > 0 {
		last := e.jumps[n-1]
		if last.File == loc.File && last.Line == loc.Line {
			return // don't stack near-duplicates
		}
	}
	e.jumps = append(e.jumps, loc)
	if len(e.jumps) > 50 {
		e.jumps = e.jumps[len(e.jumps)-50:]
	}
}

// jumpBack pops the jump stack and returns to that position.
func (e *Editor) jumpBack() {
	for len(e.jumps) > 0 {
		loc := e.jumps[len(e.jumps)-1]
		e.jumps = e.jumps[:len(e.jumps)-1]
		if loc.File == "" {
			if t := e.active(); t != nil && t.path == "" {
				e.setCursor(loc.Line, loc.Col)
				return
			}
			continue
		}
		if i := e.tabForPath(loc.File); i >= 0 {
			e.cur = i
			e.setCursor(loc.Line, loc.Col)
			return
		}
		e.openPath(loc.File)
		e.setCursor(loc.Line, loc.Col)
		return
	}
	e.statusf("No position to jump back to")
}

// updateSymbolFile refreshes the tag index for one saved file instead of
// re-running ctags over the whole project.
func (e *Editor) updateSymbolFile(path string) {
	p := e.symProvider
	if p == nil || path == "" {
		return
	}
	if !p.Ready() {
		return // a full build is pending or has never run; nothing to patch
	}
	if e.symBuilding {
		e.symRebuild = true
		return
	}
	go func() {
		err := p.UpdateFile(path)
		e.post(buildDone{err: err, partial: true})
	}()
}

// resolveCol fills in a rune column for a definition, which ctags does not
// report reliably: it word-scans the target line for the symbol.
func (e *Editor) resolveCol(sym string, loc Loc) Loc {
	if loc.Col > 0 || sym == "" {
		return loc
	}
	// Prefer the open buffer: it is authoritative and needs no disk read.
	if i := e.tabForPath(loc.File); i >= 0 {
		if line := e.tabs[i].line(loc.Line); line != nil {
			if c := findWordCol(sym, line); c >= 0 {
				loc.Col = c
			}
			return loc
		}
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
