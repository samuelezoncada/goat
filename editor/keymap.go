package editor

import (
	"os"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// handle routes terminal events.
func (e *Editor) handle(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		e.width, e.height = ev.Size()
		e.frame = nil
		e.screen.Sync()
	case *tcell.EventKey:
		switch e.mode {
		case ModeHelp:
			e.mode = ModeNormal
		case ModePrompt:
			e.promptKey(ev)
		case ModePicker:
			e.pickerKey(ev)
		default:
			e.handleNormalKey(ev)
		}
	case *tcell.EventMouse:
		e.handleMouse(ev)
	case *tcell.EventPaste:
		if ev.Start() {
			e.pasteActive = true
			e.pasteBuf = nil
		} else {
			e.pasteActive = false
			e.insertPaste()
		}
	case *tcell.EventError:
		e.statusf("terminal error: %v", ev.Error())
	case *tcell.EventInterrupt:
		// highlight-ready wake; the next draw shows fresh spans
	}
}

func (e *Editor) handleNormalKey(ev *tcell.EventKey) {
	if e.pasteActive {
		if ev.Key() == tcell.KeyRune {
			e.pasteBuf = append(e.pasteBuf, ev.Rune())
		}
		return
	}
	if ev.Key() == tcell.KeyCtrlP {
		e.openPicker()
		return
	}
	if ev.Key() == tcell.KeyCtrlB {
		e.sidebar.toggle()
		return
	}
	t := e.active()
	if t == nil {
		return
	}

	mod := ev.Modifiers()
	ctrl := mod&tcell.ModCtrl != 0
	shift := mod&tcell.ModShift != 0

	if e.focus == FocusSidebar && e.sidebar.open {
		e.sidebarKey(ev)
		return
	}

	switch ev.Key() {
	case tcell.KeyCtrlQ:
		e.exit()
	case tcell.KeyCtrlO, tcell.KeyCtrlS:
		e.save()
	case tcell.KeyCtrlR:
		e.readFile()
	case tcell.KeyCtrlG:
		e.mode = ModeHelp
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
	case tcell.KeyCtrlT:
		e.statusf("Spell check not implemented")
	case tcell.KeyCtrlA:
		t.home()
	case tcell.KeyCtrlE:
		t.end()
	case tcell.KeyCtrlY:
		t.pgup(e.mainHeight() - 1)
	case tcell.KeyCtrlN:
		t.moveDown()
	case tcell.KeyCtrlP:
		t.moveUp()
	case tcell.KeyCtrlF:
		t.moveRight()
	case tcell.KeyCtrlD:
		t.deleteForward()
	case tcell.KeyCtrlH:
		t.backspace()
	case tcell.KeyEnter:
		t.insertNewline()
	case tcell.KeyUp:
		e.selectMove(t.moveUp, shift)
	case tcell.KeyDown:
		e.selectMove(t.moveDown, shift)
	case tcell.KeyLeft:
		if mod&tcell.ModAlt != 0 || ctrl {
			e.selectMove(t.wordLeft, shift)
		} else {
			e.selectMove(t.moveLeft, shift)
		}
	case tcell.KeyRight:
		if mod&tcell.ModAlt != 0 || ctrl {
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
		t.deleteForward()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		t.backspace()
	case tcell.KeyTab:
		if mod&tcell.ModAlt != 0 {
			e.cycleFocus()
		} else if ctrl {
			e.switchTab(1)
		} else {
			t.insertTab()
		}
	case tcell.KeyBacktab:
		e.switchTab(-1)
	case tcell.KeyEsc:
		e.clearMsg()
		if t := e.active(); t != nil {
			t.mark = nil
		}
	case tcell.KeyRune:
		if ctrl {
			return
		}
		if mod&tcell.ModAlt != 0 {
			e.altRune(ev.Rune())
			return
		}
		if ev.Rune() >= 0x20 && ev.Rune() != 0x7f {
			t.insertRune(ev.Rune())
		}
	}
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
	switch r {
	case 's':
		e.sidebar.toggle()
	case 't':
		e.newTab()
	case 'w':
		e.closeCurrentTab()
	case 'a':
		e.selectAll()
	case ' ':
		if t := e.active(); t != nil {
			t.toggleMark()
		}
	case 'g':
		e.gotoLine()
	case 'z':
		if t := e.active(); t != nil {
			t.undo()
		}
	case 'y':
		if t := e.active(); t != nil {
			t.redo()
		}
	case 'c':
		if e.search.text != "" {
			e.search.caseSens = !e.search.caseSens
			e.statusf("Case sensitivity: %v", e.search.caseSens)
		}
	case 'q':
		e.exit()
	}
}

// sidebarKey handles keys while the sidebar has focus.
func (e *Editor) sidebarKey(ev *tcell.EventKey) {
	s := e.sidebar
	switch ev.Key() {
	case tcell.KeyCtrlQ:
		e.exit()
	case tcell.KeyEsc:
		e.sidebar.toggle()
	case tcell.KeyCtrlB:
		s.toggle()
	case tcell.KeyUp:
		s.moveUp()
	case tcell.KeyDown:
		s.moveDown()
	case tcell.KeyHome:
		s.home()
	case tcell.KeyEnd:
		s.end()
	case tcell.KeyPgUp:
		for i := 0; i < e.mainHeight()-1 && s.sel > 0; i++ {
			s.moveUp()
		}
	case tcell.KeyPgDn:
		for i := 0; i < e.mainHeight()-1 && s.sel < len(s.entries)-1; i++ {
			s.moveDown()
		}
	case tcell.KeyEnter:
		s.enter()
	case tcell.KeyTab:
		e.focusText()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		s.goUp()
	case tcell.KeyRune:
		if ev.Modifiers()&tcell.ModAlt != 0 && ev.Rune() == 's' {
			s.toggle()
		}
	}
}

func (e *Editor) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	btn := ev.Buttons()
	if btn&tcell.WheelUp != 0 {
		if e.focus == FocusSidebar && e.sidebar.open && x < e.sidebar.width() {
			e.sidebar.top--
			return
		}
		if t := e.active(); t != nil && t.top > 0 {
			t.top--
		}
		return
	}
	if btn&tcell.WheelDown != 0 {
		if e.focus == FocusSidebar && e.sidebar.open && x < e.sidebar.width() {
			e.sidebar.top++
			return
		}
		if t := e.active(); t != nil {
			t.top++
		}
		return
	}
	if btn&tcell.Button1 != 0 {
		if y < e.mainTop() && e.handleTabBarClick(x) {
			return
		}
		if e.sidebar.open && x < e.sidebar.width() && y >= e.mainTop() {
			idx := e.sidebar.top + (y - e.mainTop() - 1)
			if idx >= 0 && idx < len(e.sidebar.entries) {
				e.sidebar.sel = idx
				e.focus = FocusSidebar
				en := e.sidebar.selEntry()
				if en != nil && !en.isDir && en.name != ".." {
					e.sidebar.enter()
				}
			}
			return
		}
		e.focusText()
		e.clickToCursor(x, y)
	}
}

