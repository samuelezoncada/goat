package editor

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// handle routes terminal events. Every event type is dispatched through the
// current mode, so an overlay (help, picker, results, prompt) is never
// bypassed by a mouse click or a paste landing in the buffer behind it.
func (e *Editor) handle(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		e.width, e.height = ev.Size()
		e.frame = nil
		e.screen.Sync()
	case *tcell.EventKey:
		switch e.mode {
		case ModeHelp:
			e.helpKey(ev)
		case ModePrompt:
			e.promptKey(ev)
		case ModePicker:
			e.pickerKey(ev)
		case ModeResults:
			e.resultsKey(ev)
		default:
			e.handleNormalKey(ev)
		}
	case *tcell.EventMouse:
		switch e.mode {
		case ModeHelp:
			e.helpMouse(ev)
		case ModePicker:
			e.pickerMouse(ev)
		case ModeResults:
			e.resultsMouse(ev)
		case ModePrompt:
			// A click cannot meaningfully act on the buffer while a prompt is
			// waiting for an answer; ignore it rather than moving the hidden
			// cursor or clobbering the pending prompt.
		default:
			e.handleMouse(ev)
		}
	case *tcell.EventPaste:
		e.handlePasteEvent(ev)
	case *tcell.EventClipboard:
		e.insertClipboard(ev.Data())
	case *tcell.EventError:
		e.errorf("terminal error: %v", ev.Error())
	case *tcell.EventInterrupt:
		// highlight-ready wake (nil payload), async symbol results, a finished
		// index build, or a terminating signal
		switch d := ev.Data().(type) {
		case symbolEvent:
			e.onSymbolEvent(d)
		case buildDone:
			e.onBuildDone(d)
		case indexDone:
			e.onIndexDone(d)
		case signalEvent:
			e.onSignal(d)
		}
	}
}

// onSignal saves modified buffers and exits when the process is asked to
// terminate, mirroring nano's emergency .save files.
func (e *Editor) onSignal(d signalEvent) {
	saved := e.emergencySave()
	if len(saved) > 0 {
		e.statusf("Received %v; wrote %d buffer(s) to .save files", d.sig, len(saved))
	}
	e.quit()
}

// handlePasteEvent brackets a terminal paste. The pasted text arrives as
// ordinary key events between the start and end markers.
func (e *Editor) handlePasteEvent(ev *tcell.EventPaste) {
	if ev.Start() {
		e.pasteActive = true
		e.pasteBuf = nil
		return
	}
	e.pasteActive = false
	buf := e.pasteBuf
	e.pasteBuf = nil
	if len(buf) == 0 {
		return
	}
	switch e.mode {
	case ModePrompt:
		e.promptInsert(buf)
	case ModePicker:
		e.pickerInsert(buf)
	case ModeNormal:
		if e.focus == FocusBrowser {
			return
		}
		if t := e.active(); t != nil {
			if t.readOnly {
				e.statusf("Buffer is read-only")
				return
			}
			t.insertRunes(t.cur.Line, t.cur.Col, buf)
		}
	}
}

// pasteKey accumulates one key event belonging to a paste. Newlines and tabs
// arrive as key events too, so they must be translated back into text instead
// of being dropped.
func (e *Editor) pasteKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyRune:
		e.pasteBuf = append(e.pasteBuf, ev.Rune())
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		// KeyEnter is CR, KeyCtrlJ is LF; both mean "line break" in a paste.
		e.pasteBuf = append(e.pasteBuf, '\n')
	case tcell.KeyTab:
		e.pasteBuf = append(e.pasteBuf, '\t')
	}
	if len(e.pasteBuf) > maxOpenBytes {
		e.pasteBuf = e.pasteBuf[:maxOpenBytes]
	}
}

// insertClipboard handles clipboard data returned by the terminal.
func (e *Editor) insertClipboard(data []byte) {
	if len(data) == 0 {
		return
	}
	rs := decodeRunes(data)
	e.clip = rs
	if e.mode == ModeNormal && e.focus == FocusText {
		if t := e.active(); t != nil && !t.readOnly {
			t.insertRunes(t.cur.Line, t.cur.Col, rs)
		}
	}
}

