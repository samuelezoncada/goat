package editor

import (
	"math/rand"
	"strings"
	"testing"
)

// model is a flat rune-slice reference implementation of the buffer, used to
// check that the line-slice implementation agrees with it. It has to be
// rune-based, since the editor addresses text in runes, not bytes.
type model struct{ s []rune }

func newModel(s string) *model { return &model{s: []rune(s)} }

func (m *model) String() string { return string(m.s) }

func (m *model) insert(off int, text string) {
	off = clampInt(off, 0, len(m.s))
	rs := []rune(text)
	out := make([]rune, 0, len(m.s)+len(rs))
	out = append(out, m.s[:off]...)
	out = append(out, rs...)
	out = append(out, m.s[off:]...)
	m.s = out
}

func (m *model) delete(off, n int) string {
	off = clampInt(off, 0, len(m.s))
	if off+n > len(m.s) {
		n = len(m.s) - off
	}
	if n <= 0 {
		return ""
	}
	cut := string(m.s[off : off+n])
	m.s = append(m.s[:off:off], m.s[off+n:]...)
	return cut
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// offsetOf converts a line:col position into a rune offset in the joined text.
func offsetOf(t Text, line, col int) int {
	off := 0
	for i := 0; i < line && i < t.LineCount(); i++ {
		off += len(t.Line(i)) + 1
	}
	return off + col
}

func joinText(t Text) string {
	var b strings.Builder
	for i := 0; i < t.LineCount(); i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(t.Line(i)))
	}
	return b.String()
}

// TestBufferMatchesModel drives random inserts and deletes against both the
// buffer and a string model and asserts they stay identical.
func TestBufferMatchesModel(t *testing.T) {
	rng := rand.New(rand.NewSource(20260828))
	words := []string{"a", "ab", "\n", "x\ny", "é", "世", "\t", "", "one\ntwo\nthree"}

	for iter := 0; iter < 400; iter++ {
		txt := newText([]byte("seed line\nsecond"))
		m := newModel("seed line\nsecond")
		for step := 0; step < 60; step++ {
			line := rng.Intn(txt.LineCount())
			col := 0
			if l := len(txt.Line(line)); l > 0 {
				col = rng.Intn(l + 1)
			}
			off := offsetOf(txt, line, col)
			if rng.Intn(2) == 0 {
				w := words[rng.Intn(len(words))]
				insertText(txt, line, col, []rune(w))
				m.insert(off, w)
			} else {
				n := rng.Intn(6)
				got := deleteText(txt, line, col, n)
				want := m.delete(off, n)
				if string(got) != want {
					t.Fatalf("iter %d step %d: deleted %q want %q\nbuffer=%q", iter, step, string(got), want, joinText(txt))
				}
			}
			if got, want := joinText(txt), m.String(); got != want {
				t.Fatalf("iter %d step %d: buffer %q != model %q", iter, step, got, want)
			}
			if txt.LineCount() < 1 {
				t.Fatalf("iter %d step %d: buffer lost its last line", iter, step)
			}
		}
	}
}

// TestUndoRestoresExactly drives random edits through the Tab API and asserts
// that undoing everything returns the buffer to its initial content.
func TestUndoRestoresExactly(t *testing.T) {
	rng := rand.New(rand.NewSource(4242))
	const start = "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n"

	for iter := 0; iter < 200; iter++ {
		tb := newTestTab(start)
		tb.cfg = DefaultConfig()
		e := &Editor{tabs: []*Tab{tb}, cfg: tb.cfg, width: 80, height: 24}
		e.search.caseSens = true
		e.search.text = "main"
		e.replaceTo = "MAIN"

		for step := 0; step < 40; step++ {
			tb.cur = Pos{rng.Intn(tb.lineCount()), 0}
			if l := len(tb.line(tb.cur.Line)); l > 0 {
				tb.cur.Col = rng.Intn(l + 1)
			}
			if rng.Intn(4) == 0 {
				// half the time, work on a selection
				m := Pos{rng.Intn(tb.lineCount()), 0}
				if l := len(tb.line(m.Line)); l > 0 {
					m.Col = rng.Intn(l + 1)
				}
				tb.mark = &m
			} else {
				tb.mark = nil
			}
			switch rng.Intn(10) {
			case 0:
				tb.insertRune(rune('a' + rng.Intn(26)))
			case 1:
				tb.insertNewline()
			case 2:
				tb.backspace()
			case 3:
				tb.deleteForward()
			case 4:
				tb.insertTab()
			case 5:
				tb.dedent()
			case 6:
				e.cut()
			case 7:
				e.uncut()
			case 8:
				e.justify()
			case 9:
				e.replaceAll()
			}
			checkCur(t, tb)
		}
		for tb.edits.CanUndo() {
			tb.undo()
		}
		// Compare the on-disk form, so the trailing newline is checked too.
		if got := string(tb.bytes()); got != start {
			t.Fatalf("iter %d: undoing everything gave\n%q\nwant\n%q", iter, got, start)
		}
		if tb.dirty {
			t.Fatalf("iter %d: buffer still dirty after a full undo", iter)
		}
	}
}

// TestRedoRestoresExactly checks undo/redo symmetry.
func TestRedoRestoresExactly(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for iter := 0; iter < 100; iter++ {
		tb := newTestTab("alpha beta\ngamma\n")
		tb.cfg = DefaultConfig()
		e := &Editor{tabs: []*Tab{tb}, cfg: tb.cfg, width: 60, height: 20}
		for step := 0; step < 20; step++ {
			tb.cur = Pos{rng.Intn(tb.lineCount()), 0}
			switch rng.Intn(5) {
			case 0:
				tb.insertRune('z')
			case 1:
				tb.insertNewline()
			case 2:
				tb.backspace()
			case 3:
				e.cut()
			case 4:
				tb.insertTab()
			}
		}
		final := joinLines(t, tb)
		n := 0
		for tb.edits.CanUndo() {
			tb.undo()
			n++
		}
		for i := 0; i < n; i++ {
			tb.redo()
		}
		if got := joinLines(t, tb); got != final {
			t.Fatalf("iter %d: redo did not restore\n%q\nwant\n%q", iter, got, final)
		}
	}
}

// FuzzBufferEdits is the go-fuzz entry point for the same invariants.
func FuzzBufferEdits(f *testing.F) {
	f.Add("hello\nworld", 0, 1, "x\ny")
	f.Add("", 0, 0, "\n")
	f.Fuzz(func(t *testing.T, initial string, line, col int, text string) {
		if len(initial) > 4096 || len(text) > 256 {
			t.Skip()
		}
		txt := newText([]byte(initial))
		if line < 0 || col < 0 {
			t.Skip()
		}
		line = line % txt.LineCount()
		col = clampCol(txt.Line(line), col)
		before := joinText(txt)
		insertText(txt, line, col, []rune(text))
		if txt.LineCount() < 1 {
			t.Fatal("buffer lost its last line")
		}
		// Compare in rune terms: the fuzzer can hand us a string holding
		// invalid UTF-8, which []rune() normalizes to U+FFFD before it ever
		// reaches the buffer.
		want := string([]rune(text))
		got := deleteText(txt, line, col, len([]rune(text)))
		if string(got) != want {
			t.Fatalf("insert/delete round trip: %q != %q", string(got), want)
		}
		if after := joinText(txt); after != before {
			t.Fatalf("insert then delete changed the buffer:\n%q\n%q", before, after)
		}
	})
}
