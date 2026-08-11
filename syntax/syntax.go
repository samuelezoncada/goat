// Package syntax provides syntax highlighting backed by the chroma lexers
// (a Go port of Pygments), which are maintained upstream.
package syntax

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Language describes a highlighting target: a chroma lexer plus its name.
type Language struct {
	Name  string
	Lexer chroma.Lexer
}

// Detect resolves a language for a path (extension/filename) or a shebang
// line. Returns nil when no lexer matches (plain text).
func Detect(path, firstLine string) *Language {
	lx := lexers.Match(path)
	if lx == nil {
		lx = analyse(firstLine)
	}
	if lx == nil {
		return nil
	}
	return &Language{Name: lx.Config().Name, Lexer: lx}
}

// shebangNames maps interpreter names found in #! lines to chroma lexers.
var shebangNames = map[string]string{
	"python": "Python", "python2": "Python", "python3": "Python",
	"ruby": "Ruby", "perl": "Perl", "php": "PHP", "lua": "Lua",
	"bash": "Bash", "sh": "Bash", "zsh": "Bash", "dash": "Bash", "ksh": "Bash",
	"pwsh": "PowerShell", "powershell": "PowerShell",
	"node": "JavaScript",
	"awk":  "Awk", "sed": "Sed",
}

func analyse(firstLine string) chroma.Lexer {
	if firstLine == "" {
		return nil
	}
	if lx := lexers.Analyse(firstLine); lx != nil {
		return lx
	}
	interp := shebangInterp(firstLine)
	if interp == "" {
		return nil
	}
	for name, interpName := range shebangNames {
		if interp == name || strings.HasPrefix(interp, name) {
			if lx := lexers.Get(interpName); lx != nil {
				return lx
			}
		}
	}
	return nil
}

// shebangInterp extracts the interpreter basename from a #! line.
func shebangInterp(line string) string {
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return ""
	}
	base := filepath.Base(fields[0])
	if base == "env" && len(fields) > 1 {
		base = fields[1]
	}
	return base
}