// setClipboard records a cut/copy and, when enabled, hands it to the terminal
// so it lands in the system clipboard (OSC 52).
func (e *Editor) setClipboard(rs []rune) {
	e.clip = rs
	if !e.config().Clipboard || e.screen == nil || len(rs) == 0 {
		return
	}
	e.screen.SetClipboard(appendEncoded(nil, rs))
}

func (e *Editor) handleNormalKey(ev *tcell.EventKey) {
	if e.pasteActive {
		e.pasteKey(ev)
		return
	}
	if ev.Key() == tcell.KeyCtrlB {
		e.browser.toggle()
		return
	}
	t := e.active()
	if t == nil {
		return
	}

	mod := ev.Modifiers()
	ctrl := mod&tcell.ModCtrl != 0
	shift := mod&tcell.ModShift != 0
	alt := mod&tcell.ModAlt != 0

	// A status message belongs to the action that produced it: the next key
	// clears it, so the filename and position come back into view. Handlers
	// below are free to set a new one.
	e.clearMsg()

	if e.focus == FocusBrowser && e.browser.open {
		e.browserKey(ev)
		return
	}

	// Editing keys are refused on a read-only buffer; movement still works.
	switch ev.Key() {
	case tcell.KeyCtrlQ:
		e.exit()
	case tcell.KeyCtrlO, tcell.KeyCtrlS:
		e.save()
	case tcell.KeyCtrlR:
		e.readFile()
	case tcell.KeyCtrlG:
		e.openHelp()
	case tcell.KeyCtrlW:
		e.beginSearch()
	case tcell.KeyCtrlBackslash:
		e.beginReplace()
	case tcell.KeyCtrlK, tcell.KeyCtrlX:
		e.cut()
	case tcell.KeyCtrlU, tcell.KeyCtrlV:
		e.uncut()
	case tcell.KeyCtrlC:
		e.copySelection()
	case tcell.KeyCtrlL:
		e.redrawScreen()
	case tcell.KeyCtrlJ:
		e.justify()
	case tcell.KeyCtrlA:
		t.home()
	case tcell.KeyCtrlE:
		t.end()
	case tcell.KeyCtrlY:
		e.editAction(t.redo)
	case tcell.KeyCtrlZ:
		e.editAction(t.undo)
	case tcell.KeyCtrlN:
		e.selectMove(t.moveDown, shift)
	case tcell.KeyCtrlP:
		if alt {
			e.openPicker()
			return
		}
		e.selectMove(t.moveUp, shift)
	case tcell.KeyCtrlF:
		e.selectMove(t.moveRight, shift)
	case tcell.KeyCtrlD:
		e.editAction(t.deleteForward)
	case tcell.KeyCtrlH:
		e.editAction(t.backspace)
	case tcell.KeyEnter:
		e.editAction(t.insertNewline)
	case tcell.KeyUp:
		if alt {
			e.scrollUp()
		} else {
			e.selectMove(t.moveUp, shift)
		}
	case tcell.KeyDown:
		if alt {
			e.scrollDown()
		} else {
			e.selectMove(t.moveDown, shift)
		}
	case tcell.KeyLeft:
		if alt || ctrl {
			e.selectMove(t.wordLeft, shift)
		} else {
			e.selectMove(t.moveLeft, shift)
		}
	case tcell.KeyRight:
		if alt || ctrl {
			e.selectMove(t.wordRight, shift)
		} else {
			e.selectMove(t.moveRight, shift)
		}
	case tcell.KeyPgUp:
		e.selectMove(func() { t.pgup(e.mainHeight() - 1) }, shift)
	case tcell.KeyPgDn:
		e.selectMove(func() { t.pgdn(e.mainHeight() - 1) }, shift)
	case tcell.KeyHome:
		e.selectMove(t.home, shift)
	case tcell.KeyEnd:
		e.selectMove(t.end, shift)
	case tcell.KeyDelete:
		e.editAction(t.deleteForward)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		e.editAction(t.backspace)
	case tcell.KeyTab:
		switch {
		case alt:
			e.cycleFocus()
		case ctrl:
			e.switchTab(1)
		default:
			e.editAction(t.insertTab)
		}
	case tcell.KeyBacktab:
		if ctrl {
			e.switchTab(-1)
			return
		}
		e.editAction(t.dedent)
	case tcell.KeyEsc:
		e.clearMsg()
		e.search.highlight = false
		t.mark = nil
	case tcell.KeyRune:
		if ctrl {
			return
		}
		if alt {
			e.altRune(ev.Rune())
			return
		}
		if ev.Rune() >= 0x20 && ev.Rune() != 0x7f {
			e.editAction(func() { t.insertRune(ev.Rune()) })
		}
	}
}

