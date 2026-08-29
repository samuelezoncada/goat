package editor

import (
	"time"

	"github.com/gdamore/tcell/v2"
)

// frameCell is one cached screen cell, used to skip unchanged writes.
type frameCell struct {
	r    rune
	comb []rune // combining runes attached to r
	s    tcell.Style
}

func (c *frameCell) same(r rune, comb []rune, s tcell.Style) bool {
	if c.r != r || c.s != s || len(c.comb) != len(comb) {
		return false
	}
	for i := range comb {
		if c.comb[i] != comb[i] {
			return false
		}
	}
	return true
}

func (e *Editor) allocFrame() {
	if e.frame == nil || len(e.frame) != e.height {
		f := make([][]frameCell, e.height)
		for y := range f {
			f[y] = make([]frameCell, e.width)
		}
		e.frame = f
	}
	for y := range e.frame {
		if len(e.frame[y]) != e.width {
			e.frame[y] = make([]frameCell, e.width)
			continue
		}
		for x := range e.frame[y] {
			e.frame[y][x] = frameCell{}
		}
	}
}

func (e *Editor) drawCell(x, y int, r rune, style tcell.Style) {
	e.drawCellComb(x, y, r, nil, style)
}

// drawCellComb writes a base rune plus any combining runes into one cell.
func (e *Editor) drawCellComb(x, y int, r rune, comb []rune, style tcell.Style) {
	if y < 0 || y >= len(e.frame) || x < 0 || x >= len(e.frame[y]) {
		return
	}
	c := &e.frame[y][x]
	if c.same(r, comb, style) {
		return
	}
	c.r = r
	c.comb = append(c.comb[:0], comb...)
	c.s = style
	e.screen.SetContent(x, y, r, comb, style)
}

// fillRow clears a run of cells with spaces.
func (e *Editor) fillRow(x0, x1, y int, style tcell.Style) {
	for x := x0; x < x1 && x < e.width; x++ {
		e.drawCell(x, y, ' ', style)
	}
}

func (e *Editor) fillRect(x0, x1, y0, y1 int, style tcell.Style) {
	for y := y0; y < y1 && y < e.height; y++ {
		e.fillRow(x0, x1, y, style)
	}
}

// putStr draws a string left-aligned at (x,y) using style.
func (e *Editor) putStr(x, y int, s string, style tcell.Style) int {
	for _, r := range s {
		if x >= e.width {
			break
		}
		w := runeWidth(r)
		if x >= 0 {
			e.drawCell(x, y, r, style)
		}
		if w == 0 {
			w = 1
		}
		x += w
	}
	return x
}

// --- viewport rows -------------------------------------------------------

// row is one display line: a buffer line, or a slice of it when wrapping.
type row struct {
	line  int  // buffer line index
	start int  // first rune column shown on this row
	end   int  // one past the last rune column shown
	first bool // first row of its buffer line (gutter shows the number)
}

// wrapStarts returns the rune column at which each display row of a line
// begins. Breaks happen at the last space that fits, else hard at the edge.
func wrapStarts(line []rune, width, tabW int) []int {
	starts := []int{0}
	if width < 1 || len(line) == 0 {
		return starts
	}
	col := 0 // display cells used on the current row
	lastSpace := -1
	i := 0
	rowStart := 0
	for i < len(line) {
		var w, end int
		if line[i] == '\t' {
			w = tabW - col%tabW
			end = i + 1
		} else {
			end, w = clusterEnd(line, i)
		}
		if col+w > width && i > rowStart {
			brk := i
			if lastSpace > rowStart {
				brk = lastSpace + 1
			}
			starts = append(starts, brk)
			rowStart = brk
			i = brk
			col = 0
			lastSpace = -1
			continue
		}
		if line[i] == ' ' || line[i] == '\t' {
			lastSpace = i
		}
		col += w
		i = end
	}
	return starts
}

// rowsForLine expands one buffer line into its display rows.
func (e *Editor) rowsForLine(t *Tab, lineIdx, textW int) []row {
	line := t.line(lineIdx)
	if !t.config().Wrap {
		return []row{{line: lineIdx, start: 0, end: len(line), first: true}}
	}
	starts := wrapStarts(line, textW, t.tabW())
	out := make([]row, 0, len(starts))
	for i, s := range starts {
		end := len(line)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		out = append(out, row{line: lineIdx, start: s, end: end, first: i == 0})
	}
	return out
}

