package editor

const helpText = `goat - a text editor for mere mortals
Copyright (C) 2026 Samuele Zoncada

Movement: ^A/^E home/end, ^B/^F left/right, ^N/^P down/up,
^Y page up, arrows move, Meta+Left/Right or Ctrl+Left/Right word.

Editing: ^K/^X cut, ^U/^V paste, ^O/^S save, ^R read file,
^W search, ^\ replace, ^D delete, ^H backspace, ^J justify,
^I tab, Enter newline (auto-indent).

Select: Shift+arrows extend, Alt+Space mark on/off, ^C copy,
^K/^X cut selection, Esc clear.

Tabs: Ctrl+Tab next, Ctrl+Shift+Tab prev, Meta+T new,
Meta+W or click × to close (prompts to save).
Exit: ^Q (or Meta+Q), prompts to save.

Browser: Ctrl+B/Alt+S toggle, Alt+Tab focus, Enter open file / expand dir,
Right or + expand, Left or - collapse, Backspace collapse/up, Tab or Esc close.

Other: ^G this help, ^P find file, Meta+A select all, ^L refresh,
Meta+D go to definition/usages, ^Z/Alt+Z undo, ^Y/Alt+Y redo, Alt+Up/Down scroll.

Search prompt: ^W next, Alt+Q reverse, Alt+C case, Esc/^X cancel.
Replace: y yes, n no, a all.
`

func (e *Editor) drawHelp() {
	bg := e.theme.Plain
	e.fillRect(0, e.width, 0, e.height, bg)
	lines := splitLines(helpText)
	y := 1
	for i := 0; i < len(lines) && y < e.height-1; i++ {
		e.putStr(2, y, truncateRunes(lines[i], e.width-4), bg)
		y++
	}
	// footer
	footerStyle := bg.Reverse(true)
	e.fillRow(0, e.width, e.height-1, footerStyle)
	e.putStr(1, e.height-1, " ^G or Esc to close help", footerStyle)
	e.screen.HideCursor()
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