// editAction runs a mutating action unless the buffer is read-only.
func (e *Editor) editAction(fn func()) {
	t := e.active()
	if t == nil {
		return
	}
	if t.readOnly {
		e.statusf("%s is read-only (^O to write elsewhere)", t.name)
		return
	}
	fn()
}

// selectMove wraps a movement so Shift extends the selection.
func (e *Editor) selectMove(fn func(), shift bool) {
	t := e.active()
	if t == nil {
		return
	}
	if shift {
		t.selectMove(fn)
		return
	}
	t.mark = nil
	fn()
}

// altRune handles Meta+<key> extras.
func (e *Editor) altRune(r rune) {
	if r >= '1' && r <= '9' {
		e.gotoTab(int(r - '1'))
		return
	}
	switch r {
	case 's':
		e.browser.toggle()
	case 'd':
		e.gotoSymbol()
	case 'b':
		e.jumpBack()
	case 't':
		e.newTab()
	case 'w':
		e.closeCurrentTab()
	case 'a':
		e.selectAll()
	case 'p':
		e.openPicker()
	case ' ':
		if t := e.active(); t != nil {
			t.toggleMark()
		}
	case 'g':
		e.gotoLine()
	case 'z':
		e.editAction(func() { e.active().undo() })
	case 'y':
		e.editAction(func() { e.active().redo() })
	case 'c':
		e.search.caseSens = !e.search.caseSens
		e.statusf("Case sensitivity: %v", e.search.caseSens)
	case 'r':
		e.search.regex = !e.search.regex
		e.statusf("Regular expression: %v", e.search.regex)
	case 'u':
		e.search.wholeWord = !e.search.wholeWord
		e.statusf("Whole word: %v", e.search.wholeWord)
	case 'q':
		e.exit()
	}
}

// browserKey handles keys while the file browser has focus.
func (e *Editor) browserKey(ev *tcell.EventKey) {
	b := e.browser
	switch ev.Key() {
	case tcell.KeyCtrlQ:
		e.exit()
	case tcell.KeyCtrlG:
		e.openHelp()
	case tcell.KeyEsc:
		b.toggle()
	case tcell.KeyCtrlB:
		b.toggle()
	case tcell.KeyCtrlH:
		b.toggleHidden()
	case tcell.KeyCtrlP:
		e.openPicker()
	case tcell.KeyUp:
		b.moveUp()
	case tcell.KeyDown:
		b.moveDown()
	case tcell.KeyHome:
		b.home()
	case tcell.KeyEnd:
		b.end()
	case tcell.KeyPgUp:
		for i := 0; i < e.mainHeight()-1 && b.sel > 0; i++ {
			b.moveUp()
		}
	case tcell.KeyPgDn:
		for i := 0; i < e.mainHeight()-1 && b.sel < len(b.entries)-1; i++ {
			b.moveDown()
		}
	case tcell.KeyEnter:
		b.enter()
	case tcell.KeyRight:
		b.expand()
	case tcell.KeyLeft:
		b.collapseOrUp()
	case tcell.KeyTab:
		e.focusText()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		b.collapseOrUp()
	case tcell.KeyDelete:
		b.remove()
	case tcell.KeyRune:
		if ev.Modifiers()&tcell.ModAlt != 0 {
			switch ev.Rune() {
			case 's':
				b.toggle()
			case 'q':
				e.exit()
			case 'n':
				b.newFile()
			case 'd':
				b.newDir()
			case 'r':
				b.rename()
			case 'x':
				b.remove()
			case 'p':
				e.openPicker()
			}
			return
		}
		switch ev.Rune() {
		case '+', 'l', 'L':
			b.expand()
		case '-', 'h', 'H':
			b.collapseOrUp()
		default:
			b.jumpToPrefix(ev.Rune())
		}
	}
}

