package editor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"goat/syntax"
)

// maxOpenBytes caps the size of a file goat will load. The buffer holds runes
// (4 bytes each) plus a highlighter snapshot, so a much larger file would use
// several times its size in memory and re-lex too slowly to type in.
const maxOpenBytes = 64 << 20

// errBinary is returned when a file looks binary; editing it would corrupt it.
var errBinary = errors.New("binary file (contains NUL bytes)")

type Pos struct{ Line, Col int }

// Tab is one open document.
type Tab struct {
	text       Text
	path       string
	name       string
	dirty      bool
	trailingNL bool // the file ended with a newline
	readOnly   bool // the file is not writable by this process
	rawBytes   bool // the file was not valid UTF-8; raw bytes are escaped
	diskMod    time.Time
	diskSize   int64
	cur        Pos
	destCol    int
	top        int
	topSub     int // display row within the top line (soft wrap only)
	left       int
	lastScroll Pos
	lastW      int // viewport size at the last ensureVisible, to re-follow on resize
	lastH      int
	mark       *Pos
	edits      UndoStack
	savedRev   int // edits.rev at the last save; dirty == (edits.rev != savedRev) after undo/redo
	lang       *syntax.Language
	hl         *syntax.Highlighter
	cfg        *Config

	// Highlighting is snapshotted at most once per hlInterval instead of on
	// every keystroke; see flushHighlight.
	hlFrom    int
	hlPending bool
	hlAt      time.Time
}

func NewTab() *Tab { return newTabWith(DefaultConfig()) }

func newTabWith(cfg *Config) *Tab {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Tab{
		text: &lineSlice{lines: [][]rune{{}}},
		name: "New Buffer",
		hl:   syntax.NewHighlighter(nil),
		cfg:  cfg,
	}
}

// OpenTab loads path into a new tab. Returns an error if the file can't be
// read, is too large, or looks binary.
func OpenTab(path string) (*Tab, error) { return openTabWith(path, DefaultConfig()) }

func openTabWith(path string, cfg *Config) (*Tab, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	fi, err := os.Stat(path)
	if err == nil {
		if fi.IsDir() {
			return nil, errors.New("is a directory")
		}
		if fi.Size() > maxOpenBytes {
			return nil, fmt.Errorf("file is too large (%d MB; limit %d MB)", fi.Size()>>20, int64(maxOpenBytes)>>20)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, errBinary
	}
	t := &Tab{
		text:       newText(data),
		trailingNL: len(data) > 0 && data[len(data)-1] == '\n',
		cfg:        cfg,
	}
	t.rawBytes = hasRawBytes(t.text)
	abs, _ := filepath.Abs(path)
	t.path = abs
	t.name = filepath.Base(path)
	t.readOnly = !writable(path)
	t.noteDiskState()
	t.lang = syntax.Detect(t.path, t.firstLine())
	t.hl = syntax.NewHighlighter(t.lang)
	t.invalidate(0)
	return t, nil
}

// writable reports whether the file (or, for a new file, its directory) can be
// written, so the editor can warn before the user types a screenful of text.
func writable(path string) bool {
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		// A new file: what matters is whether the directory accepts one.
		return writableDir(filepath.Dir(path))
	}
	if err != nil {
		return true // unknown: let the save attempt report the real error
	}
	if !fi.Mode().IsRegular() {
		// Opening a FIFO for writing would block; don't probe it.
		return true
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err == nil {
		f.Close()
		return true
	}
	return !os.IsPermission(err)
}

// writableDir reports whether a file can be created in dir.
func writableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".goat-perm-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func (t *Tab) config() *Config {
	if t.cfg == nil {
		t.cfg = DefaultConfig()
	}
	return t.cfg
}

func (t *Tab) tabW() int {
	w := t.config().TabWidth
	if w < 1 {
		return 8
	}
	return w
}

func (t *Tab) lineCount() int { return t.text.LineCount() }
func (t *Tab) line(i int) []rune {
	if i < 0 || i >= t.lineCount() {
		return nil
	}
	return t.text.Line(i)
}
func (t *Tab) firstLine() string { return string(t.line(0)) }

// spans returns the highlight spans for a line; a tab without a highlighter
// (plain text, or already closed) renders unstyled.
func (t *Tab) spans(i int, line []rune) []syntax.Span {
	if t.hl == nil {
		return nil
	}
	return t.hl.Spans(i, line)
}

