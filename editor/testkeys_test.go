package editor

import (
	"time"

	"github.com/gdamore/tcell/v2"

	"goat/syntax"
)

// Small helpers so tests can synthesize key events readably.
const (
	tcell_KeyBackspace = tcell.KeyBackspace2
)

func keyEvent(k tcell.Key) *tcell.EventKey {
	return tcell.NewEventKey(k, 0, tcell.ModNone)
}

func runeEvent(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
}

func altRuneEvent(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModAlt)
}

func cmdRuneEvent(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModMeta)
}

func cmdShiftRuneEvent(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModMeta|tcell.ModShift)
}

func cmdKeyEvent(k tcell.Key) *tcell.EventKey {
	return tcell.NewEventKey(k, 0, tcell.ModMeta)
}

func cmdShiftKeyEvent(k tcell.Key) *tcell.EventKey {
	return tcell.NewEventKey(k, 0, tcell.ModMeta|tcell.ModShift)
}

const (
	tcellKeyEnter = tcell.KeyEnter
	tcellKeyEsc   = tcell.KeyEsc
)

func sleepMillis(n int) { time.Sleep(time.Duration(n) * time.Millisecond) }

func timeZero() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

// newTestHighlighter returns a highlighter with a real lexer attached.
func newTestHighlighter() *syntax.Highlighter {
	return syntax.NewHighlighter(syntax.Detect("x.go", ""))
}