// --- mouse ---------------------------------------------------------------

func (e *Editor) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	btn := ev.Buttons()
	if btn&tcell.WheelUp != 0 {
		if e.focus == FocusBrowser && e.browser.open {
			e.browser.scroll(-3)
			return
		}
		e.scrollBy(-3)
		return
	}
	if btn&tcell.WheelDown != 0 {
		if e.focus == FocusBrowser && e.browser.open {
			e.browser.scroll(3)
			return
		}
		e.scrollBy(3)
		return
	}
	if btn&tcell.Button1 != 0 {
		if y < e.mainTop() && e.handleTabBarClick(x) {
			return
		}
		if e.browser != nil && e.browser.open && y >= e.mainTop() {
			e.handleBrowserClick(y)
			return
		}
		if e.mouseDown {
			e.handleTextMouseDrag(x, y)
		} else {
			e.handleTextMousePress(x, y, ev.Modifiers()&tcell.ModShift != 0)
		}
		return
	}
	if btn&tcell.Button3 != 0 {
		// middle click: paste at the click position, like an X11 terminal
		e.handleTextMousePress(x, y, false)
		e.mouseDown = false
		e.uncut()
		return
	}
	if btn == tcell.ButtonNone && e.mouseDown {
		// left-button release (ButtonNone is also used for hover, which is
		// only meaningful while nothing is held)
		if !e.mouseDrag {
			e.activeMarkClear()
		}
		e.mouseDown = false
		e.mouseDrag = false
	}
}

// helpMouse scrolls the help page with the wheel and closes it on a click.
func (e *Editor) helpMouse(ev *tcell.EventMouse) {
	switch {
	case ev.Buttons()&tcell.WheelUp != 0:
		e.helpTop -= 3
		if e.helpTop < 0 {
			e.helpTop = 0
		}
	case ev.Buttons()&tcell.WheelDown != 0:
		e.helpTop += 3
	case ev.Buttons()&tcell.Button1 != 0:
		e.mode = ModeNormal
		e.helpTop = 0
	}
}

// pickerMouse selects the clicked result; a second click opens it.
func (e *Editor) pickerMouse(ev *tcell.EventMouse) {
	p := e.picker
	if p == nil {
		return
	}
	_, y := ev.Position()
	switch {
	case ev.Buttons()&tcell.WheelUp != 0:
		if p.sel > 0 {
			p.sel--
		}
	case ev.Buttons()&tcell.WheelDown != 0:
		if p.sel+1 < len(p.matches) {
			p.sel++
		}
	case ev.Buttons()&tcell.Button1 != 0:
		idx := p.top + (y - e.mainTop() - 1)
		if idx < 0 || idx >= len(p.matches) {
			return
		}
		if idx == p.sel {
			e.pickerOpen()
			return
		}
		p.sel = idx
	}
}

// resultsMouse selects the clicked location; a second click jumps to it.
func (e *Editor) resultsMouse(ev *tcell.EventMouse) {
	r := e.results
	if r == nil {
		return
	}
	_, y := ev.Position()
	switch {
	case ev.Buttons()&tcell.WheelUp != 0:
		if r.sel > 0 {
			r.sel--
		}
	case ev.Buttons()&tcell.WheelDown != 0:
		if r.sel+1 < len(r.locs) {
			r.sel++
		}
	case ev.Buttons()&tcell.Button1 != 0:
		idx := r.top + (y - e.mainTop() - 1)
		if idx < 0 || idx >= len(r.locs) {
			return
		}
		if idx == r.sel {
			e.openResult()
			return
		}
		r.sel = idx
	}
}

// activeMarkClear clears the current tab's selection without moving the cursor.
func (e *Editor) activeMarkClear() {
	if t := e.active(); t != nil {
		t.mark = nil
	}
}

// scrollBy scrolls the text viewport by n display rows without moving the
// cursor, clamped to the buffer.
func (e *Editor) scrollBy(n int) {
	t := e.active()
	if t == nil {
		return
	}
	for ; n > 0; n-- {
		if !t.scrollDownOne(e) {
			break
		}
	}
	for ; n < 0; n++ {
		if !t.scrollUpOne(e) {
			break
		}
	}
	t.lastScroll = t.cur
}