// highlightTooLarge reports whether highlighting was skipped for size.
func (t *Tab) highlightTooLarge() bool { return t.hl != nil && t.hl.TooLarge() }

// bytes renders the buffer exactly as it will be written to disk.
func (t *Tab) bytes() []byte {
	out := make([]byte, 0, t.lineCount()*32)
	for i := 0; i < t.lineCount(); i++ {
		out = appendEncoded(out, t.line(i))
		if i < t.lineCount()-1 {
			out = append(out, '\n')
		}
	}
	if t.trailingNL {
		out = append(out, '\n')
	}
	return out
}

// noteDiskState records the file's identity so a later save can tell whether
// something else changed it in the meantime.
func (t *Tab) noteDiskState() {
	if fi, err := os.Stat(t.path); err == nil {
		t.diskMod = fi.ModTime()
		t.diskSize = fi.Size()
		return
	}
	t.diskMod = time.Time{}
	t.diskSize = 0
}

// externallyChanged reports whether the file on disk differs from what was
// last read or written by this tab.
func (t *Tab) externallyChanged() bool {
	if t.path == "" || t.diskMod.IsZero() {
		return false
	}
	fi, err := os.Stat(t.path)
	if err != nil {
		return false // gone or unreadable; the save attempt will report it
	}
	return fi.Size() != t.diskSize || !fi.ModTime().Equal(t.diskMod)
}

// saveTo writes the buffer to path and updates metadata.
func (t *Tab) saveTo(path string) error {
	if err := writeFileAtomic(path, t.bytes()); err != nil {
		return err
	}
	abs, _ := filepath.Abs(path)
	t.path = abs
	t.name = filepath.Base(path)
	t.readOnly = false
	t.noteDiskState()
	t.savedRev = t.edits.rev
	t.edits.sealed = true
	t.dirty = false
	if t.lang == nil {
		if lang := syntax.Detect(path, t.firstLine()); lang != nil {
			t.lang = lang
			if t.hl != nil {
				t.hl.Close()
			}
			t.hl = syntax.NewHighlighter(t.lang)
			t.invalidate(0)
		}
	}
	return nil
}

// writeFileAtomic writes data to a temporary file in the same directory and
// renames it into place, so an interrupted save (crash, full disk) cannot
// leave a truncated file behind. The original file's mode is preserved, and a
// symlink is followed rather than replaced.
//
// The rename replaces the directory entry, so the file gets a new inode: a
// second hard link to the old inode keeps the previous content, and anything
// holding the old file open keeps reading it. That is the same trade-off vim
// makes by default, and it is what buys the crash safety.
func writeFileAtomic(path string, data []byte) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	dir := filepath.Dir(target)
	mode := os.FileMode(0644)
	if fi, err := os.Stat(target); err == nil {
		mode = fi.Mode().Perm()
		if !fi.Mode().IsRegular() {
			// A device, FIFO or similar: renaming over it would replace it, so
			// write in place instead.
			return os.WriteFile(target, data, mode)
		}
	}
	f, err := os.CreateTemp(dir, ".goat-*.tmp")
	if err != nil {
		// An unwritable directory (read-only mount, restrictive perms) still
		// allows an in-place write when the file itself is writable.
		return os.WriteFile(target, data, mode)
	}
	tmp := f.Name()
	cleanup := func() {
		f.Close()
		os.Remove(tmp)
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Chmod(mode); err != nil && !errors.Is(err, os.ErrInvalid) {
		// Best effort: Windows and some filesystems reject chmod.
		_ = err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return err
	}
	syncDir(dir)
	return nil
}

// syncDir flushes a directory entry so the rename survives a power loss. Not
// supported everywhere; failures are not fatal.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	d.Close()
}

// close stops the background highlighter goroutine.
func (t *Tab) close() {
	if t.hl != nil {
		t.hl.Close()
		t.hl = nil
	}
}

// invalidate marks the buffer dirty for highlighting from line `from` down.
// The snapshot itself is taken later by flushHighlight, so a burst of
// keystrokes costs one snapshot instead of one per key.
func (t *Tab) invalidate(from int) {
	if t.hl == nil {
		return
	}
	if from < 0 {
		from = 0
	}
	if !t.hlPending || from < t.hlFrom {
		t.hlFrom = from
	}
	t.hlPending = true
}

// hlInterval is the shortest gap between two highlighter snapshots.
const hlInterval = 25 * time.Millisecond