// visibleRows computes the display rows filling the text pane, starting at the
// tab's scroll position.
func (e *Editor) visibleRows(t *Tab, textW, height int) []row {
	out := make([]row, 0, height)
	sub := t.topSub
	for li := t.top; li < t.lineCount() && len(out) < height; li++ {
		rs := e.rowsForLine(t, li, textW)
		if sub > 0 {
			if sub >= len(rs) {
				sub -= len(rs)
				continue
			}
			rs = rs[sub:]
			sub = 0
		}
		for _, r := range rs {
			if len(out) >= height {
				break
			}
			out = append(out, r)
		}
	}
	return out
}

// cursorRow returns the index within rows of the row holding the cursor, or -1.
func cursorRow(rows []row, cur Pos) int {
	for i, r := range rows {
		if r.line != cur.Line {
			continue
		}
		if cur.Col >= r.start && (cur.Col < r.end || (i+1 >= len(rows) || rows[i+1].line != r.line)) {
			return i
		}
	}
	return -1
}

// --- main draw -----------------------------------------------------------

func (e *Editor) draw() {
	// Hand the highlighter a snapshot if one is due, and make sure the render
	// loop wakes up again while edits are still pending.
	if t := e.active(); t != nil && t.hl != nil {
		t.hl.SetOnReady(e.wakeup)
		if t.flushHighlight(time.Now()) {
			e.scheduleWake(hlInterval)
		}
	}
	if e.frame == nil || len(e.frame) != e.height {
		e.allocFrame()
		e.screen.Clear()
	}
	switch e.mode {
	case ModeHelp:
		e.drawHelp()
		e.screen.Show()
		return
	case ModePicker:
		e.drawTabBar()
		e.drawPicker()
		e.screen.Show()
		return
	case ModeResults:
		e.drawTabBar()
		e.drawResults()
		e.screen.Show()
		return
	}
	e.drawTabBar()
	e.drawBrowser()
	if e.focus != FocusBrowser {
		e.drawEditor()
	}
	e.drawStatus()
	e.screen.Show()
}

func (e *Editor) drawEditor() {
	t := e.active()
	if t == nil {
		e.fillRect(e.editorLeft(), e.width, e.mainTop(), e.mainTop()+e.mainHeight(), e.theme.Plain)
		e.screen.HideCursor()
		e.rows = nil
		return
	}
	areaW := e.editorWidth()
	gutter := e.gutterWidth()
	textW := areaW - gutter
	if textW < 1 {
		textW = 1
	}
	e.ensureVisible(textW, e.mainHeight())

	sel := e.selection()
	rows := e.visibleRows(t, textW, e.mainHeight())
	e.rows = rows
	e.rowsGutter = gutter
	e.rowsTextW = textW

	for i := 0; i < e.mainHeight(); i++ {
		y := e.mainTop() + i
		e.fillRow(e.editorLeft(), e.width, y, e.theme.Plain)
		if i >= len(rows) {
			continue
		}
		r := rows[i]
		e.drawGutter(e.editorLeft(), y, r.line, gutter, r.first)
		e.drawTextRow(t, r, y, e.editorLeft()+gutter, textW, sel)
	}

	e.drawCursor(rows, gutter, textW)
}

func (e *Editor) drawGutter(x0, y, lineIdx, width int, showNum bool) {
	style := e.theme.Comment
	if t := e.active(); t != nil && lineIdx == t.cur.Line {
		style = e.theme.Plain.Reverse(true)
	}
	e.fillRow(x0, x0+width, y, style)
	if !showNum {
		return
	}
	s := itoa(lineIdx + 1)
	e.putStr(x0+width-1-len(s), y, s, style)
}