// scrollDownOne advances the viewport by one display row. Returns false at the
// end of the buffer.
func (t *Tab) scrollDownOne(e *Editor) bool {
	if t.config().Wrap {
		rs := e.rowsForLine(t, t.top, e.wrapWidth(t))
		if t.topSub+1 < len(rs) {
			t.topSub++
			return true
		}
		if t.top+1 < t.lineCount() {
			t.top++
			t.topSub = 0
			return true
		}
		return false
	}
	if t.top+1 < t.lineCount() {
		t.top++
		return true
	}
	return false
}

func (t *Tab) scrollUpOne(e *Editor) bool {
	if t.config().Wrap {
		if t.topSub > 0 {
			t.topSub--
			return true
		}
		if t.top > 0 {
			t.top--
			t.topSub = len(e.rowsForLine(t, t.top, e.wrapWidth(t))) - 1
			return true
		}
		return false
	}
	if t.top > 0 {
		t.top--
		return true
	}
	return false
}

// wrapWidth returns the text width used for soft wrapping.
func (e *Editor) wrapWidth(t *Tab) int {
	w := e.editorWidth() - e.gutterWidth()
	if w < 1 {
		w = 1
	}
	return w
}

func (e *Editor) scrollUp()   { e.scrollBy(-1) }
func (e *Editor) scrollDown() { e.scrollBy(1) }

// handleTextMousePress handles a left-button press in the text area: a plain
// click moves the cursor, shift+click extends the selection, a double-click
// selects the word, and a held button starts a drag selection.
func (e *Editor) handleTextMousePress(x, y int, shift bool) {
	e.focusText()
	t := e.active()
	if t == nil {
		return
	}
	if shift {
		if t.mark == nil {
			m := t.cur
			t.mark = &m
		}
		e.clickToCursor(x, y)
		e.mouseDown = true
		e.mouseDrag = true
		return
	}
	e.clickToCursor(x, y)
	e.mouseAnchor = t.cur

	now := time.Now().UnixMilli()
	if now-e.mouseLast < 400 && e.mouseLine == t.cur.Line {
		// double-click: select the word under the cursor
		start, end := wordRange(t.line(t.cur.Line), t.cur.Col)
		t.mark = &Pos{Line: t.cur.Line, Col: start}
		t.cur.Col = end
		t.destCol = end
		e.mouseLast = 0 // don't treat the next click as a triple-click
		e.mouseDown = false
		return
	}
	e.mouseLast = now
	e.mouseLine = t.cur.Line
	e.mouseDown = true
	e.mouseDrag = false
	if t.mark == nil {
		m := t.cur
		t.mark = &m
	}
}

// handleTextMouseDrag extends the selection while the left button is held.
func (e *Editor) handleTextMouseDrag(x, y int) {
	e.clickToCursor(x, y)
	e.mouseDrag = true
}

// handleBrowserClick selects a browser row on click; a double-click opens or
// expands the selected entry.
func (e *Editor) handleBrowserClick(y int) {
	b := e.browser
	idx := b.top + (y - e.mainTop() - 1)
	if idx < 0 || idx >= len(b.entries) {
		e.focus = FocusBrowser
		return
	}
	e.focus = FocusBrowser
	now := time.Now().UnixMilli()
	if idx == b.lastClickRow && now-b.lastClick < 400 {
		b.sel = idx
		b.lastClick = 0
		b.enter()
		return
	}
	b.sel = idx
	b.lastClick = now
	b.lastClickRow = idx
}

