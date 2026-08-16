package editor

import (
	"os"
	"path/filepath"
	"strings"

	"goat/syntax"
)

const tabStop = 8

type Pos struct{ Line, Col int }

// Tab is one open document.
type Tab struct {
	text       Text
	path       string
	name       string
	dirty      bool
	trailingNL bool
	cur        Pos
	destCol    int
	top        int
	left       int
	lastScroll Pos
	mark       *Pos
	edits      UndoStack
	lang       *syntax.Language
	hl         *syntax.Highlighter
}

func NewTab() *Tab {
	return &Tab{
		text: &lineSlice{lines: [][]rune{{}}},
		name: "New Buffer",
		hl:   syntax.NewHighlighter(nil),
	}
}

// OpenTab loads path into a new tab. Returns an error if the file can't be read.
func OpenTab(path string) (*Tab, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t := &Tab{
		text:       newText(data),
		trailingNL: len(data) > 0 && data[len(data)-1] == '\n',
	}
	abs, _ := filepath.Abs(path)
	t.path = abs
	t.name = filepath.Base(path)
	t.lang = syntax.Detect(t.path, t.firstLine())
	t.hl = syntax.NewHighlighter(t.lang)
	t.hl.SetLineCount(t.lineCount())
	t.invalidate(0)
	return t, nil
}

func (t *Tab) lineCount() int { return t.text.LineCount() }
func (t *Tab) line(i int) []rune {
	if i < 0 || i >= t.lineCount() {
		return nil
	}
	return t.text.Line(i)
}
func (t *Tab) firstLine() string { return string(t.line(0)) }

// saveTo writes the buffer to path and updates metadata.
func (t *Tab) saveTo(path string) error {
	var b strings.Builder
	for i := 0; i < t.lineCount(); i++ {
		b.WriteString(string(t.line(i)))
		if i < t.lineCount()-1 {
			b.WriteByte('\n')
		}
	}
	if t.trailingNL && t.lineCount() > 0 {
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return err
	}
	abs, _ := filepath.Abs(path)
	t.path = abs
	t.name = filepath.Base(path)
	t.dirty = false
	if t.lang == nil {
		t.lang = syntax.Detect(path, t.firstLine())
		t.hl = syntax.NewHighlighter(t.lang)
		t.hl.SetLineCount(t.lineCount())
		t.invalidate(0)
	}
	return nil
}

// close stops the background highlighter goroutine.
func (t *Tab) close() {
	if t.hl != nil {
		t.hl.Close()
		t.hl = nil
	}
}

func (t *Tab) invalidate(from int) {
	if t.hl != nil {
		t.hl.Invalidate(from, t.lineCount(), func(i int) []rune { return t.text.Line(i) })
	}
}

func (t *Tab) setDirty() {
	if !t.dirty {
		t.dirty = true
	}
}

// --- editing -------------------------------------------------------------

func (t *Tab) insertRunes(line, col int, s []rune) {
	o := &op{kind: opInsert, line: line, col: col, text: s, curBefore: t.cur}
	insertText(t.text, line, col, s)
	t.cur = Pos{line, col + len([]rune(string(s)))}
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(line)
}

func (t *Tab) insertRune(r rune) {
	o := &op{kind: opInsert, line: t.cur.Line, col: t.cur.Col, text: []rune{r}, curBefore: t.cur}
	insertText(t.text, t.cur.Line, t.cur.Col, o.text)
	t.cur.Col++
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(t.cur.Line)
}

func (t *Tab) insertNewline() {
	indent := t.leadingWhitespace(t.line(t.cur.Line))
	o := &op{kind: opInsert, line: t.cur.Line, col: t.cur.Col, text: []rune{'\n'}, curBefore: t.cur}
	insertText(t.text, t.cur.Line, t.cur.Col, []rune{'\n'})
	t.cur.Line++
	t.cur.Col = len(indent)
	t.destCol = t.cur.Col
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(t.cur.Line - 1)
	insertText(t.text, t.cur.Line, 0, indent)
}

func (t *Tab) insertTab() { t.insertRune('\t') }

