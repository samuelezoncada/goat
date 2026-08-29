package editor

import (
	"github.com/gdamore/tcell/v2"
)

// minLeftW is the room the filename cluster needs before the right-hand
// fields start dropping detail.
const minLeftW = 28

var (
	statusStyle = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite).Reverse(true)
	hintStyle   = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite).Reverse(true)
)

func (e *Editor) drawStatus() {
	if e.mode == ModePrompt && e.prompt != nil {
		e.drawPromptLine()
		return
	}
	t := e.active()
	y := e.height - 2

	// The left side is the file's context, in three parts: a prefix that must
	// always be visible, the name (or a status message), and short suffix
	// fields. Only the middle part is shortened, and a path is shortened from
	// the front so the filename survives.
	prefix, body, suffix := "", e.msg, ""
	if body == "" && t != nil {
		body = e.displayPath(t)
		if e.config().GitBranch {
			if b := e.branchFor(e.gitContextDir()); b != "" {
				suffix = "  git:" + b
			}
		}
	}
	if t != nil {
		if t.dirty {
			prefix = "* "
		}
		if t.readOnly {
			suffix += "  [read-only]"
		}
	}
	e.fillRow(0, e.width, y, statusStyle)

	right := ""
	if t != nil {
		right = " " + t.statusRight()
		// On a narrow screen the descriptive fields give way so the filename
		// and branch stay readable; the cursor position is kept.
		if e.width-runeLen(right)-2 < minLeftW {
			right = " " + t.statusRightShort()
		}
	}
	rightW := runeLen(right)
	// A message matters more than the cursor position: on a narrow screen the
	// position fields give way rather than truncating the message to nothing.
	if e.msg != "" && runeLen(prefix)+runeLen(body)+rightW+3 > e.width {
		right, rightW = "", 0
	}

	avail := e.width - 2 - rightW
	if avail < 0 {
		avail = 0
	}
	left := prefix + body + suffix
	if runeLen(left) > avail {
		room := avail - runeLen(prefix) - runeLen(suffix)
		if room < 8 {
			// Not enough room for the suffix fields either: keep the name.
			left = prefix + elideLeft(body, avail-runeLen(prefix))
		} else if e.msg != "" {
			left = prefix + truncateRunes(body, room) + suffix
		} else {
			left = prefix + elideLeft(body, room) + suffix
		}
	}
	e.putStr(1, y, truncateRunes(left, avail), statusStyle)
	if rightW > 0 && rightW+1 < e.width {
		e.putStr(e.width-rightW-1, y, right, statusStyle)
	}

	e.drawHints(e.height - 1)
}

// statusRight renders the right-hand status fields: language, encoding, line
// endings, and the cursor position in both rune and display columns.
func (t *Tab) statusRight() string {
	lang := "plain text"
	if t.lang != nil {
		lang = t.lang.Name
	}
	enc := "UTF-8"
	if t.rawBytes {
		enc = "UTF-8?" // holds bytes that are not valid UTF-8
	}
	eol := "LF"
	if !t.trailingNL {
		eol = "LF (no final newline)"
	}
	return sprintf("%s  %s  %s  %s", lang, enc, eol, t.statusRightShort())
}

// statusRightShort is the position alone, used when the status row is too
// narrow for the language, encoding and line-ending fields.
func (t *Tab) statusRightShort() string {
	disp := displayCol(t.line(t.cur.Line), t.cur.Col, t.tabW()) + 1
	if disp != t.cur.Col+1 {
		// Tabs or wide characters make the two differ; show both.
		return sprintf("Ln %d/%d  Col %d (%d)", t.cur.Line+1, t.lineCount(), t.cur.Col+1, disp)
	}
	return sprintf("Ln %d/%d  Col %d", t.cur.Line+1, t.lineCount(), t.cur.Col+1)
}

func (e *Editor) drawHints(y int) {
	e.fillRow(0, e.width, y, hintStyle)
	hints := " " + hintLine()
	if e.focus == FocusBrowser && e.browser.open {
		hints = " ↑/↓ Move   Enter/→ Open   ← Collapse   ^H Hidden   Alt+N New   Alt+R Rename   Alt+X Delete   ^B Close   ^G Help"
	}
	e.putStr(1, y, truncateRunes(hints, e.width-2), hintStyle)
}

// displayName is the buffer's absolute path, or its placeholder name.
func (t *Tab) displayName() string {
	if t.path != "" {
		return t.path
	}
	return t.name
}

// elideLeft shortens s to n cells by dropping the front, marking the cut with
// a leading ellipsis.
func elideLeft(s string, n int) string {
	r := []rune(s)
	if n < 0 {
		n = 0
	}
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return truncateRunes(s, n)
	}
	return "…" + string(r[len(r)-(n-1):])
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