// clickToCursor places the cursor at a clicked cell, using the display rows of
// the last frame so soft-wrapped lines map correctly.
func (e *Editor) clickToCursor(x, y int) {
	t := e.active()
	if t == nil {
		return
	}
	ri := y - e.mainTop()
	gutter := e.rowsGutter
	if gutter == 0 {
		gutter = e.gutterWidth()
	}
	if len(e.rows) == 0 {
		// No frame drawn yet: fall back to one row per line.
		line := t.top + ri
		if line < 0 || line >= t.lineCount() {
			return
		}
		disp := x - (e.editorLeft() + gutter) + t.left
		t.cur = Pos{line, colFromDisp(t.line(line), disp, t.tabW())}
		t.destCol = t.cur.Col
		return
	}
	if ri < 0 {
		ri = 0
	}
	if ri >= len(e.rows) {
		ri = len(e.rows) - 1
	}
	r := e.rows[ri]
	line := t.line(r.line)
	left := t.left
	if t.config().Wrap {
		left = 0
	}
	base := 0
	if r.start > 0 {
		base = displayCol(line, r.start, t.tabW())
	}
	disp := x - (e.editorLeft() + gutter) + left + base
	if disp < 0 {
		disp = 0
	}
	col := colFromDisp(line, disp, t.tabW())
	if col > r.end {
		col = r.end
	}
	if col < r.start {
		col = r.start
	}
	t.cur = Pos{r.line, col}
	t.destCol = t.cur.Col
}

// colFromDisp maps a display column to a rune column.
func colFromDisp(line []rune, disp, tabW int) int {
	if tabW < 1 {
		tabW = 8
	}
	c := 0
	d := 0
	for c < len(line) {
		var w, end int
		if line[c] == '\t' {
			w = tabW - d%tabW
			end = c + 1
		} else {
			end, w = clusterEnd(line, c)
		}
		if d+w > disp {
			break
		}
		d += w
		c = end
	}
	return c
}

// --- actions -------------------------------------------------------------

func (e *Editor) save() {
	t := e.active()
	if t == nil {
		return
	}
	if t.path == "" {
		e.beginPrompt("File Name to Write: ", t.name, nil, func(name string) {
			if name == "" {
				return
			}
			e.writeTab(t, name)
		})
		return
	}
	e.writeTabChecked(t, t.path, nil)
}

// writeTabChecked warns when the file changed on disk since it was read, so a
// save cannot silently discard someone else's edit.
func (e *Editor) writeTabChecked(t *Tab, path string, done func(bool)) {
	if samePath(path, t.path) && t.externallyChanged() {
		e.beginPrompt(sprintf("%s changed on disk. Overwrite? [y/N] ", t.name), "", nil, func(ans string) {
			switch ans {
			case "y", "Y":
				ok := e.writeTab(t, path)
				if done != nil {
					done(ok)
				}
			default:
				e.statusf("Not saved; use ^R to read the file or ^O to write elsewhere")
				if done != nil {
					done(false)
				}
			}
		})
		return
	}
	ok := e.writeTab(t, path)
	if done != nil {
		done(ok)
	}
}

