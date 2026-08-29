package editor

import (
	"os"
	"path/filepath"
	"sort"
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

	histKey string
	histIdx int      // position while browsing history; len(hist) = current input
	histSav []rune   // input stashed when history browsing started
	comp    []string // pending completion candidates
	compIdx int
}

func (e *Editor) beginPrompt(label, initial string, onChange func(string), onSubmit func(string)) {
	e.beginPromptCancel(label, initial, onChange, onSubmit, nil)
}

// beginPromptCancel is beginPrompt with a callback for dismissal, so a
// multi-step flow (exit, replace) can unwind instead of being left half done.
func (e *Editor) beginPromptCancel(label, initial string, onChange func(string), onSubmit func(string), onCancel func()) {
	if e.prompt != nil {
		// A prompt opened from inside another prompt's callback: remember the
		// outer one so it is not silently lost.
		e.promptStack = append(e.promptStack, e.prompt)
	}
	key := promptHistoryKey(label)
	e.prompt = &Prompt{
		label:    label,
		initial:  initial,
		input:    []rune(initial),
		pos:      len([]rune(initial)),
		onSubmit: onSubmit,
		onChange: onChange,
		onCancel: onCancel,
		histKey:  key,
		histIdx:  len(e.history[key]),
	}
	e.mode = ModePrompt
	e.clearMsg()
}

func (e *Editor) cancelPrompt() {
	e.prompt = nil
	e.promptStack = nil
	e.mode = ModeNormal
	if e.screen != nil {
		e.screen.HideCursor()
	}
}

// promptHistoryKey groups prompts that should share a history list.
func promptHistoryKey(label string) string {
	switch {
	case strings.HasPrefix(label, "Search"), strings.HasPrefix(label, "Replace Where"):
		return "search"
	case strings.HasPrefix(label, "Replace with"):
		return "replace"
	case strings.HasPrefix(label, "File"):
		return "file"
	case strings.HasPrefix(label, "Goto"):
		return "goto"
	}
	return ""
}

// rememberPrompt appends a submitted value to its history list.
func (e *Editor) rememberPrompt(key, val string) {
	if key == "" || val == "" {
		return
	}
	if e.history == nil {
		e.history = map[string][]string{}
	}
	h := e.history[key]
	for i, v := range h {
		if v == val {
			h = append(h[:i], h[i+1:]...)
			break
		}
	}
	h = append(h, val)
	if len(h) > 50 {
		h = h[len(h)-50:]
	}
	e.history[key] = h
}

// promptInsert inserts text at the caret, used for pasting into a prompt.
func (e *Editor) promptInsert(rs []rune) {
	p := e.prompt
	if p == nil {
		return
	}
	clean := make([]rune, 0, len(rs))
	for _, r := range rs {
		if r != '\n' && r != '\r' && r != '\t' {
			clean = append(clean, r)
		}
	}
	if len(clean) == 0 {
		return
	}
	p.input = append(p.input, clean...)
	copy(p.input[p.pos+len(clean):], p.input[p.pos:len(p.input)-len(clean)])
	copy(p.input[p.pos:], clean)
	p.pos += len(clean)
	p.comp = nil
	e.promptChanged(p)
}

