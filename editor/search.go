package editor

import "unicode"

// Search holds the active search/replace state.
type Search struct {
	text      string
	dir       int // 1 = forward, -1 = backward
	caseSens  bool
	lastStart int // last cursor pos used to seed a search
}

// beginSearch opens the where-is prompt with live forward matching.
func (e *Editor) beginSearch() {
	e.search.dir = 1
	e.beginPrompt("Search: ", e.search.text, func(t string) {
		e.search.text = t
		if t == "" {
			return
		}
		e.doSearch(t, 1, true)
	}, func(t string) {
		if t != "" {
			e.search.text = t
			e.doSearch(t, 1, true)
		}
	})
}

// searchNext repeats the last search in the current direction.
func (e *Editor) searchNext() {
	if e.search.text == "" {
		e.beginSearch()
		return
	}
	if !e.doSearch(e.search.text, e.search.dir, false) {
		e.statusf("Search not found: %q", e.search.text)
	}
}

// searchReverse searches in the opposite direction.
func (e *Editor) searchReverse() {
	if e.search.text == "" {
		return
	}
	e.search.dir = -e.search.dir
	if !e.doSearch(e.search.text, e.search.dir, false) {
		e.statusf("Search not found: %q", e.search.text)
	}
}

// doSearch finds needle relative to the cursor. initial=true searches
// inclusive of the cursor position; repeat=true starts just after it.
func (e *Editor) doSearch(needle string, dir int, initial bool) bool {
	t := e.active()
	if t == nil || needle == "" {
		return false
	}
	start := t.cur
	line, col, ok := e.scanFrom(t, needle, start.Line, start.Col, dir, initial)
	if !ok {
		return false
	}
	e.setCursor(line, col)
	e.statusf("")
	return true
}

func (e *Editor) scanFrom(t *Tab, needle string, startLine, startCol, dir int, initial bool) (int, int, bool) {
	return e.scanFromWrap(t, needle, startLine, startCol, dir, initial, true)
}

// scanFromWrap is scanFrom with explicit wrap control. Replace-all scans
// without wrapping so a replacement that contains the search text cannot loop
// forever re-finding matches in already-replaced content.
func (e *Editor) scanFromWrap(t *Tab, needle string, startLine, startCol, dir int, initial bool, wrap bool) (int, int, bool) {
	n := t.lineCount()
	if n == 0 || needle == "" {
		return 0, 0, false
	}
	needleRunes := []rune(needle)
	if !e.search.caseSens {
		needleRunes = lowerRunes(needleRunes)
	}
	// Iterate n times (n+1 with wrap) so a full wrap re-visits the start line;
	// the first visit is restricted to startCol, the wrap-around pass searches
	// the rest.
	limit := n
	if wrap {
		limit = n + 1
	}
	for k := 0; k < limit; k++ {
		var lineIdx int
		if dir > 0 {
			lineIdx = startLine + k
			if wrap && lineIdx >= n {
				lineIdx -= n
			}
			if lineIdx >= n {
				continue
			}
		} else {
			lineIdx = startLine - k
			if wrap && lineIdx < 0 {
				lineIdx += n
			}
			if lineIdx < 0 {
				continue
			}
		}
		onStart := k == 0
		line := t.line(lineIdx)
		hay := line
		if !e.search.caseSens {
			hay = lowerRunes(line)
		}
		var pos int
		if dir > 0 {
			from := 0
			if onStart {
				from = startCol
				if !initial {
					from = startCol + 1
				}
			}
			pos = indexRunes(hay, needleRunes, from)
		} else {
			end := len(hay)
			if onStart {
				end = startCol
				if !initial {
					end = startCol - 1
				}
			}
			pos = lastIndexRunes(hay, needleRunes, end)
		}
		if pos >= 0 {
			return lineIdx, pos, true
		}
	}
	return 0, 0, false
}

// lowerRunes returns runes mapped to lowercase (one-to-one, preserving length).
func lowerRunes(rs []rune) []rune {
	out := make([]rune, len(rs))
	for i, r := range rs {
		out[i] = unicode.ToLower(r)
	}
	return out
}

