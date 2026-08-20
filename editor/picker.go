package editor

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

// match is a single picker result.
type match struct {
	path  string // absolute
	rel   string // relative to root, used for display/matching
	score int
	pos   []int // matched rune indices within rel
	isDir bool  // heavy dir shown as expandable, not walked into
}

// skipDirs are directories never indexed implicitly; they appear as
// expandable entries and are only walked when the user opens them.
var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "target": true, "dist": true,
	"build": true, "out": true, "bin": true, "obj": true,
	"__pycache__": true, ".venv": true, "venv": true, "env": true,
	".git": true, ".hg": true, ".svn": true, ".bzr": true,
	".idea": true, ".vscode": true, ".next": true, ".nuxt": true,
	".gradle": true, ".tox": true, ".mypy_cache": true, ".pytest_cache": true,
	"coverage": true, ".terraform": true, ".cargo": true, ".cache": true,
	"tmp": true,
}

// Picker is the Ctrl+P fuzzy file finder.
type Picker struct {
	e       *Editor
	input   []rune
	pos     int
	root    string
	files   []match
	matches []match
	sel     int
	top     int
}

func (e *Editor) openPicker() {
	root := e.root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			e.statusf("no working directory")
			return
		}
	}
	p := &Picker{e: e, root: root}
	p.buildIndex()
	p.refilter()
	e.picker = p
	e.mode = ModePicker
	e.clearMsg()
}

// SetRoot sets the directory the file picker searches (abs). Empty keeps the
// launch working directory as the root.
func (e *Editor) SetRoot(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		e.root = abs
	}
}

func (e *Editor) cancelPicker() {
	e.picker = nil
	e.mode = ModeNormal
	e.screen.HideCursor()
}

// buildIndex walks root and collects files, recording heavy dirs as
// expandable entries without walking into them.
func (p *Picker) buildIndex() {
	p.files = p.files[:0]
	p.indexDir(p.root)
	sort.Slice(p.files, func(i, j int) bool {
		return strings.ToLower(p.files[i].rel) < strings.ToLower(p.files[j].rel)
	})
}

// indexDir walks dir, appending files to p.files. Heavy subdirectories are
// recorded as expandable dir matches but are not walked into; they are only
// indexed when the user opens them (see expandDir).
func (p *Picker) indexDir(dir string) {
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != dir && skipDirs[name] {
				rel, _ := filepath.Rel(p.root, path)
				p.files = append(p.files, match{path: path, rel: rel, isDir: true})
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(p.root, path)
		if err != nil {
			return nil
		}
		p.files = append(p.files, match{path: path, rel: rel})
		return nil
	})
}

// expandDir replaces the selected heavy-dir match with its indexed contents,
// making the files inside searchable. Heavy dirs found within it are recorded
// as expandable entries in turn.
func (p *Picker) expandDir() {
	if p.sel < 0 || p.sel >= len(p.matches) {
		return
	}
	m := p.matches[p.sel]
	if !m.isDir {
		return
	}
	out := p.files[:0]
	for _, f := range p.files {
		if f.path != m.path {
			out = append(out, f)
		}
	}
	p.files = out
	p.indexDir(m.path)
	sort.Slice(p.files, func(i, j int) bool {
		return strings.ToLower(p.files[i].rel) < strings.ToLower(p.files[j].rel)
	})
	p.sel, p.top = 0, 0
	p.refilter()
}

// fuzzyScore scores a case-insensitive subsequence match of query against s.
// Returns the score, the matched rune indices, and whether it matched at all.
func fuzzyScore(query, s string) (int, []int, bool) {
	if query == "" {
		return 0, nil, true
	}
	q := []rune(strings.ToLower(query))
	rs := []rune(s)
	sl := []rune(strings.ToLower(s))
	if len(q) > len(rs) {
		return 0, nil, false
	}

	matched := []int{}
	qi := 0
	score := 0
	prev := -2
	for i, r := range sl {
		if r != q[qi] {
			continue
		}
		matched = append(matched, i)
		score++
		if i == prev+1 {
			score += 8 // consecutive run
		}
		if i == 0 || rs[i-1] == '/' || rs[i-1] == '_' || rs[i-1] == '-' || rs[i-1] == '.' || rs[i-1] == ' ' {
			score += 5 // path/word boundary start
		} else if i > 0 && unicode.IsLower(rs[i-1]) && unicode.IsUpper(rs[i]) {
			score += 5 // camelCase transition
		}
		prev = i
		qi++
		if qi == len(q) {
			base := 0
			for j, r := range rs {
				if r == '/' {
					base = j + 1
				}
			}
			if matched[0] >= base {
				score += 10 // match lives in the basename
			}
			return score, matched, true
		}
	}
	return 0, nil, false
}

// refilter applies the current query, ordering by MRU then score then name.
func (p *Picker) refilter() {
	query := string(p.input)
	rank := p.e.recentRank()
	p.matches = p.matches[:0]
	if query == "" {
		ordered := make([]match, len(p.files))
		copy(ordered, p.files)
		sort.Slice(ordered, func(i, j int) bool {
			ri, rj := rank(ordered[i].path), rank(ordered[j].path)
			if ri != rj {
				return ri < rj
			}
			return strings.ToLower(ordered[i].rel) < strings.ToLower(ordered[j].rel)
		})
		p.matches = ordered
	} else {
		for _, f := range p.files {
			if sc, pos, ok := fuzzyScore(query, f.rel); ok {
				p.matches = append(p.matches, match{path: f.path, rel: f.rel, score: sc, pos: pos})
			}
		}
		sort.SliceStable(p.matches, func(i, j int) bool {
			if p.matches[i].score != p.matches[j].score {
				return p.matches[i].score > p.matches[j].score
			}
			ri, rj := rank(p.matches[i].path), rank(p.matches[j].path)
			if ri != rj {
				return ri < rj
			}
			return strings.ToLower(p.matches[i].rel) < strings.ToLower(p.matches[j].rel)
		})
	}
	if p.sel >= len(p.matches) {
		p.sel = len(p.matches) - 1
	}
	if p.sel < 0 {
		p.sel = 0
	}
}

