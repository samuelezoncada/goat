package editor

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

// indexEntry is one file (or unexpanded heavy directory) known to the picker.
type indexEntry struct {
	path     string // absolute
	rel      string // relative to root, used for display and matching
	relLower string // pre-lowered, so filtering allocates nothing per keystroke
	ascii    bool   // rel is pure ASCII: byte offsets are rune columns
	isDir    bool   // heavy dir shown as expandable, not walked into
}

// match is a single picker result.
type match struct {
	path  string
	rel   string
	score int
	pos   []int // matched rune indices within rel
	isDir bool
}

// skipDirs are directories never indexed implicitly; they appear as
// expandable entries and are only walked when the user opens them. Inside a
// git repository the ignore rules take over and this list is only a fallback.
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

// maxIndexFiles bounds the picker index so an accidental ^P at $HOME cannot
// grow without limit.
const maxIndexFiles = 200000

// fileIndex is the project file list backing the picker. It is built in the
// background and cached on the Editor, so ^P opens instantly and re-opening
// does not re-walk the tree.
type fileIndex struct {
	root     string
	entries  []indexEntry
	expanded map[string]bool // heavy dirs the user chose to index
	ready    bool
	building bool
	seq      uint64 // generation, so a stale build result is ignored
	err      string
}

// indexDone carries a finished background index build into the event loop.
type indexDone struct {
	seq     uint64
	entries []indexEntry
	err     string
}

// Picker is the Ctrl+P fuzzy file finder.
type Picker struct {
	e       *Editor
	input   []rune
	pos     int
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
			e.errorf("no working directory")
			return
		}
	}
	if e.fileIndex == nil || e.fileIndex.root != root {
		e.fileIndex = &fileIndex{root: root, expanded: map[string]bool{}}
	}
	e.picker = &Picker{e: e}
	e.mode = ModePicker
	e.clearMsg()
	e.picker.refilter()
	if !e.fileIndex.ready && !e.fileIndex.building {
		e.startIndex()
	}
}

// startIndex kicks off a background walk of the project tree. The UI stays
// responsive and shows results as soon as they arrive.
func (e *Editor) startIndex() {
	idx := e.fileIndex
	if idx == nil || idx.building {
		return
	}
	idx.building = true
	idx.seq++
	seq := idx.seq
	root := idx.root
	expanded := make(map[string]bool, len(idx.expanded))
	for k, v := range idx.expanded {
		expanded[k] = v
	}
	go func() {
		entries, err := buildFileIndex(root, expanded)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		e.post(indexDone{seq: seq, entries: entries, err: msg})
	}()
}

// onIndexDone installs a finished index and refreshes the open picker.
func (e *Editor) onIndexDone(d indexDone) {
	idx := e.fileIndex
	if idx == nil || d.seq != idx.seq {
		return // superseded by a newer build
	}
	idx.building = false
	idx.ready = true
	idx.entries = d.entries
	idx.err = d.err
	if e.picker != nil {
		e.picker.refilter()
	}
}

// InvalidateIndex drops the cached file list, so the next ^P re-walks the tree.
func (e *Editor) InvalidateIndex() {
	if e.fileIndex != nil {
		e.fileIndex.ready = false
	}
}

// buildFileIndex lists the project's files. In a git work tree it asks git,
// which applies .gitignore, .git/info/exclude and the global ignore file
// exactly; otherwise it walks the tree skipping known-heavy directories.
func buildFileIndex(root string, expanded map[string]bool) ([]indexEntry, error) {
	entries, err := gitFileIndex(root)
	if err != nil || entries == nil {
		entries = walkFileIndex(root, expanded)
	}
	sortEntries(entries)
	return entries, nil
}

// gitFileIndex returns the tracked and untracked-but-not-ignored files, or nil
// when root is not a git work tree (or git is unavailable).
func gitFileIndex(root string) ([]indexEntry, error) {
	exe, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "-C", root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var entries []indexEntry
	for _, b := range bytes.Split(out, []byte{0}) {
		if len(b) == 0 {
			continue
		}
		rel := filepath.FromSlash(string(b))
		entries = append(entries, newIndexEntry(root, rel, false))
		if len(entries) >= maxIndexFiles {
			break
		}
	}
	if entries == nil {
		// A git repo with no files at all: still a valid answer.
		return []indexEntry{}, nil
	}
	return entries, nil
}