// Highlighter lexes a buffer with chroma in a background goroutine, caching
// per-line spans so the renderer never blocks on highlighting.
type Highlighter struct {
	mu      sync.RWMutex
	lexer   chroma.Lexer
	spans   [][]Span
	text    string
	version int
	onReady func()
	wake    chan struct{}
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewHighlighter returns a highlighter for lang; a nil lang renders plain.
func NewHighlighter(lang *Language) *Highlighter {
	h := &Highlighter{
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
	if lang != nil {
		h.lexer = lang.Lexer
	}
	if h.lexer != nil {
		h.wg.Add(1)
		go h.loop()
	}
	return h
}

// SetOnReady registers a callback invoked (from the worker) after fresh
// spans have been stored. Typically used to wake the render loop.
func (h *Highlighter) SetOnReady(fn func()) {
	h.mu.Lock()
	h.onReady = fn
	h.mu.Unlock()
}

// Close stops the background lexer goroutine.
func (h *Highlighter) Close() {
	if h.lexer == nil {
		return
	}
	close(h.done)
	h.wg.Wait()
}

// SetLineCount is kept for API compatibility; chroma lexes whole buffers.
func (h *Highlighter) SetLineCount(n int) {}

// Invalidate snapshots the current buffer text and schedules a re-lex.
func (h *Highlighter) Invalidate(from, count int, getLine func(i int) []rune) {
	if h.lexer == nil {
		return
	}
	var b strings.Builder
	for i := 0; i < count; i++ {
		b.WriteString(string(getLine(i)))
		if i < count-1 {
			b.WriteByte('\n')
		}
	}
	h.mu.Lock()
	h.text = b.String()
	h.version++
	h.mu.Unlock()
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// Spans returns the cached spans for line i (nil means plain text).
func (h *Highlighter) Spans(i int, line []rune) []Span {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if i < 0 || i >= len(h.spans) {
		return nil
	}
	return h.spans[i]
}

func (h *Highlighter) loop() {
	defer h.wg.Done()
	for {
		select {
		case <-h.done:
			return
		case <-h.wake:
		}
		// Debounce so a burst of edits coalesces into one re-lex.
	drain:
		for {
			select {
			case <-h.wake:
				continue
			case <-h.done:
				return
			case <-time.After(30 * time.Millisecond):
				break drain
			}
		}
		h.mu.RLock()
		text := h.text
		ver := h.version
		h.mu.RUnlock()
		if text == "" {
			h.mu.Lock()
			h.spans = nil
			h.mu.Unlock()
			continue
		}
		spans := lexToLines(h.lexer, text)
		h.mu.Lock()
		changed := false
		if h.version == ver {
			h.spans = spans
			changed = true
		}
		fn := h.onReady
		h.mu.Unlock()
		if changed && fn != nil {
			fn()
		}
	}
}

// lexToLines lexes text and splits the token stream into per-line spans.
func lexToLines(lx chroma.Lexer, text string) [][]Span {
	it, err := lx.Tokenise(nil, text)
	if err != nil {
		return nil
	}
	var lines [][]Span
	cur := []Span{}
	col := 0
	for tok := it(); tok != chroma.EOF; tok = it() {
		if tok.Value == "" {
			continue
		}
		tt := mapType(tok.Type)
		val := tok.Value
		for {
			nl := strings.IndexByte(val, '\n')
			if nl < 0 {
				n := len([]rune(val))
				if n > 0 {
					cur = append(cur, Span{col, n, tt})
					col += n
				}
				break
			}
			part := val[:nl]
			if pn := len([]rune(part)); pn > 0 {
				cur = append(cur, Span{col, pn, tt})
				col += pn
			}
			lines = append(lines, cur)
			cur = []Span{}
			col = 0
			val = val[nl+1:]
		}
	}
	lines = append(lines, cur)
	return lines
}

// mapType maps a chroma token type to a goat token type.
func mapType(t chroma.TokenType) TokenType {
	switch {
	case t >= chroma.Comment && t < chroma.CommentPreproc:
		return TComment
	case t >= chroma.CommentPreproc && t < chroma.Generic:
		return TPreproc
	case t >= chroma.LiteralString && t < chroma.LiteralNumber:
		return TString
	case t >= chroma.LiteralNumber && t < chroma.Operator:
		return TNumber
	case t >= chroma.Literal && t < chroma.LiteralString:
		return TLiteral
	case t >= chroma.Keyword && t < chroma.Name:
		switch t {
		case chroma.KeywordType:
			return TType
		case chroma.KeywordConstant:
			return TConstant
		case chroma.KeywordNamespace:
			return TPreproc
		}
		return TKeyword
	case t >= chroma.Name && t < chroma.Literal:
		switch t {
		case chroma.NameFunction, chroma.NameFunctionMagic:
			return TFunction
		case chroma.NameBuiltin, chroma.NameBuiltinPseudo:
			return TBuiltin
		case chroma.NameVariable, chroma.NameVariableAnonymous, chroma.NameVariableClass, chroma.NameVariableGlobal, chroma.NameVariableInstance, chroma.NameVariableMagic:
			return TVariable
		case chroma.NameClass, chroma.NameNamespace, chroma.NameLabel:
			return TType
		case chroma.NameConstant, chroma.NameEntity:
			return TConstant
		case chroma.NameDecorator:
			return TDecorator
		case chroma.NameAttribute, chroma.NameProperty:
			return TAttribute
		case chroma.NameTag:
			return TTag
		}
		return TPlain
	case t >= chroma.Operator && t < chroma.Punctuation:
		return TOperator
	case t >= chroma.Generic && t < chroma.Text:
		switch t {
		case chroma.GenericHeading, chroma.GenericSubheading:
			return THeading
		case chroma.GenericError:
			return TError
		}
		return TComment
	case t == chroma.Error:
		return TError
	}
	return TPlain
}