// flushHighlight hands the highlighter a snapshot if one is due. Called from
// the draw loop.
func (t *Tab) flushHighlight(now time.Time) bool {
	if !t.hlPending || t.hl == nil {
		return false
	}
	if !t.hlAt.IsZero() && now.Sub(t.hlAt) < hlInterval {
		return true // still pending; ask the caller to come back
	}
	t.hlPending = false
	t.hlAt = now
	t.hl.Invalidate(t.hlFrom, t.lineCount(), func(i int) []rune { return t.text.Line(i) })
	t.hlFrom = 0
	return false
}

// setDirty marks the tab modified. Revision bookkeeping for undo/redo lives
// in UndoStack.push, which is called for every recorded mutation.
func (t *Tab) setDirty() {
	if !t.dirty {
		t.dirty = true
	}
}

// --- selection -----------------------------------------------------------

// selRange returns the normalized selection [sLine,sCol,eLine,eCol], or
// [-1,0,0,0] when nothing is marked. Bounds are clamped to the buffer, so a
// mark left behind by an edit cannot address a line that no longer exists.
func (t *Tab) selRange() [4]int {
	if t.mark == nil {
		return [4]int{-1, 0, 0, 0}
	}
	a, b := *t.mark, t.cur
	if b.Line < a.Line || (b.Line == a.Line && b.Col < a.Col) {
		a, b = b, a
	}
	a = t.clampPos(a)
	b = t.clampPos(b)
	return [4]int{a.Line, a.Col, b.Line, b.Col}
}

func (t *Tab) clampPos(p Pos) Pos {
	if p.Line < 0 {
		p.Line = 0
	}
	if p.Line >= t.lineCount() {
		p.Line = t.lineCount() - 1
	}
	p.Col = clampCol(t.line(p.Line), p.Col)
	return p
}

// clampCursor keeps the cursor inside the buffer after an edit or undo.
func (t *Tab) clampCursor() {
	t.cur = t.clampPos(t.cur)
	if t.mark != nil {
		m := t.clampPos(*t.mark)
		t.mark = &m
	}
}

func (t *Tab) hasSelection() bool {
	if t.mark == nil {
		return false
	}
	s := t.selRange()
	return s[0] != s[2] || s[1] != s[3]
}

// deleteRegion removes the rune range [sLine:sCol .. eLine:eCol).
func (t *Tab) deleteRegion(sLine, sCol, eLine, eCol int) []rune {
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
	if total <= 0 {
		return nil
	}
	return deleteText(t.text, sLine, sCol, total)
}

// deleteSelection removes the marked region as one recorded edit and returns
// the removed runes. The caller is expected to have opened a transaction when
// the deletion is part of a larger action.
func (t *Tab) deleteSelection() []rune {
	if t.mark == nil {
		return nil
	}
	sel := t.selRange()
	t.mark = nil
	if sel[0] < 0 || (sel[0] == sel[2] && sel[1] == sel[3]) {
		return nil
	}
	before := t.cur
	removed := t.deleteRegion(sel[0], sel[1], sel[2], sel[3])
	t.cur = Pos{sel[0], sel[1]}
	t.destCol = t.cur.Col
	if len(removed) == 0 {
		return nil
	}
	o := &op{kind: opDelete, line: sel[0], col: sel[1], text: removed, curBefore: before, curAfter: t.cur}
	t.edits.push(o)
	t.setDirty()
	t.invalidate(sel[0])
	return removed
}

// --- editing -------------------------------------------------------------

func (t *Tab) insertRunes(line, col int, s []rune) {
	if len(s) == 0 {
		return
	}
	if t.hasSelection() {
		t.edits.begin()
		defer t.edits.end()
		t.deleteSelection()
		line, col = t.cur.Line, t.cur.Col
	} else {
		t.mark = nil
	}
	line = clampLine(t, line)
	col = clampCol(t.line(line), col)
	o := &op{kind: opInsert, line: line, col: col, text: s, curBefore: t.cur}
	insertText(t.text, line, col, s)
	t.cur = endOfInsert(line, col, s)
	t.destCol = t.cur.Col
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(line)
	t.clampCursor()
}

func clampLine(t *Tab, line int) int {
	if line < 0 {
		return 0
	}
	if line >= t.lineCount() {
		return t.lineCount() - 1
	}
	return line
}

// endOfInsert returns the cursor position just after inserting s at line:col:
// at the end of the last inserted line when s contains newlines.
func endOfInsert(line, col int, s []rune) Pos {
	lastNL := -1
	nl := 0
	for i, r := range s {
		if r == '\n' {
			lastNL = i
			nl++
		}
	}
	if lastNL < 0 {
		return Pos{line, col + len(s)}
	}
	return Pos{line + nl, len(s) - lastNL - 1}
}

