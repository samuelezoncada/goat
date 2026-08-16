package editor

import "github.com/gdamore/tcell/v2"

// helpRow is one keybinding shown in the help page.
type helpRow struct {
	keys string
	desc string
}

// helpSection groups related keybindings.
type helpSection struct {
	title string
	rows  []helpRow
}

const helpTitle = "goat - a text editor for mere mortals"

var helpSections = []helpSection{
	{
		title: "Movement",
		rows: []helpRow{
			{"^A / ^E", "Home / End"},
			{"^B / ^F", "Left / Right"},
			{"^N / ^P", "Down / Up"},
			{"^Y", "Page Up"},
			{"Arrows", "Move"},
			{"^Left / ^Right", "Word"},
			{"Alt+Left / Right", "Word"},
		},
	},
	{
		title: "Editing",
		rows: []helpRow{
			{"^K / ^X", "Cut"},
			{"^U / ^V", "Paste"},
			{"^O / ^S", "Save"},
			{"^R", "Read file"},
			{"^W", "Search"},
			{"^\\", "Replace"},
			{"^D", "Delete forward"},
			{"^H", "Backspace"},
			{"^J", "Justify"},
			{"^I", "Tab"},
			{"Enter", "Newline"},
		},
	},
	{
		title: "Select",
		rows: []helpRow{
			{"Shift+Arrows", "Extend selection"},
			{"Alt+Space", "Mark on / off"},
			{"^C", "Copy selection"},
			{"^K / ^X", "Cut selection"},
			{"Esc", "Clear selection"},
		},
	},
	{
		title: "Tabs",
		rows: []helpRow{
			{"Ctrl+Tab", "Next tab"},
			{"Ctrl+Shift+Tab", "Previous tab"},
			{"Alt+T", "New tab"},
			{"Alt+W", "Close tab"},
			{"Click \u00d7", "Close tab"},
		},
	},
	{
		title: "Exit",
		rows: []helpRow{
			{"^Q / Alt+Q", "Quit (prompts to save)"},
		},
	},
	{
		title: "File browser",
		rows: []helpRow{
			{"Ctrl+B / Alt+S", "Toggle browser"},
			{"Alt+Tab", "Focus"},
			{"Enter", "Open / expand"},
			{"Right / +", "Expand dir"},
			{"Left / -", "Collapse / parent"},
			{"Backspace", "Collapse / up"},
			{"Esc", "Close browser"},
		},
	},
	{
		title: "Other",
		rows: []helpRow{
			{"^G", "This help"},
			{"^P", "Find file"},
			{"Alt+A", "Select all"},
			{"^L", "Refresh screen"},
			{"Alt+D", "Definition / usages"},
			{"^Z / Alt+Z", "Undo"},
			{"^Y / Alt+Y", "Redo"},
			{"Alt+Up / Down", "Scroll"},
		},
	},
	{
		title: "Search prompt",
		rows: []helpRow{
			{"^W", "Next match"},
			{"Alt+Q", "Reverse"},
			{"Alt+C", "Case sensitivity"},
			{"Esc / ^X", "Cancel"},
		},
	},
	{
		title: "Replace",
		rows: []helpRow{
			{"y / n / a", "Yes / No / All"},
		},
	},
}

// openHelp shows the help page, resetting the scroll position.
func (e *Editor) openHelp() {
	e.helpTop = 0
	e.mode = ModeHelp
}

// helpKey handles keys while the help page is open: scroll keys move the
// view, any other key closes it.
func (e *Editor) helpKey(ev *tcell.EventKey) {
	twoCol := (e.width-4)/2 >= 24
	total := helpLineCount(twoCol)
	viewH := e.height - 3
	switch ev.Key() {
	case tcell.KeyUp:
		if e.helpTop > 0 {
			e.helpTop--
		}
	case tcell.KeyDown:
		if e.helpTop < total-viewH {
			e.helpTop++
		}
	case tcell.KeyPgUp:
		e.helpTop -= viewH
		if e.helpTop < 0 {
			e.helpTop = 0
		}
	case tcell.KeyPgDn:
		e.helpTop += viewH
		if e.helpTop > total-viewH {
			e.helpTop = total - viewH
		}
	default:
		e.mode = ModeNormal
		e.helpTop = 0
	}
}

// helpPart is one styled run of text on a help line.
type helpPart struct {
	x, w  int
	text  string
	style tcell.Style
}

func (e *Editor) drawHelp() {
	bg := e.theme.Plain
	titleStyle := e.theme.Heading
	keyStyle := e.theme.Keyword

	e.fillRect(0, e.width, 0, e.height, bg)

	const margin = 2
	colW := (e.width - 2*margin) / 2
	twoCol := colW >= 24
	keyW := 16
	if keyW > colW-4 {
		keyW = colW - 4
	}
	if keyW < 6 {
		keyW = 6
	}

	total := helpLineCount(twoCol)
	viewH := e.height - 3
	if e.helpTop > total-viewH {
		e.helpTop = total - viewH
	}
	if e.helpTop < 0 {
		e.helpTop = 0
	}

	line := 0
	drawLine := func(parts ...helpPart) {
		y := 1 + line - e.helpTop
		if y >= 1 && y < e.height-1 {
			for _, p := range parts {
				if p.w > 0 {
					e.putStr(p.x, y, truncateRunes(p.text, p.w), p.style)
				}
			}
		}
		line++
	}

	drawLine(helpPart{margin, e.width - margin - 1, helpTitle, titleStyle})
	line++ // blank after title
	for _, sec := range helpSections {
		drawLine(helpPart{margin, e.width - margin - 1, sec.title, titleStyle})
		half := len(sec.rows)
		if twoCol {
			half = (len(sec.rows) + 1) / 2
		}
		for i := 0; i < half; i++ {
			var parts []helpPart
			if i < len(sec.rows) {
				r := sec.rows[i]
				parts = append(parts,
					helpPart{margin, keyW, r.keys, keyStyle},
					helpPart{margin + keyW + 1, colW - keyW - 1, r.desc, bg},
				)
			}
			if twoCol && i+half < len(sec.rows) {
				r := sec.rows[i+half]
				xr := margin + colW
				parts = append(parts,
					helpPart{xr, keyW, r.keys, keyStyle},
					helpPart{xr + keyW + 1, e.width - (xr + keyW + 1) - margin, r.desc, bg},
				)
			}
			drawLine(parts...)
		}
		line++ // blank between sections
	}

	footerStyle := bg.Reverse(true)
	e.fillRow(0, e.width, e.height-1, footerStyle)
	e.putStr(1, e.height-1, " ^G or Esc to close help   ↑/↓ or PgUp/PgDn scroll", footerStyle)
	e.screen.HideCursor()
}

// helpLineCount returns the number of rendered content lines, mirroring the
// layout in drawHelp so scrolling can be clamped correctly.
func helpLineCount(twoCol bool) int {
	total := 2 // title + blank
	for _, sec := range helpSections {
		total += 1 // section title
		half := len(sec.rows)
		if twoCol {
			half = (len(sec.rows) + 1) / 2
		}
		total += half
		total += 1 // blank between sections
	}
	return total
}