// writeTab saves and reports the outcome.
func (e *Editor) writeTab(t *Tab, path string) bool {
	if err := t.saveTo(path); err != nil {
		e.errorf("Error writing %s: %v", path, err)
		return false
	}
	if e.symProvider != nil {
		e.updateSymbolFile(t.path)
	}
	e.InvalidateIndex()
	n := t.lineCount()
	extra := ""
	if t.rawBytes {
		extra = " (contains non-UTF-8 bytes, preserved)"
	}
	e.statusf("Wrote %d line%s%s", n, plural(n), extra)
	return true
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (e *Editor) readFile() {
	t := e.active()
	if t == nil {
		return
	}
	e.beginPrompt("File to read: ", "", nil, func(path string) {
		if path == "" {
			return
		}
		if fi, err := os.Stat(path); err == nil && fi.Size() > maxOpenBytes {
			e.errorf("Error reading %s: file is too large", path)
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			e.errorf("Error reading %s: %v", path, err)
			return
		}
		if bytesHasNUL(data) {
			e.errorf("Error reading %s: %v", path, errBinary)
			return
		}
		rs := decodeRunes(data)
		if hasRawRunes(rs) {
			t.rawBytes = true
		}
		e.editAction(func() { t.insertRunes(t.cur.Line, t.cur.Col, rs) })
		e.statusf("Read %d bytes", len(data))
	})
}

func bytesHasNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

func hasRawRunes(rs []rune) bool {
	for _, r := range rs {
		if isRawByte(r) {
			return true
		}
	}
	return false
}

func (e *Editor) redrawScreen() {
	e.frame = nil
	e.screen.Sync()
}

// cycleFocus moves keyboard focus between text and the browser.
func (e *Editor) cycleFocus() {
	if !e.browser.open {
		e.browser.open = true
	}
	if e.focus == FocusText {
		e.focus = FocusBrowser
		e.browser.rebuild()
	} else {
		e.focusText()
	}
}

func (e *Editor) selectAll() {
	t := e.active()
	if t == nil {
		return
	}
	t.mark = &Pos{0, 0}
	t.cur.Line = t.lineCount() - 1
	t.cur.Col = len(t.line(t.cur.Line))
	t.destCol = t.cur.Col
}

func (e *Editor) gotoLine() {
	e.beginPrompt("Goto Line: ", "", nil, func(s string) {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 {
			e.errorf("Invalid line number")
			return
		}
		t := e.active()
		if t == nil {
			return
		}
		e.pushJump()
		n--
		if n >= t.lineCount() {
			n = t.lineCount() - 1
		}
		t.mark = nil
		t.cur.Line = n
		t.cur.Col = 0
		t.destCol = 0
	})
}

// --- cut / uncut ---------------------------------------------------------

func (e *Editor) cut() {
	t := e.active()
	if t == nil {
		return
	}
	if t.readOnly {
		e.copySelection()
		return
	}
	if t.mark != nil {
		// deleteSelection clears the mark and is a no-op for a zero-width
		// selection, so a stray mark never turns ^K into a line cut.
		removed := t.deleteSelection()
		if len(removed) > 0 {
			e.setClipboard(removed)
		}
		return
	}
	line := t.cur.Line
	l := t.line(line)
	n := len(l)
	if line+1 < t.lineCount() {
		n++ // take the line's newline too
	}
	if n == 0 {
		return // nothing to cut; keep the clipboard intact
	}
	removed := deleteText(t.text, line, 0, n)
	if len(removed) == 0 {
		return
	}
	o := &op{kind: opDelete, line: line, col: 0, text: removed, curBefore: t.cur, curAfter: Pos{line, 0}}
	t.cur = Pos{line, 0}
	t.destCol = 0
	t.edits.push(o)
	t.setDirty()
	t.invalidate(line)
	t.clampCursor()
	e.setClipboard(removed)
}

func (e *Editor) uncut() {
	t := e.active()
	if t == nil || len(e.clip) == 0 {
		return
	}
	e.editAction(func() { t.insertRunes(t.cur.Line, t.cur.Col, e.clip) })
}

// copySelection copies the selected region to the clipboard without removing it.
func (e *Editor) copySelection() {
	t := e.active()
	if t == nil || !t.hasSelection() {
		return
	}
	sel := t.selRange()
	var out []rune
	for l := sel[0]; l <= sel[2]; l++ {
		if l > sel[0] {
			out = append(out, '\n')
		}
		line := t.line(l)
		start, end := 0, len(line)
		if l == sel[0] {
			start = sel[1]
		}
		if l == sel[2] {
			end = sel[3]
		}
		start, end = clampCol(line, start), clampCol(line, end)
		if start < end {
			out = append(out, line[start:end]...)
		}
	}
	e.setClipboard(out)
	e.statusf("Copied %d character%s", len(out), plural(len(out)))
}

// --- justify -------------------------------------------------------------

// justify re-wraps the paragraph at the cursor to the width of the text pane,
// as a single undoable action.
func (e *Editor) justify() {
	t := e.active()
	if t == nil {
		return
	}
	if t.readOnly {
		e.statusf("%s is read-only", t.name)
		return
	}
	start, end := paragraphBounds(t, t.cur.Line)
	if start < 0 {
		e.statusf("Nothing to justify")
		return
	}
	width := e.wrapWidth(t)
	if width < 20 {
		width = 20
	}
	indent := string(leadingWhitespace(t.line(start)))
	var words []string
	for i := start; i <= end; i++ {
		words = append(words, strings.Fields(string(t.line(i)))...)
	}
	if len(words) == 0 {
		e.statusf("Nothing to justify")
		return
	}
	wrapped := wrapWords(words, indent, width)
	if wrapped == joinLinesOf(t, start, end) {
		e.statusf("Already justified")
		return
	}

	t.edits.begin()
	// Replace the paragraph: one delete plus one insert, undone together.
	lastLen := len(t.line(end))
	removed := t.deleteRegion(start, 0, end, lastLen)
	if len(removed) > 0 {
		t.edits.push(&op{kind: opDelete, line: start, col: 0, text: removed, curBefore: t.cur, curAfter: Pos{start, 0}})
	}
	ins := []rune(wrapped)
	o := &op{kind: opInsert, line: start, col: 0, text: ins, curBefore: Pos{start, 0}}
	insertText(t.text, start, 0, ins)
	o.curAfter = endOfInsert(start, 0, ins)
	t.edits.push(o)
	t.edits.end()

	t.mark = nil
	t.cur = o.curAfter
	t.destCol = t.cur.Col
	t.setDirty()
	t.invalidate(start)
	t.clampCursor()
}

// paragraphBounds returns the run of non-blank lines around line, or -1 when
// the cursor is on a blank line.
func paragraphBounds(t *Tab, line int) (int, int) {
	if line < 0 || line >= t.lineCount() || len(strings.TrimSpace(string(t.line(line)))) == 0 {
		return -1, -1
	}
	start, end := line, line
	for start > 0 && len(strings.TrimSpace(string(t.line(start-1)))) > 0 {
		start--
	}
	for end+1 < t.lineCount() && len(strings.TrimSpace(string(t.line(end+1)))) > 0 {
		end++
	}
	return start, end
}

func joinLinesOf(t *Tab, start, end int) string {
	var b strings.Builder
	for i := start; i <= end; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		b.WriteString(string(t.line(i)))
	}
	return b.String()
}

