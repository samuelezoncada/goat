package editor

import "strings"

// Text is the editable text store. The concrete implementation is a slice of
// lines; it is exposed through this interface so a rope/piece-table can be
// swapped in later without touching the editor.
type Text interface {
	LineCount() int
	Line(i int) []rune
	Set(i int, line []rune)
	InsertLine(i int, line []rune)
	RemoveLine(i int) []rune
}

// lineSlice is the default Text implementation.
type lineSlice struct {
	lines [][]rune
}

func newText(content []byte) *lineSlice {
	text := &lineSlice{}
	text.lines = make([][]rune, 0, 16)
	for _, ln := range strings.Split(string(content), "\n") {
		text.lines = append(text.lines, []rune(ln))
	}
	return text
}

func (t *lineSlice) LineCount() int         { return len(t.lines) }
func (t *lineSlice) Line(i int) []rune      { return t.lines[i] }
func (t *lineSlice) Set(i int, line []rune) { t.lines[i] = line }
func (t *lineSlice) InsertLine(i int, line []rune) {
	t.lines = append(t.lines, nil)
	copy(t.lines[i+1:], t.lines[i:])
	t.lines[i] = line
}
func (t *lineSlice) RemoveLine(i int) []rune {
	rem := t.lines[i]
	copy(t.lines[i:], t.lines[i+1:])
	t.lines = t.lines[:len(t.lines)-1]
	return rem
}

// insertText inserts runes (possibly containing newlines) at line:col.
func insertText(t Text, line, col int, s []rune) {
	if len(s) == 0 {
		return
	}
	parts := strings.Split(string(s), "\n")
	rr := [][]rune{}
	for _, p := range parts {
		rr = append(rr, []rune(p))
	}
	if len(rr) == 1 {
		l := t.Line(line)
		nl := make([]rune, 0, len(l)+len(rr[0]))
		nl = append(nl, l[:col]...)
		nl = append(nl, rr[0]...)
		nl = append(nl, l[col:]...)
		t.Set(line, nl)
		return
	}
	head := t.Line(line)
	tail := append([]rune{}, head[col:]...)
	head = append(head[:col], rr[0]...)
	t.Set(line, head)
	at := line + 1
	for i := 1; i < len(rr); i++ {
		if i == len(rr)-1 {
			l := append(rr[i], tail...)
			t.InsertLine(at, l)
		} else {
			t.InsertLine(at, rr[i])
		}
		at++
	}
}

// deleteText removes n runes starting at line:col and returns them.
func deleteText(t Text, line, col, n int) []rune {
	var out []rune
	cur, c := line, col
	for n > 0 {
		l := t.Line(cur)
		if c < len(l) {
			take := n
			if take > len(l)-c {
				take = len(l) - c
			}
			out = append(out, l[c:c+take]...)
			nl := append([]rune{}, l[:c]...)
			nl = append(nl, l[c+take:]...)
			t.Set(cur, nl)
			n -= take
			if n == 0 {
				break
			}
		}
		if cur+1 >= t.LineCount() {
			break
		}
		out = append(out, '\n')
		n--
		next := t.Line(cur + 1)
		merged := append(t.Line(cur), next...)
		t.Set(cur, merged)
		t.RemoveLine(cur + 1)
	}
	return out
}
