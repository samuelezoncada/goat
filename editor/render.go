package editor

import (
	"github.com/gdamore/tcell/v2"
)

func (e *Editor) allocFrame() {
	if e.frame == nil || len(e.frame) != e.height {
		f := make([][]frameCell, e.height)
		for y := range f {
			f[y] = make([]frameCell, e.width)
		}
		e.frame = f
	}
	for y := range e.frame {
		for x := range e.frame[y] {
			e.frame[y][x] = frameCell{}
		}
	}
}

func (e *Editor) drawCell(x, y int, r rune, style tcell.Style) {
	if y < 0 || y >= len(e.frame) || x < 0 || x >= len(e.frame[y]) {
		return
	}
	c := &e.frame[y][x]
	if c.r == r && c.s == style {
		return
	}
	c.r = r
	c.s = style
	e.screen.SetContent(x, y, r, nil, style)
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
		if x >= 0 {
			e.drawCell(x, y, r, style)
		}
		x += runeWidth(r)
	}
	return x
}

// --- main draw -----------------------------------------------------------

func (e *Editor) draw() {
	// ensure the active tab's highlighter wakes the render loop when ready
	if t := e.active(); t != nil && t.hl != nil {
		t.hl.SetOnReady(func() { e.screen.PostEvent(tcell.NewEventInterrupt(nil)) })
	}
	if e.frame == nil || len(e.frame) != e.height {
		e.allocFrame()
		e.screen.Clear()
	}
	if e.mode == ModeHelp {
		e.drawHelp()
		e.screen.Show()
		return
	}
	if e.mode == ModePicker {
		e.drawTabBar()
		e.drawPicker()
		e.screen.Show()
		return
	}
	if e.mode == ModeResults {
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

	for row := 0; row < e.mainHeight(); row++ {
		y := e.mainTop() + row
		lineIdx := t.top + row
		e.fillRow(e.editorLeft(), e.width, y, e.theme.Plain)
		if lineIdx >= t.lineCount() {
			continue
		}
		e.drawGutter(e.editorLeft(), y, lineIdx, gutter)
		e.drawTextLine(t, lineIdx, y, e.editorLeft()+gutter, textW, sel)
	}

	e.drawCursor()
}

func (e *Editor) drawGutter(x0, y, lineIdx, width int) {
	style := e.theme.Comment
	if t := e.active(); t != nil && lineIdx == t.cur.Line {
		style = e.theme.Plain.Reverse(true)
	}
	e.fillRow(x0, x0+width, y, style)
	s := itoa(lineIdx + 1)
	e.putStr(x0+width-1-len([]rune(s)), y, s, style)
}

func (e *Editor) drawTextLine(t *Tab, lineIdx, y, x0, textW int, sel [4]int) {
	line := t.line(lineIdx)
	spans := t.hl.Spans(lineIdx, line)

	si := 0
	colRune := 0
	disp := 0
	plain := e.theme.Plain
	for colRune < len(line) {
		for si < len(spans) && colRune >= spans[si].Start+spans[si].Len {
			si++
		}
		style := plain
		if si < len(spans) && colRune >= spans[si].Start && colRune < spans[si].Start+spans[si].Len {
			style = e.theme.Style(spans[si].Type)
		}
		if sel[0] >= 0 && inSelection(lineIdx, colRune, sel) {
			style = style.Reverse(true)
		}
		r := line[colRune]
		w := runeWidth(r)
		if r == '\t' {
			w = tabStop - disp%tabStop
		}
		if disp >= t.left && disp < t.left+textW {
			x := x0 + disp - t.left
			if r == '\t' {
				for i := 0; i < w && x+i < e.width; i++ {
					e.drawCell(x+i, y, ' ', style)
				}
			} else {
				e.drawCell(x, y, r, style)
				for i := 1; i < w && x+i < e.width; i++ {
					e.drawCell(x+i, y, ' ', style)
				}
			}
		}
		disp += w
		colRune++
	}
}

func (e *Editor) drawCursor() {
	t := e.active()
	if t == nil {
		e.screen.HideCursor()
		return
	}
	line := t.line(t.cur.Line)
	disp := displayCol(line, t.cur.Col)
	x := e.editorLeft() + e.gutterWidth() + disp - t.left
	y := e.mainTop() + t.cur.Line - t.top
	e.screen.HideCursor()
	if x >= e.editorLeft() && x < e.width && y >= e.mainTop() && y < e.mainTop()+e.mainHeight() {
		e.drawBlockCursor(line, t.cur.Col, x, y)
	}
}

// drawBlockCursor renders a solid block at the cursor cell using an explicit
// high-contrast background, so the cursor stays visible regardless of the
// terminal's default colors or reverse-video support.
func (e *Editor) drawBlockCursor(line []rune, col, x, y int) {
	ch, w := cursorCell(line, col)
	if ch != '\t' {
		e.drawCell(x, y, ch, blockCursorStyle)
	}
	for i := 1; i < w && x+i < e.width; i++ {
		e.drawCell(x+i, y, ' ', blockCursorStyle)
	}
}

// blockCursorStyle is black text on a bright yellow block.
var blockCursorStyle = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.NewHexColor(0xE5C07B))

// cursorCell returns the character to render at rune column col (a space when
// the cursor is past the end of the line) and its display width.
func cursorCell(line []rune, col int) (rune, int) {
	if col < 0 || col >= len(line) {
		return ' ', 1
	}
	r := line[col]
	w := runeWidth(r)
	if r == '\t' {
		w = tabStop - displayCol(line, col)%tabStop
	}
	return r, w
}

// selection returns the normalized selection [sLine,sCol,eLine,eCol], or [-1,...].
func (e *Editor) selection() [4]int {
	t := e.active()
	if t == nil || t.mark == nil {
		return [4]int{-1, 0, 0, 0}
	}
	a, b := t.mark, &t.cur
	if b.Line < a.Line || (b.Line == a.Line && b.Col < a.Col) {
		a, b = b, a
	}
	return [4]int{a.Line, a.Col, b.Line, b.Col}
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

// ensureVisible scrolls so the cursor is inside the viewport. It only follows
// the cursor when the cursor has moved; manual (mouse wheel) scrolling is
// respected and not overridden.
func (e *Editor) ensureVisible(width, height int) {
	t := e.active()
	if t == nil {
		return
	}
	if t.cur == t.lastScroll {
		return
	}
	if t.cur.Line < t.top {
		t.top = t.cur.Line
	}
	if t.cur.Line >= t.top+height {
		t.top = t.cur.Line - height + 1
	}
	disp := displayCol(t.line(t.cur.Line), t.cur.Col)
	if disp < t.left {
		t.left = disp
	}
	if disp >= t.left+width {
		t.left = disp - width + 1
	}
	if t.left < 0 {
		t.left = 0
	}
	t.lastScroll = t.cur
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