// wrapWords lays words out in lines no wider than width, prefixing indent.
func wrapWords(words []string, indent string, width int) string {
	var b strings.Builder
	cur := indent
	curLen := len([]rune(indent))
	first := true
	for _, w := range words {
		wl := len([]rune(w))
		if !first && curLen+1+wl > width {
			b.WriteString(cur)
			b.WriteByte('\n')
			cur = indent + w
			curLen = len([]rune(indent)) + wl
			continue
		}
		if first {
			cur += w
			curLen += wl
			first = false
			continue
		}
		cur += " " + w
		curLen += 1 + wl
	}
	b.WriteString(cur)
	return b.String()
}

// --- exit / save loop ----------------------------------------------------

func (e *Editor) exit() {
	e.exitPending = true
	e.exitIdx = 0
	e.exitNext()
}

func (e *Editor) exitNext() {
	if !e.exitPending {
		return
	}
	for i := e.exitIdx; i < len(e.tabs); i++ {
		if e.tabs[i].dirty {
			e.exitIdx = i
			e.promptSaveModified(i)
			return
		}
	}
	e.exitPending = false
	e.quit()
}

// cancelExit abandons a pending exit sequence, e.g. when a prompt in the
// sequence is dismissed with Esc.
func (e *Editor) cancelExit() {
	if e.exitPending {
		e.exitPending = false
		e.statusf("Exit cancelled")
	}
}

func (e *Editor) promptSaveModified(i int) {
	t := e.tabs[i]
	e.beginPromptCancel(sprintf("Save the buffer for %s? (No will DISCARD changes) [Y/n/c] ", t.name), "", nil,
		func(ans string) {
			switch ans {
			case "", "y", "Y":
				if t.path == "" {
					e.promptExitFilename(i)
					return
				}
				e.writeTabChecked(t, t.path, func(ok bool) {
					if !ok {
						e.exitPending = false
						return
					}
					e.exitIdx++
					e.exitNext()
				})
			case "n", "N":
				e.exitIdx++
				e.exitNext()
			default:
				e.cancelExit()
			}
		}, e.cancelExit)
}

func (e *Editor) promptExitFilename(i int) {
	t := e.tabs[i]
	e.beginPromptCancel("File Name to Write: ", t.name, nil, func(name string) {
		if name == "" {
			e.cancelExit()
			return
		}
		if !e.writeTab(t, name) {
			e.exitPending = false
			return
		}
		e.exitIdx++
		e.exitNext()
	}, e.cancelExit)
}