// recentRank returns a rank function: position in the MRU list, else a large value.
func (e *Editor) recentRank() func(string) int {
	m := make(map[string]int, len(e.recent))
	for i, p := range e.recent {
		m[p] = i
	}
	inf := len(e.recent) + 1
	return func(path string) int {
		if i, ok := m[path]; ok {
			return i
		}
		return inf
	}
}

// remember pushes a path onto the session MRU list (deduped, capped).
func (e *Editor) remember(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	out := []string{abs}
	for _, p := range e.recent {
		if p != abs {
			out = append(out, p)
		}
	}
	if len(out) > 30 {
		out = out[:30]
	}
	e.recent = out
}

// pickerKey handles keys while the picker is open.
func (e *Editor) pickerKey(ev *tcell.EventKey) {
	p := e.picker
	if p == nil {
		return
	}
	mod := ev.Modifiers()

	switch ev.Key() {
	case tcell.KeyEsc, tcell.KeyCtrlC, tcell.KeyCtrlG:
		e.cancelPicker()
	case tcell.KeyEnter:
		e.pickerOpen()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if p.pos > 0 {
			p.pos--
			p.input = append(p.input[:p.pos], p.input[p.pos+1:]...)
			p.sel, p.top = 0, 0
			p.refilter()
		}
	case tcell.KeyDelete:
		if p.pos < len(p.input) {
			p.input = append(p.input[:p.pos], p.input[p.pos+1:]...)
			p.sel, p.top = 0, 0
			p.refilter()
		}
	case tcell.KeyLeft:
		if p.pos > 0 {
			p.pos--
		}
	case tcell.KeyRight:
		if p.pos < len(p.input) {
			p.pos++
		}
	case tcell.KeyHome:
		p.pos = 0
	case tcell.KeyEnd:
		p.pos = len(p.input)
	case tcell.KeyUp:
		if p.sel > 0 {
			p.sel--
		}
	case tcell.KeyDown:
		if p.sel+1 < len(p.matches) {
			p.sel++
		}
	case tcell.KeyPgUp:
		p.sel -= e.mainHeight() - 2
		if p.sel < 0 {
			p.sel = 0
		}
	case tcell.KeyPgDn:
		p.sel += e.mainHeight() - 2
		if p.sel >= len(p.matches) {
			p.sel = len(p.matches) - 1
		}
	case tcell.KeyRune:
		if mod&tcell.ModCtrl != 0 {
			return
		}
		r := ev.Rune()
		if r >= 0x20 && r != 0x7f {
			p.input = append(p.input, 0)
			copy(p.input[p.pos+1:], p.input[p.pos:])
			p.input[p.pos] = r
			p.pos++
			p.sel, p.top = 0, 0
			p.refilter()
		}
	}
}

// pickerOpen opens the selected file into a tab, or expands a heavy dir.
func (e *Editor) pickerOpen() {
	p := e.picker
	if p == nil {
		return
	}
	if p.sel < 0 || p.sel >= len(p.matches) {
		e.cancelPicker()
		return
	}
	m := p.matches[p.sel]
	if m.isDir {
		p.expandDir()
		return
	}
	path := m.path
	e.cancelPicker()
	e.focusText()
	e.openPath(path)
}

// drawPicker renders the picker overlay.
func (e *Editor) drawPicker() {
	p := e.picker
	if p == nil {
		return
	}
	bg := e.theme.Plain
	inputStyle := statusStyle
	selStyle := bg.Reverse(true)
	dirStyle := e.theme.Type

	// input line at the top of the main area
	y := e.mainTop()
	e.fillRow(0, e.width, y, inputStyle)
	label := "> "
	px := e.drawInputLine(1, y, label, p.input, p.pos, inputStyle)

	// result list
	listTop := y + 1
	listH := e.mainHeight() - 1
	if p.sel < p.top {
		p.top = p.sel
	}
	if p.sel >= p.top+listH {
		p.top = p.sel - listH + 1
	}
	if p.top < 0 {
		p.top = 0
	}

	for row := 0; row < listH; row++ {
		idx := p.top + row
		yy := listTop + row
		if yy >= e.height-2 {
			break
		}
		if idx >= len(p.matches) {
			e.fillRow(0, e.width, yy, bg)
			continue
		}
		m := p.matches[idx]
		style := bg
		if m.isDir {
			style = dirStyle
		}
		if idx == p.sel {
			style = selStyle
		}
		e.fillRow(0, e.width, yy, style)
		label := m.rel
		if m.isDir {
			label += "/"
		}
		rr := []rune(label)
		hl := make(map[int]bool, len(m.pos))
		for _, i := range m.pos {
			hl[i] = true
		}
		xx := 2
		for col := 0; col < len(rr); col++ {
			if xx >= e.width {
				break
			}
			st := style
			if hl[col] {
				st = st.Bold(true)
			}
			ch := rr[col]
			e.drawCell(xx, yy, ch, st)
			xx += runeWidth(ch)
		}
	}

	// footer
	fy := e.height - 1
	e.fillRow(0, e.width, fy, hintStyle)
	e.putStr(1, fy, sprintf(" %d item(s)   ↑/↓ move   Enter open/expand   Esc cancel", len(p.matches)), hintStyle)

	// input cursor
	if px >= 0 && px < e.width {
		e.screen.ShowCursor(px, y)
	} else {
		e.screen.HideCursor()
	}
}