func (t *Tab) insertRune(r rune) {
	if t.hasSelection() {
		t.edits.begin()
		defer t.edits.end()
		t.deleteSelection()
	}
	t.mark = nil
	line := clampLine(t, t.cur.Line)
	col := clampCol(t.line(line), t.cur.Col)
	o := &op{kind: opInsert, line: line, col: col, text: []rune{r}, curBefore: Pos{line, col}}
	insertText(t.text, line, col, o.text)
	t.cur = Pos{line, col + 1}
	t.destCol = t.cur.Col
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(line)
}

func (t *Tab) insertNewline() {
	if t.hasSelection() {
		t.edits.begin()
		defer t.edits.end()
		t.deleteSelection()
	}
	t.mark = nil
	line := clampLine(t, t.cur.Line)
	col := clampCol(t.line(line), t.cur.Col)
	var indent []rune
	if t.config().AutoIndent {
		indent = leadingWhitespace(t.line(line))
		// Never carry more indentation than the text that precedes the split.
		if len(indent) > col {
			indent = indent[:col]
		}
	}
	// Record the newline and the copied indent as a single op so undo removes
	// both, leaving no stray whitespace.
	text := append(append([]rune{}, '\n'), indent...)
	o := &op{kind: opInsert, line: line, col: col, text: text, curBefore: Pos{line, col}}
	insertText(t.text, line, col, text)
	t.cur = Pos{line + 1, len(indent)}
	t.destCol = t.cur.Col
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(line)
}

// insertTab inserts one indent step, or indents the selected lines.
func (t *Tab) insertTab() {
	if t.selectionSpansLines() {
		t.indentSelection(1)
		return
	}
	cfg := t.config()
	if !cfg.ExpandTab {
		t.insertRune('\t')
		return
	}
	line := clampLine(t, t.cur.Line)
	col := clampCol(t.line(line), t.cur.Col)
	n := cfg.TabWidth - displayCol(t.line(line), col, t.tabW())%cfg.TabWidth
	if n <= 0 {
		n = cfg.TabWidth
	}
	spaces := make([]rune, n)
	for i := range spaces {
		spaces[i] = ' '
	}
	t.insertRunes(line, col, spaces)
}

// selectionSpansLines reports whether the selection covers more than one line,
// i.e. Tab should indent a block rather than replace the selection.
func (t *Tab) selectionSpansLines() bool {
	if !t.hasSelection() {
		return false
	}
	s := t.selRange()
	return s[0] != s[2]
}

// indentSelection shifts the selected lines by dir indent steps (+1 indent,
// -1 dedent) as a single undoable action.
func (t *Tab) indentSelection(dir int) {
	s := t.selRange()
	first, last := s[0], s[2]
	if first < 0 {
		first, last = t.cur.Line, t.cur.Line
	}
	// A selection ending at column 0 does not really include that last line.
	if last > first && s[3] == 0 {
		last--
	}
	cfg := t.config()
	unit := cfg.indentUnit()
	t.edits.begin()
	changed := false
	for l := first; l <= last && l < t.lineCount(); l++ {
		line := t.line(l)
		if dir > 0 {
			if len(line) == 0 {
				continue // don't create whitespace-only lines
			}
			o := &op{kind: opInsert, line: l, col: 0, text: unit, curBefore: t.cur, curAfter: t.cur}
			insertText(t.text, l, 0, unit)
			t.edits.push(o)
			changed = true
			continue
		}
		n := dedentWidth(line, cfg.TabWidth)
		if n == 0 {
			continue
		}
		removed := deleteText(t.text, l, 0, n)
		o := &op{kind: opDelete, line: l, col: 0, text: removed, curBefore: t.cur, curAfter: t.cur}
		t.edits.push(o)
		changed = true
	}
	t.edits.end()
	if !changed {
		return
	}
	t.setDirty()
	t.invalidate(first)
	// Keep the selection over the same lines, and the cursor on its line.
	if t.mark != nil {
		m := *t.mark
		m.Col = clampCol(t.line(clampLine(t, m.Line)), m.Col)
		if m.Line == first {
			m.Col = 0
		}
		t.mark = &m
	}
	if t.cur.Line >= first && t.cur.Line <= last {
		t.cur.Col = clampCol(t.line(clampLine(t, t.cur.Line)), t.cur.Col)
	}
	t.clampCursor()
}

