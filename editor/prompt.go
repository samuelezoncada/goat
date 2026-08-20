package editor

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// Prompt captures one line of user input with a label.
type Prompt struct {
	label    string
	initial  string
	input    []rune
	pos      int
	onSubmit func(text string)
	onChange func(text string)
	onCancel func()
}

func (e *Editor) beginPrompt(label, initial string, onChange func(string), onSubmit func(string)) {
	e.prompt = &Prompt{
		label:    label,
		initial:  initial,
		input:    []rune(initial),
		pos:      len([]rune(initial)),
		onSubmit: onSubmit,
		onChange: onChange,
	}
	e.mode = ModePrompt
	e.clearMsg()
}

func (e *Editor) cancelPrompt() {
	e.prompt = nil
	e.mode = ModeNormal
}

// drawPromptLine renders the prompt input on the status row.
func (e *Editor) drawPromptLine() {
	y := e.height - 2
	style := statusStyle
	label := e.prompt.label
	e.fillRow(0, e.width, y, style)
	px := e.drawInputLine(1, y, label, e.prompt.input, e.prompt.pos, style)
	e.drawHints(e.height - 1)

	// cursor in prompt
	if px >= 0 && px < e.width {
		e.screen.ShowCursor(px, y)
	} else {
		e.screen.HideCursor()
	}
}

// drawInputLine draws label + input on row y starting at x0, scrolling the
// input horizontally so the caret stays visible, and returns the caret's x.
func (e *Editor) drawInputLine(x0, y int, label string, input []rune, pos int, style tcell.Style) int {
	labelW := runeLen(label)
	curIn := displayCol(input, pos) // caret offset within the input, in cells
	avail := e.width - x0 - labelW
	if avail < 1 {
		avail = 1
	}
	scroll := 0
	if curIn >= avail {
		scroll = curIn - (avail - 1)
	}
	e.putStr(x0, y, label, style)
	e.putStr(x0+labelW-scroll, y, string(input), style)
	return x0 + labelW + curIn - scroll
}

// key handles a key while a prompt is active.
func (e *Editor) promptKey(ev *tcell.EventKey) {
	p := e.prompt
	if p == nil {
		return
	}
	mod := ev.Modifiers()
	ctrl := mod&tcell.ModCtrl != 0
	alt := mod&tcell.ModAlt != 0

	switch ev.Key() {
	case tcell.KeyEsc:
		e.cancelPrompt()
		if p.onCancel != nil {
			p.onCancel()
		}
		return
	case tcell.KeyCtrlC, tcell.KeyCtrlG, tcell.KeyCtrlX:
		e.cancelPrompt()
		if p.onCancel != nil {
			p.onCancel()
		}
		return
	case tcell.KeyCtrlW:
		// ^W in the search/replace prompts repeats the search.
		if isSearchPrompt(p.label) {
			e.searchNext()
		}
		return
	case tcell.KeyEnter:
		text := string(p.input)
		e.prompt = nil
		e.mode = ModeNormal
		if e.screen != nil {
			e.screen.HideCursor()
		}
		if p.onSubmit != nil {
			p.onSubmit(text)
		}
		return
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if p.pos > 0 {
			p.pos--
			p.input = append(p.input[:p.pos], p.input[p.pos+1:]...)
			e.promptChanged(p)
		}
		return
	case tcell.KeyDelete:
		if p.pos < len(p.input) {
			p.input = append(p.input[:p.pos], p.input[p.pos+1:]...)
			e.promptChanged(p)
		}
		return
	case tcell.KeyLeft:
		if p.pos > 0 {
			p.pos--
		}
		return
	case tcell.KeyRight:
		if p.pos < len(p.input) {
			p.pos++
		}
		return
	case tcell.KeyHome:
		p.pos = 0
		return
	case tcell.KeyEnd:
		p.pos = len(p.input)
		return
	case tcell.KeyCtrlV:
		text := e.clip
		if len(text) == 0 {
			return
		}
		clean := make([]rune, 0, len(text))
		for _, r := range text {
			if r != '\n' && r != '\r' {
				clean = append(clean, r)
			}
		}
		if len(clean) == 0 {
			return
		}
		p.input = append(p.input, make([]rune, len(clean))...)
		copy(p.input[p.pos+len(clean):], p.input[p.pos:len(p.input)-len(clean)])
		copy(p.input[p.pos:], clean)
		p.pos += len(clean)
		e.promptChanged(p)
		return
	case tcell.KeyRune:
		if alt {
			if isSearchPrompt(p.label) {
				e.searchReverse()
			}
			return
		}
		if ctrl {
			// control chars shouldn't be inserted
			return
		}
		r := ev.Rune()
		p.input = append(p.input, 0)
		copy(p.input[p.pos+1:], p.input[p.pos:])
		p.input[p.pos] = r
		p.pos++
		e.promptChanged(p)
		return
	}
}

func (e *Editor) promptChanged(p *Prompt) {
	if p.onChange != nil {
		p.onChange(string(p.input))
	}
}

// isSearchPrompt reports whether the prompt belongs to search/replace.
func isSearchPrompt(label string) bool {
	return strings.HasPrefix(label, "Search") || strings.HasPrefix(label, "Replace")
}