func walkFileIndex(root string, expanded map[string]bool) []indexEntry {
	var entries []indexEntry
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(entries) >= maxIndexFiles {
			return filepath.SkipAll
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] && !expanded[path] {
				rel, err := filepath.Rel(root, path)
				if err == nil {
					entries = append(entries, newIndexEntry(root, rel, true))
				}
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		entries = append(entries, newIndexEntry(root, rel, false))
		return nil
	})
	return entries
}

// sortEntries orders the index by lowercased relative path.
func sortEntries(entries []indexEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].relLower < entries[j].relLower })
}

func newIndexEntry(root, rel string, isDir bool) indexEntry {
	return indexEntry{
		path:     filepath.Join(root, rel),
		rel:      rel,
		relLower: strings.ToLower(rel),
		ascii:    isASCII(rel),
		isDir:    isDir,
	}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// SetRoot sets the directory the file picker searches (abs). Empty keeps the
// launch working directory as the root.
func (e *Editor) SetRoot(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		e.root = abs
		e.fileIndex = nil
	}
}

func (e *Editor) cancelPicker() {
	e.picker = nil
	e.mode = ModeNormal
	e.screen.HideCursor()
}

// expandDir indexes a heavy directory the user opened, making its files
// searchable.
func (p *Picker) expandDir(m match) {
	idx := p.e.fileIndex
	if idx == nil {
		return
	}
	idx.expanded[m.path] = true
	// Drop the placeholder entry and re-index in the background.
	out := idx.entries[:0]
	for _, f := range idx.entries {
		if f.path != m.path {
			out = append(out, f)
		}
	}
	idx.entries = out
	p.sel, p.top = 0, 0
	p.refilter()
	p.e.startIndex()
}

