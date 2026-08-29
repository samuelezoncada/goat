package editor

import (
	"regexp"
	"strings"
	"unicode"
)

// Search holds the active search/replace state.
type Search struct {
	text      string
	dir       int // 1 = forward, -1 = backward
	caseSens  bool
	regex     bool
	wholeWord bool
	highlight bool // highlight every match in the viewport

	re    *regexp.Regexp // compiled pattern (regex mode), nil when invalid
	reErr string
	reSrc string // the source the cached regexp was built from
}

// pattern compiles and caches the search pattern for regex mode.
func (s *Search) pattern() (*regexp.Regexp, string) {
	if !s.regex || s.text == "" {
		return nil, ""
	}
	key := s.text + "\x00"
	if s.caseSens {
		key += "c"
	}
	if s.wholeWord {
		key += "w"
	}
	if s.reSrc == key {
		return s.re, s.reErr
	}
	src := s.text
	if s.wholeWord {
		src = `\b(?:` + src + `)\b`
	}
	if !s.caseSens {
		src = "(?i)" + src
	}
	re, err := regexp.Compile(src)
	s.reSrc, s.re, s.reErr = key, re, ""
	if err != nil {
		s.re = nil
		s.reErr = err.Error()
	}
	return s.re, s.reErr
}

// beginSearch opens the where-is prompt with live forward matching.
func (e *Editor) beginSearch() {
	e.search.dir = 1
	start := Pos{}
	if t := e.active(); t != nil {
		start = t.cur
	}
	e.beginPromptCancel("Search: ", e.search.text, func(txt string) {
		// Live search: always measure from where the prompt was opened, so
		// deleting a character does not skip forward through the file.
		e.search.text = txt
		if txt == "" {
			e.search.highlight = false
			return
		}
		if t := e.active(); t != nil {
			t.cur = start
		}
		e.search.highlight = true
		if !e.doSearch(txt, 1, true) {
			e.statusf("Search not found: %q", txt)
		}
	}, func(txt string) {
		if txt == "" {
			return
		}
		e.search.text = txt
		e.search.highlight = true
		e.pushJumpAt(start)
		if !e.doSearch(txt, 1, true) {
			e.statusf("Search not found: %q", txt)
		}
	}, func() {
		// Cancelled: put the cursor back where the search started.
		if t := e.active(); t != nil {
			t.cur = t.clampPos(start)
			t.destCol = t.cur.Col
		}
		e.search.highlight = false
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
	// Keep the compiled pattern in sync with what is being searched for; the
	// cache is keyed on Search.text.
	if e.search.text != needle {
		e.search.text = needle
	}
	if _, err := e.search.pattern(); err != "" {
		e.errorf("Bad pattern: %s", err)
		return false
	}
	start := t.cur
	m, ok := e.scanFrom(t, needle, start.Line, start.Col, dir, initial)
	if !ok {
		return false
	}
	e.setCursor(m.line, m.col)
	if e.msg != "" && strings.HasPrefix(e.msg, "Search not found") {
		e.statusf("")
	}
	return true
}

// foundMatch is one match: its line and the rune columns it spans.
type foundMatch struct {
	line     int
	col, end int
}

func (e *Editor) scanFrom(t *Tab, needle string, startLine, startCol, dir int, initial bool) (foundMatch, bool) {
	return e.scanFromWrap(t, needle, startLine, startCol, dir, initial, true)
}

// scanFromWrap is scanFrom with explicit wrap control.
func (e *Editor) scanFromWrap(t *Tab, needle string, startLine, startCol, dir int, initial bool, wrap bool) (foundMatch, bool) {
	n := t.lineCount()
	if n == 0 || needle == "" {
		return foundMatch{}, false
	}
	var needleRunes []rune
	if !e.search.regex {
		needleRunes = []rune(needle)
		if !e.search.caseSens {
			needleRunes = lowerRunes(needleRunes)
		}
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
			if lineIdx >= n || lineIdx < 0 {
				continue
			}
		} else {
			lineIdx = startLine - k
			if wrap && lineIdx < 0 {
				lineIdx += n
			}
			if lineIdx < 0 || lineIdx >= n {
				continue
			}
		}
		onStart := k == 0
		line := t.line(lineIdx)
		from, to := 0, len(line)
		if onStart {
			if dir > 0 {
				from = startCol
				if !initial {
					from = startCol + 1
				}
			} else {
				to = startCol
				if !initial {
					to = startCol - 1
				}
			}
		}
		if m, ok := e.matchInLine(line, needleRunes, from, to, dir); ok {
			m.line = lineIdx
			return m, true
		}
	}
	return foundMatch{}, false
}

// matchInLine finds a match in line within [from, to], scanning in dir.
func (e *Editor) matchInLine(line, needle []rune, from, to, dir int) (foundMatch, bool) {
	if from < 0 {
		from = 0
	}
	if to > len(line) {
		to = len(line)
	}
	if from > len(line) || to < 0 {
		return foundMatch{}, false
	}
	if e.search.regex {
		re, err := e.search.pattern()
		if re == nil || err != "" {
			return foundMatch{}, false
		}
		ms := reMatchesRunes(re, line)
		if dir > 0 {
			for _, m := range ms {
				if m.col >= from && m.end <= len(line) {
					return m, true
				}
			}
			return foundMatch{}, false
		}
		for i := len(ms) - 1; i >= 0; i-- {
			if ms[i].end <= to {
				return ms[i], true
			}
		}
		return foundMatch{}, false
	}
	if len(needle) == 0 {
		return foundMatch{}, false
	}
	hay := line
	if !e.search.caseSens {
		hay = lowerRunes(line)
	}
	if dir > 0 {
		for i := from; i+len(needle) <= len(hay); i++ {
			if runesEqual(hay[i:i+len(needle)], needle) && e.wordOK(line, i, i+len(needle)) {
				return foundMatch{col: i, end: i + len(needle)}, true
			}
		}
		return foundMatch{}, false
	}
	for i := to - len(needle); i >= 0; i-- {
		if runesEqual(hay[i:i+len(needle)], needle) && e.wordOK(line, i, i+len(needle)) {
			return foundMatch{col: i, end: i + len(needle)}, true
		}
	}
	return foundMatch{}, false
}

// wordOK enforces the whole-word option for plain (non-regex) searches.
func (e *Editor) wordOK(line []rune, start, end int) bool {
	if !e.search.wholeWord {
		return true
	}
	if start > 0 && isWord(line[start-1]) {
		return false
	}
	if end < len(line) && isWord(line[end]) {
		return false
	}
	return true
}

// reMatchesRunes runs a regexp over a line and converts the byte offsets it
// reports into rune columns.
func reMatchesRunes(re *regexp.Regexp, line []rune) []foundMatch {
	s := string(line)
	locs := re.FindAllStringIndex(s, -1)
	if locs == nil {
		return nil
	}
	if len(s) == len(line) {
		// All-ASCII line: byte offsets are already rune columns.
		out := make([]foundMatch, 0, len(locs))
		for _, l := range locs {
			out = append(out, foundMatch{col: l[0], end: l[1]})
		}
		return out
	}
	// Map byte offsets to rune indices in one pass.
	byteToRune := make(map[int]int, len(s)+1)
	ri := 0
	for bi := range s {
		byteToRune[bi] = ri
		ri++
	}
	byteToRune[len(s)] = ri
	out := make([]foundMatch, 0, len(locs))
	for _, l := range locs {
		c, ok1 := byteToRune[l[0]]
		en, ok2 := byteToRune[l[1]]
		if !ok1 || !ok2 || en < c {
			continue
		}
		out = append(out, foundMatch{col: c, end: en})
	}
	return out
}

// lineMatches returns every match on a line, used to highlight the viewport.
func (e *Editor) lineMatches(line []rune) []foundMatch {
	if !e.search.highlight || e.search.text == "" {
		return nil
	}
	if e.search.regex {
		re, err := e.search.pattern()
		if re == nil || err != "" {
			return nil
		}
		return reMatchesRunes(re, line)
	}
	needle := []rune(e.search.text)
	if !e.search.caseSens {
		needle = lowerRunes(needle)
	}
	if len(needle) == 0 {
		return nil
	}
	hay := line
	if !e.search.caseSens {
		hay = lowerRunes(line)
	}
	var out []foundMatch
	for i := 0; i+len(needle) <= len(hay); i++ {
		if runesEqual(hay[i:i+len(needle)], needle) && e.wordOK(line, i, i+len(needle)) {
			out = append(out, foundMatch{col: i, end: i + len(needle)})
			i += len(needle) - 1
		}
	}
	return out
}

// lowerRunes returns runes mapped to lowercase (one-to-one, preserving length).
func lowerRunes(rs []rune) []rune {
	out := make([]rune, len(rs))
	for i, r := range rs {
		out[i] = unicode.ToLower(r)
	}
	return out
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
	t.mark = nil
	t.cur = t.clampPos(Pos{line, col})
	t.destCol = t.cur.Col
}

// --- replace -------------------------------------------------------------

func (e *Editor) beginReplace() {
	t := e.active()
	if t != nil && t.readOnly {
		e.statusf("%s is read-only", t.name)
		return
	}
	// A selection scopes the replacement; remember it before the prompt moves
	// the cursor around.
	e.replaceScope = nil
	if t != nil && t.hasSelection() {
		sel := t.selRange()
		e.replaceScope = &sel
	}
	e.beginPrompt("Replace Where Is: ", e.search.text, nil, func(from string) {
		if from == "" {
			return
		}
		e.search.text = from
		e.search.highlight = true
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
	case "y", "Y", "":
		e.replaceCurrent()
	case "n", "N":
		e.skipCurrent()
	case "a", "A":
		e.replaceAll()
	default:
		e.statusf("Replace cancelled")
	}
}

// matchAtCursor re-finds the match the cursor is sitting on, so the recorded
// undo text is what was actually replaced (which differs from the typed
// pattern for case-insensitive and regex searches).
func (e *Editor) matchAtCursor(t *Tab) (foundMatch, bool) {
	line := t.line(t.cur.Line)
	var needle []rune
	if !e.search.regex {
		needle = []rune(e.search.text)
		if !e.search.caseSens {
			needle = lowerRunes(needle)
		}
	}
	m, ok := e.matchInLine(line, needle, t.cur.Col, len(line), 1)
	if !ok || m.col != t.cur.Col {
		return foundMatch{}, false
	}
	m.line = t.cur.Line
	return m, true
}

// replacementFor returns the text to insert for a match, expanding regex
// capture references ($1, ${name}) when in regex mode.
func (e *Editor) replacementFor(line []rune, m foundMatch) []rune {
	if !e.search.regex {
		return []rune(e.replaceTo)
	}
	re, err := e.search.pattern()
	if re == nil || err != "" {
		return []rune(e.replaceTo)
	}
	src := string(line[m.col:m.end])
	idx := re.FindStringSubmatchIndex(src)
	if idx == nil {
		return []rune(e.replaceTo)
	}
	return []rune(string(re.ExpandString(nil, e.replaceTo, src, idx)))
}

// replaceOne swaps the given match for the replacement, recording one op.
func (e *Editor) replaceOne(t *Tab, m foundMatch) int {
	line := t.line(m.line)
	old := cloneRunes(line[m.col:m.end])
	to := e.replacementFor(line, m)
	if len(old) == 0 && len(to) == 0 {
		return 0 // nothing to record
	}
	before := t.cur
	o := &op{kind: opReplace, line: m.line, col: m.col, text: old, new: to, curBefore: before}
	if len(old) > 0 {
		deleteText(t.text, m.line, m.col, len(old))
	}
	if len(to) > 0 {
		insertText(t.text, m.line, m.col, to)
	}
	t.cur = Pos{m.line, m.col + len(to)}
	t.destCol = t.cur.Col
	o.curAfter = t.cur
	t.edits.push(o)
	t.setDirty()
	t.invalidate(m.line)
	return len(to)
}

func (e *Editor) replaceCurrent() {
	t := e.active()
	if t == nil {
		return
	}
	m, ok := e.matchAtCursor(t)
	if !ok {
		e.statusf("Replace cancelled")
		return
	}
	e.replaceOne(t, m)
	t.clampCursor()
	if e.doSearch(e.search.text, 1, false) {
		e.askReplace()
		return
	}
	e.statusf("No more occurrences")
}

func (e *Editor) skipCurrent() {
	t := e.active()
	if t == nil {
		return
	}
	if m, ok := e.matchAtCursor(t); ok {
		t.cur.Col = m.end
		if t.cur.Col > len(t.line(t.cur.Line)) {
			t.cur.Col = len(t.line(t.cur.Line))
		}
		t.destCol = t.cur.Col
		if e.doSearch(e.search.text, 1, true) {
			e.askReplace()
			return
		}
		e.statusf("No more occurrences")
		return
	}
	if e.doSearch(e.search.text, 1, false) {
		e.askReplace()
		return
	}
	e.statusf("No more occurrences")
}

// replaceAll replaces every occurrence in the buffer as one undoable action.
// The scan runs from the top of the file and never wraps, and it always
// continues past the text just inserted, so a replacement that contains the
// search text cannot loop forever.
func (e *Editor) replaceAll() {
	t := e.active()
	if t == nil {
		return
	}
	if t.readOnly {
		e.statusf("%s is read-only", t.name)
		return
	}
	var needle []rune
	if !e.search.regex {
		needle = []rune(e.search.text)
		if !e.search.caseSens {
			needle = lowerRunes(needle)
		}
	}
	if _, err := e.search.pattern(); err != "" {
		e.errorf("Bad pattern: %s", err)
		return
	}
	count := 0
	first, last := 0, t.lineCount()-1
	scope := e.replaceScope
	// endCol bounds the last line of a selection; it shifts as replacements
	// change the text length.
	endCol := -1
	if scope != nil {
		first, last = scope[0], scope[2]
		if last >= t.lineCount() {
			last = t.lineCount() - 1
		}
		endCol = scope[3]
	}
	t.edits.begin()
	for line := first; line <= last && line < t.lineCount(); line++ {
		col, prev := 0, -1
		if scope != nil && line == scope[0] {
			col = scope[1]
		}
		for {
			if col <= prev {
				break // safety net: the scan must always move forward
			}
			prev = col
			// The limit is re-read every round: a replacement can grow or
			// shrink the line under us.
			limit := len(t.line(line))
			if scope != nil && line == last && endCol >= 0 {
				limit = clampCol(t.line(line), endCol)
			}
			m, ok := e.matchInLine(t.line(line), needle, col, limit, 1)
			if !ok || m.end > limit {
				break
			}
			m.line = line
			empty := m.end == m.col
			n := e.replaceOne(t, m)
			count++
			if scope != nil && line == last && endCol >= 0 {
				endCol += n - (m.end - m.col)
			}
			col = m.col + n
			if empty {
				// A zero-width match (e.g. "x*") must also step over one
				// source character, or the scan never reaches the end.
				col++
			}
			if col > len(t.line(line)) {
				break
			}
		}
	}
	t.edits.end()
	t.clampCursor()
	scoped := ""
	if scope != nil {
		scoped = " in the selection"
		t.mark = nil
	}
	e.replaceScope = nil
	if count == 0 {
		e.statusf("Phrase not found%s: %q", scoped, e.search.text)
		return
	}
	e.statusf("Replaced %d occurrence%s%s", count, plural(count), scoped)
}