// drawTextRow renders one display row, skipping the columns scrolled off to
// the left instead of walking the whole line.
func (e *Editor) drawTextRow(t *Tab, r row, y, x0, textW int, sel [4]int) {
	line := t.line(r.line)
	spans := t.spans(r.line, line)
	matches := e.lineMatches(line)
	tabW := t.tabW()
	left := t.left
	if t.config().Wrap {
		left = 0
	}

	// Display offset of the row's first rune (0 unless wrapping mid-line).
	disp := 0
	if r.start > 0 {
		disp = displayCol(line, r.start, tabW)
	}
	rowBase := disp

	si := 0
	plain := e.theme.Plain
	for col := r.start; col < r.end && col < len(line); {
		for si < len(spans) && col >= spans[si].Start+spans[si].Len {
			si++
		}
		style := plain
		if si < len(spans) && col >= spans[si].Start && col < spans[si].Start+spans[si].Len {
			style = e.theme.Style(spans[si].Type)
		}
		if inMatch(matches, col) {
			style = e.theme.Match
		}
		if sel[0] >= 0 && inSelection(r.line, col, sel) {
			style = style.Reverse(true)
		}

		ch := line[col]
		var comb []rune
		var w, end int
		switch {
		case ch == '\t':
			w = tabW - disp%tabW
			end = col + 1
		case isRawByte(ch):
			// A byte that is not valid UTF-8: show a marker rather than a
			// random glyph, so it is visible that the file is not UTF-8.
			ch, w, end = '·', 1, col+1
			style = e.theme.Error
		default:
			end, w = clusterEnd(line, col)
			if end > col+1 {
				comb = line[col+1 : end]
			}
		}

		screenX := x0 + (disp - rowBase) - left
		if disp-rowBase >= left && disp-rowBase < left+textW {
			if ch == '\t' {
				for i := 0; i < w && screenX+i < e.width; i++ {
					e.drawCell(screenX+i, y, ' ', style)
				}
			} else {
				e.drawCellComb(screenX, y, ch, comb, style)
				for i := 1; i < w && screenX+i < e.width; i++ {
					e.drawCell(screenX+i, y, ' ', style)
				}
			}
		} else if disp-rowBase >= left+textW {
			break // everything further right is off-screen
		}
		disp += w
		col = end
	}
}

func (e *Editor) drawCursor(rows []row, gutter, textW int) {
	t := e.active()
	e.screen.HideCursor()
	if t == nil {
		return
	}
	ri := cursorRow(rows, t.cur)
	if ri < 0 {
		return
	}
	r := rows[ri]
	line := t.line(t.cur.Line)
	tabW := t.tabW()
	left := t.left
	if t.config().Wrap {
		left = 0
	}
	base := 0
	if r.start > 0 {
		base = displayCol(line, r.start, tabW)
	}
	disp := displayCol(line, t.cur.Col, tabW) - base
	x := e.editorLeft() + gutter + disp - left
	y := e.mainTop() + ri
	if disp < left || disp >= left+textW {
		return
	}
	if x >= e.editorLeft() && x < e.width && y >= e.mainTop() && y < e.mainTop()+e.mainHeight() {
		e.drawBlockCursor(line, t.cur.Col, x, y, tabW)
	}
}

// drawBlockCursor renders a solid block at the cursor cell using an explicit
// high-contrast background, so the cursor stays visible regardless of the
// terminal's default colors or reverse-video support.
func (e *Editor) drawBlockCursor(line []rune, col, x, y, tabW int) {
	ch, w := cursorCell(line, col, tabW)
	var comb []rune
	if col >= 0 && col < len(line) && ch != '\t' {
		if end, _ := clusterEnd(line, col); end > col+1 {
			comb = line[col+1 : end]
		}
	}
	if isRawByte(ch) {
		ch, comb = '·', nil
	}
	if ch != '\t' {
		e.drawCellComb(x, y, ch, comb, blockCursorStyle)
	}
	for i := 1; i < w && x+i < e.width; i++ {
		e.drawCell(x+i, y, ' ', blockCursorStyle)
	}
}

// blockCursorStyle is black text on a bright yellow block.
var blockCursorStyle = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.NewHexColor(0xE5C07B))