// clickToCursor places the cursor at a clicked cell.
func (e *Editor) clickToCursor(x, y int) {
	t := e.active()
	if t == nil {
		return
	}
	line := t.top + (y - e.mainTop())
	if line < 0 || line >= t.lineCount() {
		return
	}
	disp := x - (e.editorLeft() + e.gutterWidth()) + t.left
	if disp < 0 {
		disp = 0
	}
	t.cur.Line = line
	t.cur.Col = colFromDisp(t.line(line), disp)
	t.destCol = t.cur.Col
}

// colFromDisp maps a display column to a rune column.
func colFromDisp(line []rune, disp int) int {
	c := 0
	d := 0
	for c < len(line) {
		w := runeWidth(line[c])
		if line[c] == '\t' {
			w = tabStop - d%tabStop
		}
		if d+w > disp {
			break
		}
		d += w
		c++
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
	e.writeTab(t, t.path)
}

func (e *Editor) writeTab(t *Tab, path string) {
	if err := t.saveTo(path); err != nil {
		e.statusf("Error writing %s: %v", path, err)
		return
	}
	e.statusf("Wrote %d lines", t.lineCount())
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
		data, err := os.ReadFile(path)
		if err != nil {
			e.statusf("Error reading %s: %v", path, err)
			return
		}
		t.insertRunes(t.cur.Line, t.cur.Col, []rune(string(data)))
		e.statusf("Read %d bytes", len(data))
	})
}

func (e *Editor) redrawScreen() {
	e.frame = nil
	e.screen.Sync()
}

// cycleFocus moves keyboard focus between text and the sidebar.
func (e *Editor) cycleFocus() {
	if !e.sidebar.open {
		e.sidebar.open = true
	}
	if e.focus == FocusText {
		e.focus = FocusSidebar
		e.sidebar.refresh()
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
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			e.statusf("Invalid line number")
			return
		}
		t := e.active()
		if t == nil {
			return
		}
		n--
		if n >= t.lineCount() {
			n = t.lineCount() - 1
		}
		t.cur.Line = n
		t.cur.Col = 0
		t.destCol = 0
	})
}

func (e *Editor) insertPaste() {
	t := e.active()
	if t == nil || len(e.pasteBuf) == 0 {
		return
	}
	t.insertRunes(t.cur.Line, t.cur.Col, e.pasteBuf)
	e.pasteBuf = nil
}

// --- cut / uncut ---------------------------------------------------------