func (t *Tab) backspace() {
	if t.cur.Col > 0 {
		rem := deleteText(t.text, t.cur.Line, t.cur.Col-1, 1)
		o := &op{kind: opDelete, line: t.cur.Line, col: t.cur.Col - 1, text: rem, curBefore: t.cur}
		t.cur.Col--
		o.curAfter = t.cur
		t.edits.push(o)
		t.setDirty()
		t.invalidate(t.cur.Line)
		return
	}
	if t.cur.Line > 0 {
		prev := t.line(t.cur.Line - 1)
		rem := deleteText(t.text, t.cur.Line-1, len(prev), 1)
		o := &op{kind: opDelete, line: t.cur.Line - 1, col: len(prev), text: rem, curBefore: t.cur}
		t.cur.Line--
		t.cur.Col = len(prev)
		t.destCol = t.cur.Col
		o.curAfter = t.cur
		t.edits.push(o)
		t.setDirty()
		t.invalidate(t.cur.Line)
	}
}

func (t *Tab) deleteForward() {
	l := t.line(t.cur.Line)
	if t.cur.Col < len(l) {
		rem := deleteText(t.text, t.cur.Line, t.cur.Col, 1)
		o := &op{kind: opDelete, line: t.cur.Line, col: t.cur.Col, text: rem, curBefore: t.cur}
		o.curAfter = t.cur
		t.edits.push(o)
		t.setDirty()
		t.invalidate(t.cur.Line)
		return
	}
	if t.cur.Line+1 < t.lineCount() {
		rem := deleteText(t.text, t.cur.Line, len(l), 1)
		o := &op{kind: opDelete, line: t.cur.Line, col: len(l), text: rem, curBefore: t.cur}
		o.curAfter = t.cur
		t.edits.push(o)
		t.setDirty()
		t.invalidate(t.cur.Line)
	}
}

func (t *Tab) undo() {
	o := t.edits.undo()
	if o == nil {
		return
	}
	t.applyInverse(o)
}

func (t *Tab) redo() {
	o := t.edits.redo()
	if o == nil {
		return
	}
	t.applyInverse(o.inverse())
}

func (t *Tab) applyInverse(o *op) {
	switch o.kind {
	case opInsert:
		deleteText(t.text, o.line, o.col, len([]rune(string(o.text))))
	case opDelete:
		insertText(t.text, o.line, o.col, o.text)
	case opReplace:
		deleteText(t.text, o.line, o.col, len([]rune(string(o.new))))
		insertText(t.text, o.line, o.col, o.text)
	case opLineRemove:
		t.text.InsertLine(o.line, o.text)
	case opLineInsert:
		t.text.RemoveLine(o.line)
	}
	t.cur = o.curBefore
	t.destCol = t.cur.Col
	t.setDirty()
	t.invalidate(o.line)
}

// --- cursor movement -----------------------------------------------------

func (t *Tab) moveLeft() {
	if t.cur.Col > 0 {
		t.cur.Col--
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
		t.cur.Col++
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
		t.clampCol()
	}
}

func (t *Tab) moveDown() {
	if t.cur.Line+1 < t.lineCount() {
		t.cur.Line++
		t.clampCol()
	}
}

func (t *Tab) clampCol() {
	l := len(t.line(t.cur.Line))
	if t.cur.Col > l {
		t.cur.Col = l
	}
	t.destCol = t.cur.Col
}

func (t *Tab) home() {
	t.cur.Col = 0
	t.destCol = 0
}

func (t *Tab) end() {
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
	if col < 0 {
		col = 0
	}
	if col > len(line) {
		col = len(line)
	}
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
	t.clampCol()
}

func (t *Tab) pgdn(viewH int) {
	for i := 0; i < viewH && t.cur.Line+1 < t.lineCount(); i++ {
		t.cur.Line++
	}
	t.clampCol()
}

// leadingWhitespace returns the indentation prefix of a line.
func (t *Tab) leadingWhitespace(l []rune) []rune {
	i := 0
	for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
		i++
	}
	out := make([]rune, i)
	copy(out, l[:i])
	return out
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
func displayCol(line []rune, col int) int {
	c := 0
	for i := 0; i < col && i < len(line); i++ {
		if line[i] == '\t' {
			c += tabStop - (c % tabStop)
		} else {
			c += runeWidth(line[i])
		}
	}
	return c
}

// lineLengthCells returns the full display width of a line.
func lineLengthCells(line []rune) int { return displayCol(line, len(line)) }