// cursorCell returns the character to render at rune column col (a space when
// the cursor is past the end of the line) and its display width.
func cursorCell(line []rune, col, tabW int) (rune, int) {
	if col < 0 || col >= len(line) {
		return ' ', 1
	}
	r := line[col]
	if r == '\t' {
		return r, tabW - displayCol(line, col, tabW)%tabW
	}
	_, w := clusterEnd(line, col)
	return r, w
}

// selection returns the normalized selection [sLine,sCol,eLine,eCol], or [-1,...].
func (e *Editor) selection() [4]int {
	t := e.active()
	if t == nil {
		return [4]int{-1, 0, 0, 0}
	}
	return t.selRange()
}

// inMatch reports whether a rune column falls inside a search hit.
func inMatch(matches []foundMatch, col int) bool {
	for _, m := range matches {
		if col >= m.col && col < m.end {
			return true
		}
		if m.col > col {
			break // matches are ordered
		}
	}
	return false
}

func inSelection(lineIdx, col int, sel [4]int) bool {
	if lineIdx < sel[0] || lineIdx > sel[2] {
		return false
	}
	if lineIdx == sel[0] && lineIdx == sel[2] {
		return col >= sel[1] && col < sel[3]
	}
	if lineIdx == sel[0] {
		return col >= sel[1]
	}
	if lineIdx == sel[2] {
		return col < sel[3]
	}
	return true
}

// ensureVisible scrolls so the cursor is inside the viewport. It follows the
// cursor when the cursor moved or the viewport was resized; plain (mouse
// wheel) scrolling with a still cursor is respected and not overridden.
func (e *Editor) ensureVisible(width, height int) {
	t := e.active()
	if t == nil {
		return
	}
	// A resize can leave the cursor off-screen, so follow it again; a plain
	// wheel scroll with a still cursor must not be overridden.
	resized := (t.lastW != 0 || t.lastH != 0) && (t.lastW != width || t.lastH != height)
	t.lastW, t.lastH = width, height
	if t.cur == t.lastScroll && !resized {
		return
	}
	wrap := t.config().Wrap
	if wrap {
		t.left = 0
		e.ensureVisibleWrapped(t, width, height)
	} else {
		if t.cur.Line < t.top {
			t.top = t.cur.Line
			t.topSub = 0
		}
		if t.cur.Line >= t.top+height {
			t.top = t.cur.Line - height + 1
			t.topSub = 0
		}
		disp := displayCol(t.line(t.cur.Line), t.cur.Col, t.tabW())
		if disp < t.left {
			t.left = disp
		}
		if disp >= t.left+width {
			t.left = disp - width + 1
		}
		if t.left < 0 {
			t.left = 0
		}
	}
	if t.top < 0 {
		t.top = 0
	}
	t.lastScroll = t.cur
}

// ensureVisibleWrapped scrolls in display-row space so a wrapped cursor line
// stays on screen.
func (e *Editor) ensureVisibleWrapped(t *Tab, width, height int) {
	if t.cur.Line < t.top {
		t.top, t.topSub = t.cur.Line, 0
	}
	// Count display rows from the scroll position to the cursor.
	count := 0
	found := false
	for li := t.top; li <= t.cur.Line && li < t.lineCount(); li++ {
		rs := e.rowsForLine(t, li, width)
		start := 0
		if li == t.top {
			start = t.topSub
			if start >= len(rs) {
				start = len(rs) - 1
			}
		}
		for i := start; i < len(rs); i++ {
			if li == t.cur.Line && t.cur.Col >= rs[i].start && (t.cur.Col < rs[i].end || i == len(rs)-1) {
				found = true
				break
			}
			count++
		}
		if found {
			break
		}
	}
	if !found {
		t.top, t.topSub = t.cur.Line, 0
		return
	}
	for count >= height {
		// Scroll down one display row.
		rs := e.rowsForLine(t, t.top, width)
		if t.topSub+1 < len(rs) {
			t.topSub++
		} else {
			t.top++
			t.topSub = 0
		}
		count--
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [24]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// drawTextRowBench renders a row without a screen attached, for benchmarks.
func (e *Editor) drawTextRowBench(t *Tab, r row, y, x0, textW int) {
	e.drawTextRow(t, r, y, x0, textW, [4]int{-1, 0, 0, 0})
}
