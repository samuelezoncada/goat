package syntax

import (
	"strings"
	"testing"
	"time"
)

func lexLines(lang *Language, text string) [][]Span {
	if lang == nil {
		return nil
	}
	return lexToLines(lang.Lexer, text)
}

func lineSpans(spans [][]Span, line int) []Span {
	if line < 0 || line >= len(spans) {
		return nil
	}
	return spans[line]
}

func hasType(spans []Span, typ TokenType) bool {
	for _, s := range spans {
		if s.Type == typ {
			return true
		}
	}
	return false
}

func TestPHPFunctionAndVariable(t *testing.T) {
	sp := lexLines(Detect("x.php", ""), "function greet($name) {\n    echo $name;\n}")
	if !hasType(lineSpans(sp, 0), TKeyword) {
		t.Fatalf("no keyword: %+v", lineSpans(sp, 0))
	}
	if !hasType(lineSpans(sp, 0), TFunction) {
		t.Fatalf("function name not highlighted: %+v", lineSpans(sp, 0))
	}
	if !hasType(lineSpans(sp, 0), TVariable) {
		t.Fatalf("parameter not a variable: %+v", lineSpans(sp, 0))
	}
	if !hasType(lineSpans(sp, 1), TVariable) {
		t.Fatalf("echo $name var missing: %+v", lineSpans(sp, 1))
	}
}

func TestGoHighlighting(t *testing.T) {
	sp := lexLines(Detect("main.go", ""), "package main\n\nfunc main() {\n\t// comment\n\tx := \"str\"\n}")
	line0 := lineSpans(sp, 0)
	// chroma tags package/import as KeywordNamespace -> preproc colour
	if !hasType(line0, TKeyword) && !hasType(line0, TPreproc) {
		t.Fatalf("package not highlighted: %+v", line0)
	}
	if !hasType(lineSpans(sp, 2), TFunction) {
		t.Fatalf("main not a function: %+v", lineSpans(sp, 2))
	}
	if !hasType(lineSpans(sp, 3), TComment) {
		t.Fatalf("comment not highlighted: %+v", lineSpans(sp, 3))
	}
	if !hasType(lineSpans(sp, 4), TString) {
		t.Fatalf("string not highlighted: %+v", lineSpans(sp, 4))
	}
}

func TestGoBlockCommentAcrossLines(t *testing.T) {
	sp := lexLines(Detect("main.go", ""), "/* open\nstill\nend */\ncode")
	if !hasType(lineSpans(sp, 0), TComment) || !hasType(lineSpans(sp, 1), TComment) {
		t.Fatalf("block comment not spanning lines: %+v", sp)
	}
	if hasType(lineSpans(sp, 3), TComment) {
		t.Fatalf("line after close should not be comment: %+v", lineSpans(sp, 3))
	}
}

func TestPythonDecorator(t *testing.T) {
	sp := lexLines(Detect("x.py", ""), "@app.route('/x')\ndef f():\n    return 42")
	if !hasType(lineSpans(sp, 0), TDecorator) {
		t.Fatalf("decorator missing: %+v", lineSpans(sp, 0))
	}
	if !hasType(lineSpans(sp, 2), TNumber) {
		t.Fatalf("number missing: %+v", lineSpans(sp, 2))
	}
}

func TestBashVariable(t *testing.T) {
	sp := lexLines(Detect("x.sh", ""), "echo $HOME")
	if !hasType(lineSpans(sp, 0), TVariable) {
		t.Fatalf("$HOME not a variable: %+v", lineSpans(sp, 0))
	}
}

func TestHTMLTagAttribute(t *testing.T) {
	sp := lexLines(Detect("x.html", ""), `<div class="x">hi</div>`)
	line := lineSpans(sp, 0)
	if !hasType(line, TTag) {
		t.Fatalf("tag missing: %+v", line)
	}
	if !hasType(line, TAttribute) {
		t.Fatalf("attribute missing: %+v", line)
	}
}

func TestMarkdownHeading(t *testing.T) {
	sp := lexLines(Detect("x.md", ""), "# Title\n\n`code`")
	if !hasType(lineSpans(sp, 0), THeading) {
		t.Fatalf("heading missing: %+v", lineSpans(sp, 0))
	}
	if !hasType(lineSpans(sp, 2), TString) {
		t.Fatalf("inline code missing: %+v", lineSpans(sp, 2))
	}
}

func TestDetectByExt(t *testing.T) {
	cases := map[string]string{
		"main.go":    "Go",
		"app.py":     "Python",
		"server.rs":  "Rust",
		"data.json":  "JSON",
		"index.html": "HTML",
		"style.css":  "CSS",
		"README.md":  "Markdown",
		"script.php": "PHP",
		"x.sh":       "Bash",
		"x.sql":      "SQL",
		"x.toml":     "TOML",
		"x.yaml":     "YAML",
	}
	for path, want := range cases {
		l := Detect(path, "")
		got := ""
		if l != nil {
			got = l.Name
		}
		if got == "" || !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
			t.Errorf("Detect(%q) = %q want something containing %q", path, got, want)
		}
	}
	if l := Detect("unknown.xyz", ""); l != nil {
		t.Errorf("unknown ext should be nil, got %v", l.Name)
	}
}

func TestDetectByShebang(t *testing.T) {
	if l := Detect("script", "#!/usr/bin/env python3"); l == nil {
		t.Fatal("shebang python not detected")
	} else if !strings.Contains(strings.ToLower(l.Name), "python") {
		t.Fatalf("got %q", l.Name)
	}
	if l := Detect("script", "#!/bin/bash"); l == nil {
		t.Fatal("shebang bash not detected")
	} else if !strings.Contains(strings.ToLower(l.Name), "shell") && !strings.Contains(strings.ToLower(l.Name), "bash") {
		t.Fatalf("got %q", l.Name)
	}
}

func TestHighlighterCacheLifecycle(t *testing.T) {
	hl := NewHighlighter(Detect("main.go", ""))
	hl.Invalidate(0, 1, func(i int) []rune { return []rune("package main") })
	// wait for the background lexer (debounced ~30ms)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if sp := hl.Spans(0, nil); len(sp) > 0 {
			if !hasType(sp, TKeyword) && !hasType(sp, TPreproc) {
				t.Fatalf("no keyword after async lex: %+v", sp)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for highlight")
		}
		time.Sleep(10 * time.Millisecond)
	}
	hl.Close()
}