// drawPromptLine renders the prompt input on the status row.
func (e *Editor) drawPromptLine() {
	y := e.height - 2
	style := statusStyle
	e.fillRow(0, e.width, y, style)
	px := e.drawInputLine(1, y, e.prompt.label, e.prompt.input, e.prompt.pos, style)
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
	curIn := displayCol(input, pos, 8) // caret offset within the input, in cells
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
		e.mode = ModeNormal
		return
	}
	if e.pasteActive {
		e.pasteKey(ev)
		return
	}
	mod := ev.Modifiers()
	ctrl := mod&tcell.ModCtrl != 0
	alt := mod&tcell.ModAlt != 0

	if ev.Key() != tcell.KeyTab {
		p.comp = nil
	}

	switch ev.Key() {
	case tcell.KeyEsc, tcell.KeyCtrlC, tcell.KeyCtrlG, tcell.KeyCtrlX:
		cancel := p.onCancel
		e.cancelPrompt()
		if cancel != nil {
			cancel()
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
		e.rememberPrompt(p.histKey, text)
		e.prompt = nil
		e.mode = ModeNormal
		if len(e.promptStack) > 0 {
			e.promptStack = e.promptStack[:len(e.promptStack)-1]
		}
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
	case tcell.KeyHome, tcell.KeyCtrlA:
		p.pos = 0
		return
	case tcell.KeyEnd, tcell.KeyCtrlE:
		p.pos = len(p.input)
		return
	case tcell.KeyUp:
		e.promptHistory(p, -1)
		return
	case tcell.KeyDown:
		e.promptHistory(p, 1)
		return
	case tcell.KeyCtrlU:
		p.input = p.input[:0]
		p.pos = 0
		e.promptChanged(p)
		return
	case tcell.KeyTab:
		e.promptComplete(p)
		return
	case tcell.KeyCtrlV:
		e.promptInsert(e.clip)
		return
	case tcell.KeyRune:
		if alt {
			if isSearchPrompt(p.label) {
				switch ev.Rune() {
				case 'c':
					e.search.caseSens = !e.search.caseSens
					e.statusf("Case sensitivity: %v", e.search.caseSens)
				case 'r':
					e.search.regex = !e.search.regex
					e.statusf("Regular expression: %v", e.search.regex)
				case 'u':
					e.search.wholeWord = !e.search.wholeWord
					e.statusf("Whole word: %v", e.search.wholeWord)
				default:
					e.searchReverse()
				}
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

// promptHistory replaces the input with a previous entry (dir -1 = older).
func (e *Editor) promptHistory(p *Prompt, dir int) {
	h := e.history[p.histKey]
	if len(h) == 0 {
		return
	}
	if p.histIdx == len(h) {
		p.histSav = cloneRunes(p.input)
	}
	idx := p.histIdx + dir
	if idx < 0 {
		idx = 0
	}
	if idx > len(h) {
		idx = len(h)
	}
	p.histIdx = idx
	if idx == len(h) {
		p.input = cloneRunes(p.histSav)
	} else {
		p.input = []rune(h[idx])
	}
	p.pos = len(p.input)
	e.promptChanged(p)
}

// promptComplete completes a filename prefix, cycling through the candidates
// on repeated Tab presses.
func (e *Editor) promptComplete(p *Prompt) {
	if p.histKey != "file" {
		return
	}
	if len(p.comp) > 0 {
		p.compIdx = (p.compIdx + 1) % len(p.comp)
		e.setPromptText(p, p.comp[p.compIdx])
		return
	}
	text := string(p.input[:p.pos])
	cands := completePath(text)
	if len(cands) == 0 {
		return
	}
	if len(cands) == 1 {
		e.setPromptText(p, cands[0])
		return
	}
	if common := commonPrefix(cands); len(common) > len(text) {
		e.setPromptText(p, common)
	}
	p.comp = cands
	p.compIdx = -1
	e.statusf("%d completions (Tab to cycle)", len(cands))
}

func (e *Editor) setPromptText(p *Prompt, s string) {
	p.input = []rune(s)
	p.pos = len(p.input)
	e.promptChanged(p)
}

// completePath returns the paths that start with the given prefix.
func completePath(prefix string) []string {
	dir, base := filepath.Split(prefix)
	lookIn := dir
	if lookIn == "" {
		lookIn = "."
	}
	if strings.HasPrefix(lookIn, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			lookIn = filepath.Join(home, strings.TrimPrefix(lookIn, "~"))
		}
	}
	des, err := os.ReadDir(lookIn)
	if err != nil {
		return nil
	}
	var out []string
	for _, de := range des {
		name := de.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		if base == "" && strings.HasPrefix(name, ".") {
			continue // don't propose hidden files unprompted
		}
		full := dir + name
		if de.IsDir() {
			full += string(filepath.Separator)
		}
		out = append(out, full)
	}
	sort.Strings(out)
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}

func commonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	p := ss[0]
	for _, s := range ss[1:] {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
			if p == "" {
				return ""
			}
		}
	}
	return p
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