// dedentWidth returns how many leading runes make up one indent step.
func dedentWidth(line []rune, tabWidth int) int {
	if len(line) == 0 {
		return 0
	}
	if line[0] == '\t' {
		return 1
	}
	n := 0
	for n < len(line) && n < tabWidth && line[n] == ' ' {
		n++
	}
	return n
}

// dedent removes one indent step from the current line or selection.
func (t *Tab) dedent() { t.indentSelection(-1) }

func (t *Tab) backspace() {
	if t.hasSelection() {
		t.deleteSelection()
		return
	}
	t.mark = nil
	line := clampLine(t, t.cur.Line)
	col := clampCol(t.line(line), t.cur.Col)
	if col > 0 {
		from := prevClusterCol(t.line(line), col)
		n := col - from
		rem := deleteText(t.text, line, from, n)
		o := &op{kind: opDelete, line: line, col: from, text: rem, curBefore: Pos{line, col}}
		t.cur = Pos{line, from}
		t.destCol = t.cur.Col
		o.curAfter = t.cur
		t.edits.push(o)
		t.setDirty()
		t.invalidate(line)
		return
	}
	if line > 0 {
		prev := len(t.line(line - 1))
		rem := deleteText(t.text, line-1, prev, 1)
		o := &op{kind: opDelete, line: line - 1, col: prev, text: rem, curBefore: Pos{line, col}}
		t.cur = Pos{line - 1, prev}
		t.destCol = t.cur.Col
		o.curAfter = t.cur
		t.edits.push(o)
		t.setDirty()
		t.invalidate(t.cur.Line)
	}
}

func (t *Tab) deleteForward() {
	if t.hasSelection() {
		t.deleteSelection()
		return
	}
	t.mark = nil
	line := clampLine(t, t.cur.Line)
	col := clampCol(t.line(line), t.cur.Col)
	l := t.line(line)
	if col < len(l) {
		end := nextClusterCol(l, col)
		rem := deleteText(t.text, line, col, end-col)
		o := &op{kind: opDelete, line: line, col: col, text: rem, curBefore: Pos{line, col}, curAfter: Pos{line, col}}
		t.cur = Pos{line, col}
		t.edits.push(o)
		t.setDirty()
		t.invalidate(line)
		return
	}
	if line+1 < t.lineCount() {
		rem := deleteText(t.text, line, len(l), 1)
		o := &op{kind: opDelete, line: line, col: len(l), text: rem, curBefore: t.cur, curAfter: t.cur}
		t.edits.push(o)
		t.setDirty()
		t.invalidate(line)
	}
}

func (t *Tab) undo() {
	ops := t.edits.undoGroup()
	if len(ops) == 0 {
		return
	}
	for _, o := range ops {
		t.applyInverse(o)
	}
	t.mark = nil
	t.clampCursor()
	t.dirty = t.edits.rev != t.savedRev
}

func (t *Tab) redo() {
	ops := t.edits.redoGroup()
	if len(ops) == 0 {
		return
	}
	for _, o := range ops {
		t.applyInverse(o.inverse())
	}
	t.mark = nil
	t.clampCursor()
	t.dirty = t.edits.rev != t.savedRev
}

func (t *Tab) applyInverse(o *op) {
	switch o.kind {
	case opInsert:
		deleteText(t.text, o.line, o.col, len(o.text))
	case opDelete:
		insertText(t.text, o.line, o.col, o.text)
	case opReplace:
		deleteText(t.text, o.line, o.col, len(o.new))
		insertText(t.text, o.line, o.col, o.text)
	case opLineRemove:
		t.text.InsertLine(o.line, cloneRunes(o.text))
	case opLineInsert:
		t.text.RemoveLine(o.line)
	}
	t.cur = o.curBefore
	t.destCol = t.cur.Col
	t.invalidate(o.line)
}

// --- cursor movement -----------------------------------------------------

func (t *Tab) moveLeft() {
	if t.cur.Col > 0 {
		t.cur.Col = prevClusterCol(t.line(t.cur.Line), clampCol(t.line(t.cur.Line), t.cur.Col))
		t.destCol = t.cur.Col
		return
	}
	if t.cur.Line > 0 {
		t.cur.Line--
		t.cur.Col = len(t.line(t.cur.Line))
	}
	t.destCol = t.cur.Col
}