func (e *Editor) cut() {
	t := e.active()
	if t == nil {
		return
	}
	if t.mark != nil {
		sel := e.selection()
		t.mark = nil
		removed := deleteRegion(t, sel[0], sel[1], sel[2], sel[3])
		e.clip = removed
		o := &op{kind: opDelete, line: sel[0], col: sel[1], text: removed, curBefore: t.cur}
		t.cur = Pos{sel[0], sel[1]}
		t.destCol = t.cur.Col
		o.curAfter = t.cur
		t.edits.push(o)
		t.setDirty()
		t.invalidate(sel[0])
		return
	}
	line := t.cur.Line
	l := t.line(line)
	var removed []rune
	o := &op{kind: opDelete, line: line, col: 0, curBefore: t.cur}
	if line+1 < t.lineCount() {
		removed = deleteText(t.text, line, 0, len(l)+1)
	} else {
		removed = deleteText(t.text, line, 0, len(l))
	}
	o.text = removed
	e.clip = removed
	t.cur = Pos{line, 0}
	t.destCol = 0
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(line)
}

func (e *Editor) uncut() {
	t := e.active()
	if t == nil || len(e.clip) == 0 {
		return
	}
	t.insertRunes(t.cur.Line, t.cur.Col, e.clip)
}

// copySelection copies the selected region to the clipboard without removing it.
func (e *Editor) copySelection() {
	t := e.active()
	if t == nil || t.mark == nil {
		return
	}
	sel := e.selection()
	var b strings.Builder
	for l := sel[0]; l <= sel[2]; l++ {
		if l > sel[0] {
			b.WriteByte('\n')
		}
		line := string(t.line(l))
		start, end := 0, len(line)
		if l == sel[0] {
			start = sel[1]
		}
		if l == sel[2] {
			end = sel[3]
		}
		if start < 0 {
			start = 0
		}
		if start > len(line) {
			start = len(line)
		}
		if end > len(line) {
			end = len(line)
		}
		if start < end {
			b.WriteString(line[start:end])
		}
	}
	e.clip = []rune(b.String())
	e.statusf("Copied %d characters", len(e.clip))
}

// deleteRegion removes the rune range [sLine:sCol .. eLine:eCol).
func deleteRegion(t *Tab, sLine, sCol, eLine, eCol int) []rune {
	var total int
	if sLine == eLine {
		total = eCol - sCol
	} else {
		total += len(t.line(sLine)) - sCol
		for l := sLine + 1; l <= eLine; l++ {
			total++
			if l < eLine {
				total += len(t.line(l))
			} else {
				total += eCol
			}
		}
	}
	return deleteText(t.text, sLine, sCol, total)
}

// --- justify -------------------------------------------------------------

// justify joins the paragraph starting at the cursor into a single line.
func (e *Editor) justify() {
	t := e.active()
	if t == nil {
		return
	}
	start := t.cur.Line
	end := start
	for end+1 < t.lineCount() && len(t.line(end+1)) > 0 {
		end++
	}
	if end == start {
		e.statusf("Nothing to justify")
		return
	}
	// Concatenate the trailing lines.
	tail := []rune{}
	for i := start + 1; i <= end; i++ {
		tail = append(tail, t.line(i)...)
	}
	// Remove lines start+1..end (highest first) and record undo ops.
	for i := end; i > start; i-- {
		removed := t.text.RemoveLine(i)
		o := &op{kind: opLineRemove, line: i, text: removed, curBefore: t.cur}
		o.curAfter = t.cur
		t.edits.push(o)
	}
	// Append the tail to the start line.
	o := &op{kind: opInsert, line: start, col: len(t.line(start)), text: tail, curBefore: t.cur}
	insertText(t.text, start, len(t.line(start)), tail)
	t.cur = Pos{start, len(t.line(start))}
	t.destCol = t.cur.Col
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(start)
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
	e.quit()
}

func (e *Editor) promptSaveModified(i int) {
	t := e.tabs[i]
	e.beginPrompt(sprintf("Save the buffer for %s? (No will DISCARD changes) [Y/n/c] ", t.name), "", nil, func(ans string) {
		switch ans {
		case "", "y", "Y":
			if t.path == "" {
				e.promptExitFilename(i)
			} else {
				if err := t.saveTo(t.path); err != nil {
					e.statusf("Error writing: %v", err)
					e.exitPending = false
					return
				}
				e.exitIdx++
				e.exitNext()
			}
		case "n", "N":
			e.exitIdx++
			e.exitNext()
		default:
			e.exitPending = false
			e.statusf("Exit cancelled")
		}
	})
}

func (e *Editor) promptExitFilename(i int) {
	t := e.tabs[i]
	e.beginPrompt("File Name to Write: ", t.name, nil, func(name string) {
		if name == "" {
			e.exitPending = false
			return
		}
		if err := t.saveTo(name); err != nil {
			e.statusf("Error writing: %v", err)
			e.exitPending = false
			return
		}
		e.exitIdx++
		e.exitNext()
	})
}