// fuzzyScoreASCII is fuzzyScoreLower for all-ASCII input: it walks bytes
// directly, so filtering a large index allocates nothing except the position
// list of an actual match.
func fuzzyScoreASCII(q, s, sLower string) (int, []int, bool) {
	if len(q) == 0 {
		return 0, nil, true
	}
	if len(q) > len(s) {
		return 0, nil, false
	}
	var matched []int
	qi, score, prev := 0, 0, -2
	base := 0
	for i := 0; i < len(sLower); i++ {
		if sLower[i] != q[qi] {
			continue
		}
		if matched == nil {
			matched = make([]int, 0, len(q))
		}
		matched = append(matched, i)
		score++
		if i == prev+1 {
			score += 8 // consecutive run
		}
		if i == 0 || isBoundaryByte(s[i-1]) {
			score += 5 // path/word boundary start
		} else if isLowerByte(s[i-1]) && isUpperByte(s[i]) {
			score += 5 // camelCase transition
		}
		prev = i
		qi++
		if qi == len(q) {
			for j := 0; j < len(s); j++ {
				if s[j] == '/' || s[j] == byte(filepath.Separator) {
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

func isBoundaryByte(b byte) bool {
	return b == '/' || b == byte(filepath.Separator) || b == '_' || b == '-' || b == '.' || b == ' '
}
func isLowerByte(b byte) bool { return b >= 'a' && b <= 'z' }
func isUpperByte(b byte) bool { return b >= 'A' && b <= 'Z' }

// fuzzyScore scores a case-insensitive subsequence match of query against s.
// query must already be lowercased and sLower must be strings.ToLower(s);
// both are precomputed by the caller so filtering does not allocate per
// candidate. Returns the score, the matched rune indices, and whether it
// matched.
func fuzzyScoreLower(q []rune, s, sLower string) (int, []int, bool) {
	if len(q) == 0 {
		return 0, nil, true
	}
	rs := []rune(s)
	sl := []rune(sLower)
	if len(q) > len(sl) {
		return 0, nil, false
	}
	var matched []int
	qi := 0
	score := 0
	prev := -2
	for i, r := range sl {
		if r != q[qi] {
			continue
		}
		if matched == nil {
			matched = make([]int, 0, len(q))
		}
		matched = append(matched, i)
		score++
		if i == prev+1 {
			score += 8 // consecutive run
		}
		if i == 0 || (i-1 < len(rs) && isBoundary(rs[i-1])) {
			score += 5 // path/word boundary start
		} else if i > 0 && i < len(rs) && unicode.IsLower(rs[i-1]) && unicode.IsUpper(rs[i]) {
			score += 5 // camelCase transition
		}
		prev = i
		qi++
		if qi == len(q) {
			base := 0
			for j, r := range rs {
				if r == '/' || r == filepath.Separator {
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

func isBoundary(r rune) bool {
	return r == '/' || r == filepath.Separator || r == '_' || r == '-' || r == '.' || r == ' '
}

// fuzzyScore is the convenience form used by tests and one-off calls.
func fuzzyScore(query, s string) (int, []int, bool) {
	return fuzzyScoreLower([]rune(strings.ToLower(query)), s, strings.ToLower(s))
}

// refilter applies the current query, ordering by MRU then score then name.
func (p *Picker) refilter() {
	idx := p.e.fileIndex
	p.matches = p.matches[:0]
	if idx == nil {
		return
	}
	query := strings.TrimSpace(string(p.input))
	rank := p.e.recentRank()
	if query == "" {
		for _, f := range idx.entries {
			p.matches = append(p.matches, match{path: f.path, rel: f.rel, isDir: f.isDir})
		}
		sort.SliceStable(p.matches, func(i, j int) bool {
			ri, rj := rank(p.matches[i].path), rank(p.matches[j].path)
			if ri != rj {
				return ri < rj
			}
			return false // entries are already sorted by name
		})
	} else {
		lower := strings.ToLower(query)
		q := []rune(lower)
		qASCII := isASCII(lower)
		for _, f := range idx.entries {
			var (
				sc  int
				pos []int
				ok  bool
			)
			if qASCII && f.ascii {
				sc, pos, ok = fuzzyScoreASCII(lower, f.rel, f.relLower)
			} else {
				sc, pos, ok = fuzzyScoreLower(q, f.rel, f.relLower)
			}
			if ok {
				p.matches = append(p.matches, match{path: f.path, rel: f.rel, score: sc, pos: pos, isDir: f.isDir})
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
			return p.matches[i].rel < p.matches[j].rel
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
	case tcell.KeyCtrlR:
		// re-index on demand, e.g. after files changed outside the editor
		e.InvalidateIndex()
		e.startIndex()
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
	case tcell.KeyUp, tcell.KeyCtrlP:
		if p.sel > 0 {
			p.sel--
		}
	case tcell.KeyDown, tcell.KeyCtrlN:
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
		if mod&tcell.ModMeta != 0 {
			// ⌘V pastes into the filter box; other ⌘ keys are ignored.
			if ev.Rune() == 'v' {
				e.pickerInsert(e.clip)
			}
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
		p.expandDir(m)
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
	px := e.drawInputLine(1, y, "> ", p.input, p.pos, inputStyle)

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
			label += string(filepath.Separator)
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
			w := runeWidth(ch)
			if w == 0 {
				w = 1
			}
			xx += w
		}
	}

	// footer
	fy := e.height - 1
	e.fillRow(0, e.width, fy, hintStyle)
	status := sprintf(" %d item(s)", len(p.matches))
	if idx := e.fileIndex; idx != nil {
		switch {
		case idx.building:
			status = " indexing..."
		case idx.err != "":
			status = sprintf(" %d item(s)  (index: %s)", len(p.matches), idx.err)
		}
	}
	e.putStr(1, fy, status+"   ↑/↓ move   Enter open/expand   ^R reindex   Esc cancel", hintStyle)

	// input cursor
	if px >= 0 && px < e.width {
		e.screen.ShowCursor(px, y)
	} else {
		e.screen.HideCursor()
	}
}

// pickerInsert inserts pasted text into the picker query.
func (e *Editor) pickerInsert(rs []rune) {
	p := e.picker
	if p == nil {
		return
	}
	clean := make([]rune, 0, len(rs))
	for _, r := range rs {
		if r >= 0x20 && r != 0x7f {
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
	p.sel, p.top = 0, 0
	p.refilter()
}
