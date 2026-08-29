package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"goat/syntax"
)

func benchTab(lines int) *Tab {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		b.WriteString("func example(arg int) error { return fmt.Errorf(\"value %d\", arg) }\n")
	}
	tb := newTestTab(b.String())
	tb.cfg = DefaultConfig()
	// A real highlighter, so the snapshot cost is actually measured.
	tb.lang = syntax.Detect("bench.go", "")
	tb.hl = syntax.NewHighlighter(tb.lang)
	return tb
}

// BenchmarkTypingLargeFile measures the per-keystroke cost in a big buffer:
// this is what the highlighter snapshot used to dominate.
func BenchmarkTypingLargeFile(b *testing.B) {
	tb := benchTab(20000)
	defer tb.close()
	tb.cur = Pos{10000, 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.insertRune('x')
	}
}

// BenchmarkKeystrokeNoFlush is the common case while typing: the snapshot is
// not due yet, so a keystroke costs nothing beyond the edit itself.
func BenchmarkKeystrokeNoFlush(b *testing.B) {
	tb := benchTab(5000)
	defer tb.close()
	tb.cur = Pos{2500, 0}
	now := time.Now()
	tb.flushHighlight(now)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.invalidate(2500)
		tb.flushHighlight(now) // same instant: still within the interval
	}
}

// BenchmarkKeystrokeWithFlush measures a keystroke that does trigger a
// snapshot (at most one per 25 ms in practice).
func BenchmarkKeystrokeWithFlush(b *testing.B) {
	tb := benchTab(5000)
	defer tb.close()
	tb.cur = Pos{2500, 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.invalidate(2500)
		tb.flushHighlight(time.Now().Add(time.Duration(i) * time.Hour))
	}
}

// BenchmarkDisplayColLongLine covers the rune-width path used per cell and per
// column calculation.
func BenchmarkDisplayColLongLine(b *testing.B) {
	line := []rune(strings.Repeat("abcdef\tghij", 500))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = displayCol(line, len(line), 8)
	}
}

func BenchmarkRuneWidthASCII(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = runeWidth('a')
	}
}

func BenchmarkRuneWidthWide(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = runeWidth('世')
	}
}

// BenchmarkFuzzyFilter measures one keystroke in the file picker over a large
// index.
func BenchmarkFuzzyFilter(b *testing.B) {
	e := &Editor{cfg: DefaultConfig()}
	idx := &fileIndex{root: "/r", expanded: map[string]bool{}, ready: true}
	for i := 0; i < 20000; i++ {
		rel := "internal/service/handler" + itoa(i) + "/impl_test.go"
		idx.entries = append(idx.entries, newIndexEntry("/r", rel, false))
	}
	e.fileIndex = idx
	p := &Picker{e: e, input: []rune("srvhandimpl")}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.refilter()
	}
}

// BenchmarkDrawFrame measures a full redraw of a screenful of highlighted text.
func BenchmarkDrawFrame(b *testing.B) {
	tb := benchTab(2000)
	defer tb.close()
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		b.Fatal(err)
	}
	defer scr.Fini()
	scr.SetSize(120, 40)
	e := &Editor{screen: scr, cfg: DefaultConfig(), theme: syntax.DefaultTheme(), width: 120, height: 40}
	tb.cfg = e.cfg
	e.tabs = []*Tab{tb}
	e.allocFrame()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.top = i % 1000
		for y := 0; y < e.mainHeight(); y++ {
			r := row{line: tb.top + y, start: 0, end: len(tb.line(tb.top + y)), first: true}
			e.drawTextRowBench(tb, r, y, 6, 114)
		}
	}
}
