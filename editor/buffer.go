package editor

import (
	"bytes"
	"unicode/utf8"
)

// Text is the editable text store. The concrete implementation is a slice of
// lines; it is exposed through this interface so a rope/piece-table can be
// swapped in later without touching the editor.
//
// Implementations must keep at least one line at all times, so line 0 is
// always addressable.
type Text interface {
	LineCount() int
	Line(i int) []rune
	Set(i int, line []rune)
	InsertLine(i int, line []rune)
	RemoveLine(i int) []rune
}

// rawByteBase maps a byte that is not valid UTF-8 onto a rune in Supplementary
// Private Use Area-A. Decoding a file maps every invalid byte b to
// rawByteBase+b and saving maps it back, so goat round-trips files in unknown
// encodings (Latin-1, mixed, truncated UTF-8) byte for byte instead of
// replacing the bytes it cannot decode with U+FFFD.
const rawByteBase = 0xF0000

// isRawByte reports whether r is an escaped raw byte produced by decodeRunes.
func isRawByte(r rune) bool { return r >= rawByteBase && r <= rawByteBase+0xFF }

// decodeRunes decodes UTF-8, escaping every invalid byte as rawByteBase+byte.
func decodeRunes(b []byte) []rune {
	// Fast path: valid UTF-8 needs no escaping.
	if utf8.Valid(b) {
		return []rune(string(b))
	}
	out := make([]rune, 0, len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			out = append(out, rawByteBase+rune(b[i]))
			i++
			continue
		}
		out = append(out, r)
		i += size
	}
	return out
}

// appendEncoded appends the UTF-8 encoding of rs to dst, turning escaped raw
// bytes back into the original bytes.
func appendEncoded(dst []byte, rs []rune) []byte {
	for _, r := range rs {
		if isRawByte(r) {
			dst = append(dst, byte(r-rawByteBase))
			continue
		}
		dst = utf8.AppendRune(dst, r)
	}
	return dst
}

// hasRawBytes reports whether any line holds an escaped raw byte, i.e. the
// file was not valid UTF-8.
func hasRawBytes(t Text) bool {
	for i := 0; i < t.LineCount(); i++ {
		for _, r := range t.Line(i) {
			if isRawByte(r) {
				return true
			}
		}
	}
	return false
}

// lineSlice is the default Text implementation.
type lineSlice struct {
	lines [][]rune
}

// newText splits content into lines. A trailing newline terminates the last
// line rather than starting an empty one: it is tracked out of band (see
// Tab.trailingNL) so saving reproduces the input byte for byte. CR is stripped
// from CRLF input; saves normalize the file to LF.
func newText(content []byte) *lineSlice {
	text := &lineSlice{}
	text.lines = make([][]rune, 0, bytes.Count(content, []byte{'\n'})+1)
	for _, ln := range bytes.Split(content, []byte{'\n'}) {
		if n := len(ln); n > 0 && ln[n-1] == '\r' {
			ln = ln[:n-1]
		}
		text.lines = append(text.lines, decodeRunes(ln))
	}
	// bytes.Split always yields a trailing empty element for newline-terminated
	// input; drop it so it does not show up as a phantom last line.
	if len(content) > 0 && content[len(content)-1] == '\n' && len(text.lines) > 1 {
		text.lines = text.lines[:len(text.lines)-1]
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

// RemoveLine drops line i and returns it. Removing the only line clears it
// instead, keeping the "at least one line" invariant.
func (t *lineSlice) RemoveLine(i int) []rune {
	rem := t.lines[i]
	if len(t.lines) == 1 {
		t.lines[0] = nil
		return rem
	}
	copy(t.lines[i:], t.lines[i+1:])
	t.lines[len(t.lines)-1] = nil // release the moved-out reference
	t.lines = t.lines[:len(t.lines)-1]
	return rem
}

// insertText inserts runes (possibly containing newlines) at line:col.
func insertText(t Text, line, col int, s []rune) {
	if len(s) == 0 {
		return
	}
	rr := splitRunes(s)
	if len(rr) == 1 {
		l := t.Line(line)
		col = clampCol(l, col)
		nl := make([]rune, 0, len(l)+len(rr[0]))
		nl = append(nl, l[:col]...)
		nl = append(nl, rr[0]...)
		nl = append(nl, l[col:]...)
		t.Set(line, nl)
		return
	}
	head := t.Line(line)
	col = clampCol(head, col)
	tail := append([]rune{}, head[col:]...)
	// Build a fresh head instead of appending in place: the old backing array
	// may be shared with a recorded undo op.
	nh := make([]rune, 0, col+len(rr[0]))
	nh = append(nh, head[:col]...)
	nh = append(nh, rr[0]...)
	t.Set(line, nh)
	at := line + 1
	for i := 1; i < len(rr); i++ {
		if i == len(rr)-1 {
			l := make([]rune, 0, len(rr[i])+len(tail))
			l = append(l, rr[i]...)
			l = append(l, tail...)
			t.InsertLine(at, l)
		} else {
			t.InsertLine(at, append([]rune{}, rr[i]...))
		}
		at++
	}
}

// splitRunes splits s on newlines without going through a string.
func splitRunes(s []rune) [][]rune {
	out := [][]rune{}
	start := 0
	for i, r := range s {
		if r == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func clampCol(line []rune, col int) int {
	if col < 0 {
		return 0
	}
	if col > len(line) {
		return len(line)
	}
	return col
}

// deleteText removes n runes starting at line:col and returns them.
func deleteText(t Text, line, col, n int) []rune {
	var out []rune
	cur, c := line, col
	if cur < 0 || cur >= t.LineCount() {
		return nil
	}
	c = clampCol(t.Line(cur), c)
	for n > 0 {
		l := t.Line(cur)
		if c < len(l) {
			take := n
			if take > len(l)-c {
				take = len(l) - c
			}
			out = append(out, l[c:c+take]...)
			nl := make([]rune, 0, len(l)-take)
			nl = append(nl, l[:c]...)
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
		// Join the next line onto this one with a fresh array; the current
		// line's array may be shared with a recorded undo op.
		l = t.Line(cur)
		next := t.Line(cur + 1)
		merged := make([]rune, 0, len(l)+len(next))
		merged = append(merged, l...)
		merged = append(merged, next...)
		t.Set(cur, merged)
		t.RemoveLine(cur + 1)
	}
	return out
}
