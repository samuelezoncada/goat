package editor

import (
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// Results is the definitions/usages overlay shared by both Alt+D views.
type Results struct {
	sym  string
	view symView
	locs []Loc
	defs map[string]bool // "file:line" of definition sites (usages view)
	sel  int
	top  int
}

func (e *Editor) openResults(sym string, view symView, locs []Loc, defs map[string]bool) {
	e.results = &Results{sym: sym, view: view, locs: locs, defs: defs}
	e.mode = ModeResults
	e.clearMsg()
}

func (e *Editor) closeResults() {
	if e.results == nil {
		return
	}
	e.results = nil
	if e.mode == ModeResults {
		e.mode = ModeNormal
	}
	if e.screen != nil {
		e.screen.HideCursor()
	}
}

// toggleResults switches the open overlay between definitions and usages.
func (e *Editor) toggleResults() {
	r := e.results
	if r == nil {
		return
	}
	r.view = 1 - r.view
	e.symLast.view = r.view
	e.runLookup(r.sym, r.view)
}

// resultsKey handles keys while the results overlay is open.
func (e *Editor) resultsKey(ev *tcell.EventKey) {
	r := e.results
	if r == nil {
		e.mode = ModeNormal
		return
	}
	switch ev.Key() {
	case tcell.KeyEsc, tcell.KeyCtrlC, tcell.KeyCtrlG:
		e.closeResults()
	case tcell.KeyEnter:
		e.openResult()
	case tcell.KeyUp:
		if r.sel > 0 {
			r.sel--
		}
	case tcell.KeyDown:
		if r.sel+1 < len(r.locs) {
			r.sel++
		}
	case tcell.KeyHome:
		r.sel = 0
	case tcell.KeyEnd:
		r.sel = len(r.locs) - 1
	case tcell.KeyPgUp:
		r.sel -= e.mainHeight() - 2
		if r.sel < 0 {
			r.sel = 0
		}
	case tcell.KeyPgDn:
		r.sel += e.mainHeight() - 2
		if r.sel >= len(r.locs) {
			r.sel = len(r.locs) - 1
		}
	case tcell.KeyRune:
		if ev.Modifiers()&(tcell.ModAlt|tcell.ModMeta) != 0 && ev.Rune() == 'd' {
			e.toggleResults()
		}
	}
}

// openResult jumps to the selected location and closes the overlay.
func (e *Editor) openResult() {
	r := e.results
	if r == nil || r.sel < 0 || r.sel >= len(r.locs) {
		return
	}
	loc := r.locs[r.sel]
	e.closeResults()
	e.jumpTo(r.sym, loc)
}

// drawResults renders the results overlay.
func (e *Editor) drawResults() {
	r := e.results
	if r == nil {
		return
	}
	bg := e.theme.Plain
	selStyle := bg.Reverse(true)
	defStyle := e.theme.Type
	titleStyle := statusStyle

	// title line
	y := e.mainTop()
	e.fillRow(0, e.width, y, titleStyle)
	label := sprintf("%s of %s", viewLabel(r.view), r.sym)
	e.putStr(1, y, truncateRunes(label, e.width-2), titleStyle)

	// result list
	listTop := y + 1
	listH := e.mainHeight() - 1
	if r.sel < r.top {
		r.top = r.sel
	}
	if r.sel >= r.top+listH {
		r.top = r.sel - listH + 1
	}
	if r.top < 0 {
		r.top = 0
	}

	for row := 0; row < listH; row++ {
		idx := r.top + row
		yy := listTop + row
		if yy >= e.height-1 {
			break
		}
		if idx >= len(r.locs) {
			e.fillRow(0, e.width, yy, bg)
			continue
		}
		loc := r.locs[idx]
		style := bg
		if r.defs[locKey(loc)] {
			style = defStyle
		}
		if idx == r.sel {
			style = selStyle
		}
		e.fillRow(0, e.width, yy, style)
		line := e.displayLoc(loc)
		e.putStr(2, yy, truncateRunes(line, e.width-4), style)
	}

	// footer
	fy := e.height - 1
	e.fillRow(0, e.width, fy, hintStyle)
	e.putStr(1, fy, sprintf(" %d match(es)   ↑/↓ move   Enter open   Alt+D toggle   Esc cancel", len(r.locs)), hintStyle)
	e.screen.HideCursor()
}

// displayLoc renders a location as "<relpath>:<line>".
func (e *Editor) displayLoc(loc Loc) string {
	rel, err := filepath.Rel(e.symbolRoot(), loc.File)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = loc.File
	}
	return sprintf("%s:%d", rel, loc.Line+1)
}
