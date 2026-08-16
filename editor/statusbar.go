package editor

import (
	"github.com/gdamore/tcell/v2"
)

var (
	statusStyle = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite).Reverse(true)
	hintStyle   = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite).Reverse(true)
)

func (e *Editor) drawStatus() {
	if e.mode == ModePrompt {
		e.drawPromptLine()
		return
	}
	t := e.active()
	y := e.height - 2

	left := e.msg
	if left == "" && t != nil {
		left = t.displayName()
	}
	if t != nil && t.dirty {
		left = "* " + left
	}
	e.fillRow(0, e.width, y, statusStyle)
	e.putStr(1, y, truncateRunes(left, e.width-2), statusStyle)

	right := ""
	if t != nil {
		lang := "plain text"
		if t.lang != nil {
			lang = t.lang.Name
		}
		right = sprintf(" %s  Ln %d/%d  Col %d", lang, t.cur.Line+1, t.lineCount(), t.cur.Col+1)
	}
	x := e.putStr(e.width-len([]rune(right))-1, y, right, statusStyle)
	_ = x

	e.drawHints(e.height - 1)
}

func (e *Editor) drawHints(y int) {
	e.fillRow(0, e.width, y, hintStyle)
	var hints string
	if e.focus == FocusBrowser && e.browser.open {
		hints = " ↑/↓ Move   Enter/→ Open   ← Collapse   ^B Close   ^G Help   ^Q Exit"
	} else {
		hints = " ^Q Exit   ^O WriteOut   ^X Cut   ^C Copy   ^V Paste   ^W Search   ^\\ Replace   ^G Help   ^B Browser   Alt+W CloseTab"
	}
	e.putStr(1, y, hints, hintStyle)
}

func (t *Tab) displayName() string {
	if t.path != "" {
		return t.path
	}
	return t.name
}

func truncateRunes(s string, n int) string {
	if n < 0 {
		n = 0
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