func (t *Tab) moveRight() {
	l := t.line(t.cur.Line)
	if t.cur.Col < len(l) {
		t.cur.Col = nextClusterCol(l, t.cur.Col)
		t.destCol = t.cur.Col
		return
	}
	if t.cur.Line+1 < t.lineCount() {
		t.cur.Line++
		t.cur.Col = 0
	}
	t.destCol = t.cur.Col
}

func (t *Tab) moveUp() {
	if t.cur.Line > 0 {
		t.cur.Line--
		t.setColToDest()
	}
}

func (t *Tab) moveDown() {
	if t.cur.Line+1 < t.lineCount() {
		t.cur.Line++
		t.setColToDest()
	}
}

// setColToDest places the cursor at the saved destination column on the
// current line, clamped to its length, so vertical movement keeps the column.
func (t *Tab) setColToDest() {
	l := t.line(t.cur.Line)
	if t.destCol > len(l) {
		t.cur.Col = len(l)
		return
	}
	t.cur.Col = clusterStart(l, t.destCol)
}

func (t *Tab) home() {
	t.cur.Col = 0
	t.destCol = 0
}

func (t *Tab) end() {
	t.cur.Col = len(t.line(t.cur.Line))
	t.destCol = t.cur.Col
}

// bufStart moves to the start of the buffer.
func (t *Tab) bufStart() {
	t.cur.Line = 0
	t.cur.Col = 0
	t.destCol = 0
}

// bufEnd moves to the end of the buffer.
func (t *Tab) bufEnd() {
	t.cur.Line = t.lineCount() - 1
	t.cur.Col = len(t.line(t.cur.Line))
	t.destCol = t.cur.Col
}

func (t *Tab) wordLeft() {
	for t.cur.Col > 0 && !isWord(t.runAt(t.cur.Line, t.cur.Col-1)) {
		t.cur.Col--
	}
	for t.cur.Col > 0 && isWord(t.runAt(t.cur.Line, t.cur.Col-1)) {
		t.cur.Col--
	}
	t.destCol = t.cur.Col
}

func (t *Tab) wordRight() {
	l := t.line(t.cur.Line)
	for t.cur.Col < len(l) && isWord(t.runAt(t.cur.Line, t.cur.Col)) {
		t.cur.Col++
	}
	for t.cur.Col < len(l) && !isWord(t.runAt(t.cur.Line, t.cur.Col)) {
		t.cur.Col++
	}
	t.destCol = t.cur.Col
}

func (t *Tab) runAt(line, col int) rune {
	l := t.line(line)
	if col < 0 || col >= len(l) {
		return 0
	}
	return l[col]
}

func isWord(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// wordRange returns the rune-column span [start, end) of the word containing
// col. When col is not on a word character, both bounds equal col.
func wordRange(line []rune, col int) (int, int) {
	if len(line) == 0 {
		return col, col
	}
	col = clampCol(line, col)
	start, end := col, col
	for start > 0 && isWord(line[start-1]) {
		start--
	}
	for end < len(line) && isWord(line[end]) {
		end++
	}
	return start, end
}

func (t *Tab) pgup(viewH int) {
	for i := 0; i < viewH && t.cur.Line > 0; i++ {
		t.cur.Line--
	}
	t.setColToDest()
}

func (t *Tab) pgdn(viewH int) {
	for i := 0; i < viewH && t.cur.Line+1 < t.lineCount(); i++ {
		t.cur.Line++
	}
	t.setColToDest()
}

// leadingWhitespace returns a copy of the line's indentation prefix.
func leadingWhitespace(l []rune) []rune {
	i := 0
	for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
		i++
	}
	return cloneRunes(l[:i])
}

// selectMove starts a selection at the cursor if none is active, then moves.
func (t *Tab) selectMove(fn func()) {
	if t.mark == nil {
		m := t.cur
		t.mark = &m
	}
	fn()
}

// toggleMark starts a mark at the cursor, or clears an existing one.
func (t *Tab) toggleMark() {
	if t.mark == nil {
		m := t.cur
		t.mark = &m
	} else {
		t.mark = nil
	}
}

// displayCol returns the terminal column (with tab expansion) of rune col.
func displayCol(line []rune, col, tabW int) int {
	if tabW < 1 {
		tabW = 8
	}
	c := 0
	for i := 0; i < col && i < len(line); {
		if line[i] == '\t' {
			c += tabW - (c % tabW)
			i++
			continue
		}
		end, w := clusterEnd(line, i)
		if end > col {
			// col falls inside a cluster: it is a boundary for display purposes.
			break
		}
		c += w
		i = end
	}
	return c
}
