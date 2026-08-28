package syntax

import (
	"strings"
	"testing"
	"time"
)

// waitSpans waits for the background lexer to publish spans for line i.
func waitSpans(t *testing.T, hl *Highlighter, i int, line []rune) []Span {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if sp := hl.Spans(i, line); len(sp) > 0 {
			return sp
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for spans on line %d", i)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestInvalidateReusesPrefix checks that Invalidate honours its `from`
// argument: lines before it are not re-read from the buffer.
func TestInvalidateReusesPrefix(t *testing.T) {
	hl := NewHighlighter(Detect("main.go", ""))
	defer hl.Close()
	lines := []string{"package main", "", "func main() {}"}
	reads := 0
	get := func(i int) []rune {
		reads++
		return []rune(lines[i])
	}
	hl.Invalidate(0, len(lines), get)
	if reads != 3 {
		t.Fatalf("initial snapshot read %d lines want 3", reads)
	}
	reads = 0
	lines[2] = "func main() { x := 1; _ = x }"
	hl.Invalidate(2, len(lines), get)
	if reads != 1 {
		t.Fatalf("re-reading from line 2 read %d lines, want only the changed tail", reads)
	}
	// The snapshot still describes the whole buffer.
	sp := waitSpans(t, hl, 0, []rune(lines[0]))
	if len(sp) == 0 {
		t.Fatal("prefix lost its highlighting")
	}
	sp2 := waitSpans(t, hl, 2, []rune(lines[2]))
	if len(sp2) == 0 {
		t.Fatal("changed line not highlighted")
	}
}

func TestHighlightDisabledForHugeBuffer(t *testing.T) {
	hl := NewHighlighter(Detect("main.go", ""))
	defer hl.Close()
	big := strings.Repeat("x", 1024)
	n := (maxHighlightBytes / len(big)) + 2
	hl.Invalidate(0, n, func(i int) []rune { return []rune(big) })
	if !hl.TooLarge() {
		t.Fatal("a buffer past the limit should disable highlighting")
	}
	if sp := hl.Spans(0, []rune(big)); sp != nil {
		t.Fatal("no spans should be published for an oversized buffer")
	}
	// Shrinking back re-enables it.
	hl.Invalidate(0, 1, func(i int) []rune { return []rune("package main") })
	if hl.TooLarge() {
		t.Fatal("highlighting should come back for a small buffer")
	}
	waitSpans(t, hl, 0, []rune("package main"))
}

// TestSpansClippedToLine covers the window between an edit and the next
// re-lex, when the cached spans can be longer than the line.
func TestSpansClippedToLine(t *testing.T) {
	hl := NewHighlighter(Detect("main.go", ""))
	defer hl.Close()
	line := []rune("package main")
	hl.Invalidate(0, 1, func(i int) []rune { return line })
	waitSpans(t, hl, 0, line)
	// Ask for the spans of a shorter version of the line.
	short := []rune("pack")
	for _, sp := range hl.Spans(0, short) {
		if sp.Start+sp.Len > len(short) {
			t.Fatalf("span %+v exceeds the line length %d", sp, len(short))
		}
	}
	if sp := hl.Spans(0, nil); len(sp) != 0 {
		t.Fatalf("an empty line cannot carry spans: %v", sp)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	hl := NewHighlighter(Detect("main.go", ""))
	hl.Invalidate(0, 1, func(i int) []rune { return []rune("package main") })
	hl.Close()
	hl.Close() // must not panic on a double close
}

func TestPlainBufferHasNoWorker(t *testing.T) {
	hl := NewHighlighter(nil)
	hl.Invalidate(0, 1, func(i int) []rune { return []rune("anything") })
	if sp := hl.Spans(0, []rune("anything")); sp != nil {
		t.Fatal("a buffer with no lexer should stay unstyled")
	}
	hl.Close()
}

func TestShrinkingBufferDropsStaleLines(t *testing.T) {
	hl := NewHighlighter(Detect("main.go", ""))
	defer hl.Close()
	lines := []string{"package main", "var a = 1", "var b = 2"}
	hl.Invalidate(0, 3, func(i int) []rune { return []rune(lines[i]) })
	waitSpans(t, hl, 2, []rune(lines[2]))
	// Now the buffer has one line.
	hl.Invalidate(0, 1, func(i int) []rune { return []rune("package main") })
	deadline := time.Now().Add(3 * time.Second)
	for {
		if len(hl.Spans(2, []rune("var b = 2"))) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("spans for a removed line are still cached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