// indexRunes returns the rune index of needle in hay at or after from, or -1.
func indexRunes(hay, needle []rune, from int) int {
	if from < 0 {
		from = 0
	}
	for i := from; i+len(needle) <= len(hay); i++ {
		if runesEqual(hay[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}

// lastIndexRunes returns the rune index of the last needle in hay[:end], or -1.
func lastIndexRunes(hay, needle []rune, end int) int {
	if end > len(hay) {
		end = len(hay)
	}
	for i := end - len(needle); i >= 0; i-- {
		if runesEqual(hay[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (e *Editor) setCursor(line, col int) {
	t := e.active()
	if t == nil {
		return
	}
	if line < 0 || line >= t.lineCount() {
		return
	}
	l := len(t.line(line))
	if col < 0 {
		col = 0
	}
	if col > l {
		col = l
	}
	t.cur = Pos{line, col}
	t.destCol = col
}

// --- replace -------------------------------------------------------------

func (e *Editor) beginReplace() {
	e.beginPrompt("Replace Where Is: ", e.search.text, nil, func(t string) {
		if t == "" {
			return
		}
		e.search.text = t
		e.beginPrompt("Replace with: ", e.replaceTo, nil, func(to string) {
			e.replaceTo = to
			e.startReplaceLoop()
		})
	})
}

func (e *Editor) startReplaceLoop() {
	if !e.doSearch(e.search.text, 1, true) {
		e.statusf("Phrase not found: %q", e.search.text)
		return
	}
	e.askReplace()
}

func (e *Editor) askReplace() {
	e.beginPrompt("Replace this instance? [y/n/a] ", "", nil, e.replaceAnswer)
}

func (e *Editor) replaceAnswer(ans string) {
	switch ans {
	case "y", "Y":
		e.replaceCurrent()
	case "n", "N":
		e.skipCurrent()
	case "a", "A":
		e.replaceAll()
	default:
		e.statusf("Replace cancelled")
	}
}

func (e *Editor) replaceCurrent() {
	t := e.active()
	from := []rune(e.search.text)
	to := []rune(e.replaceTo)
	line, col := t.cur.Line, t.cur.Col
	o := &op{kind: opReplace, line: line, col: col, text: from, new: to, curBefore: t.cur}
	deleteText(t.text, line, col, len(from))
	insertText(t.text, line, col, to)
	t.cur = Pos{line, col + len(to)}
	t.destCol = t.cur.Col
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(line)
	if e.doSearch(e.search.text, 1, false) {
		e.askReplace()
	} else {
		e.statusf("Replaced all occurrences")
	}
}

func (e *Editor) skipCurrent() {
	t := e.active()
	from := len([]rune(e.search.text))
	line := t.cur.Line
	t.cur.Col += from
	if t.cur.Col > len(t.line(line)) {
		t.cur.Col = len(t.line(line))
		if line+1 < t.lineCount() {
			t.cur.Line = line + 1
			t.cur.Col = 0
		}
	}
	t.destCol = t.cur.Col
	if e.doSearch(e.search.text, 1, false) {
		e.askReplace()
	} else {
		e.statusf("Replaced all occurrences")
	}
}

func (e *Editor) replaceAll() {
	t := e.active()
	count := 0
	needle := []rune(e.search.text)
	to := []rune(e.replaceTo)
	for {
		// Scan without wrapping: after each replacement the cursor sits just
		// past the inserted text, so a replacement containing the search text
		// cannot be re-found and loop forever.
		line, col, ok := e.scanFromWrap(t, e.search.text, t.cur.Line, t.cur.Col, 1, true, false)
		if !ok {
			break
		}
		o := &op{kind: opReplace, line: line, col: col, text: needle, new: to, curBefore: t.cur}
		deleteText(t.text, line, col, len(needle))
		insertText(t.text, line, col, to)
		t.cur = Pos{line, col + len(to)}
		t.destCol = t.cur.Col
		o.curAfter = t.cur
		t.edits.push(o)
		t.setDirty()
		t.invalidate(line)
		count++
	}
	e.statusf("Replaced %d occurrence(s)", count)
}
