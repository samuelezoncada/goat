package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// stubProvider is a hand-wired SymbolProvider for testing the command layer.
type stubProvider struct {
	ready bool
	defs  map[string][]Loc
	uses  map[string][]Loc
}

func (s *stubProvider) Ready() bool  { return s.ready }
func (s *stubProvider) Build() error { return nil }
func (s *stubProvider) Definitions(sym string) []Loc {
	if s.defs != nil {
		return s.defs[sym]
	}
	return nil
}
func (s *stubProvider) Usages(sym string) []Loc {
	if s.uses != nil {
		return s.uses[sym]
	}
	return nil
}

func openTabAt(t *testing.T, path, content string) *Tab {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tb, err := OpenTab(path)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

func TestWordAtCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	tb := openTabAt(t, path, "package x\nfunc Foo(bar int) int {\n\treturn bar\n}\n")

	tb.cur = Pos{Line: 1, Col: 6} // inside "Foo"
	if got := wordAtCursor(tb); got != "Foo" {
		t.Fatalf("wordAtCursor = %q want Foo", got)
	}
	tb.cur = Pos{Line: 1, Col: 0} // on "func"
	if got := wordAtCursor(tb); got != "func" {
		t.Fatalf("wordAtCursor = %q want func", got)
	}
	tb.cur = Pos{Line: 3, Col: 1} // on the closing brace, not a word char
	if got := wordAtCursor(tb); got != "" {
		t.Fatalf("wordAtCursor on punctuation = %q want empty", got)
	}
}

func TestFindWordCol(t *testing.T) {
	if c := findWordCol("Foo", []rune("func Foo(bar int) int {")); c != 5 {
		t.Fatalf("col = %d want 5", c)
	}
	if c := findWordCol("Foo", []rune("  Foo  ")); c != 2 {
		t.Fatalf("col = %d want 2", c)
	}
	if c := findWordCol("Foo", []rune("foo FooFoo x")); c != -1 {
		t.Fatalf("partial matches must be isolated, got %d", c)
	}
	if c := findWordCol("Foo", []rune("")); c != -1 {
		t.Fatalf("empty line col = %d want -1", c)
	}
}

func TestParseCtags(t *testing.T) {
	out := []byte("!_TAG_FILE_SORTED\t0\n" +
		"Foo\tpkg/a.go\t/^func Foo()$/\tline:5\n" +
		"ctags: Warning: cannot open source file \"nope.go\"\n" +
		"Bar\tpkg/b.go\t/^type Bar struct/$/\tline:10\n")
	idx, lower := parseCtags(out, "/root")

	foo := idx["Foo"]
	if len(foo) != 1 || foo[0].Line != 4 {
		t.Fatalf("Foo parsed wrong: %+v", foo)
	}
	if !filepath.IsAbs(foo[0].File) || !strings.HasSuffix(foo[0].File, "pkg/a.go") {
		t.Fatalf("Foo file = %q", foo[0].File)
	}
	if len(lower["bar"]) != 1 {
		t.Fatalf("lower index missing Bar: %+v", lower["bar"])
	}
}

func TestCtagsIndexDefinitions(t *testing.T) {
	c := newCtagsIndex("/x")
	c.mu.Lock()
	c.idx = map[string][]Loc{"Foo": {{File: "/x/a.go", Line: 0}}}
	c.lower = map[string][]Loc{"foo": {{File: "/x/b.go", Line: 1}}}
	c.ready = true
	c.mu.Unlock()

	if got := c.Definitions("Foo"); len(got) != 1 || got[0].File != "/x/a.go" {
		t.Fatalf("exact lookup = %+v", got)
	}
	if got := c.Definitions("FOO"); len(got) != 1 || got[0].File != "/x/b.go" {
		t.Fatalf("case-insensitive fallback = %+v", got)
	}
}

func TestCtagsUsagesScan(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\nFoo()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "dep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "dep", "x.js"), []byte("Foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("Foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{0x00, 'F', 'o', 'o'}, 0o644); err != nil {
		t.Fatal(err)
	}

	locs := newCtagsIndex(root).Usages("Foo")
	var files []string
	for _, l := range locs {
		files = append(files, filepath.Base(l.File))
	}
	joined := strings.Join(files, ",")
	if !strings.Contains(joined, "main.go") || !strings.Contains(joined, ".gitignore") {
		t.Fatalf("missing usages: %v", files)
	}
	if strings.Contains(joined, "x.js") || strings.Contains(joined, "bin.dat") {
		t.Fatalf("should skip heavy/binary files: %v", files)
	}
}

func TestGotoSymbolUniqueDefJumpsInPlace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	tb := openTabAt(t, path, "package x\nfunc Foo() {}\n")
	tb.cur = Pos{Line: 1, Col: 6}

	e := &Editor{
		root: root,
		symProvider: &stubProvider{
			ready: true,
			defs:  map[string][]Loc{"Foo": {{File: path, Line: 1, Col: 5}}},
		},
	}
	e.tabs = []*Tab{tb}
	e.cur = 0

	e.gotoSymbol()
	if tb.cur.Line != 1 || tb.cur.Col != 5 {
		t.Fatalf("cursor = %+v want line 1 col 5", tb.cur)
	}
	if e.symLast.sym != "Foo" || e.symLast.view != viewDefs {
		t.Fatalf("symLast = %+v", e.symLast)
	}
}

func TestGotoSymbolManyOpensResults(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	tb := openTabAt(t, path, "package x\nfunc Foo() {}\n")
	tb.cur = Pos{Line: 1, Col: 6}

	other := filepath.Join(root, "b.go")
	e := &Editor{
		root: root,
		symProvider: &stubProvider{
			ready: true,
			defs:  map[string][]Loc{"Foo": {{File: path, Line: 1}, {File: other, Line: 0}}},
		},
	}
	e.tabs = []*Tab{tb}
	e.cur = 0

	e.gotoSymbol()
	if e.results == nil || e.mode != ModeResults {
		t.Fatalf("overlay not opened: results=%v mode=%v", e.results, e.mode)
	}
	if len(e.results.locs) != 2 {
		t.Fatalf("results = %d locs", len(e.results.locs))
	}
}

func TestGotoSymbolNoDef(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	tb := openTabAt(t, path, "package x\nfunc Foo() {}\n")
	tb.cur = Pos{Line: 1, Col: 6}

	e := &Editor{root: root, symProvider: &stubProvider{ready: true}}
	e.tabs = []*Tab{tb}
	e.cur = 0

	e.gotoSymbol()
	if !strings.Contains(e.msg, "No definition found") {
		t.Fatalf("msg = %q", e.msg)
	}
}

func TestGotoSymbolToggleToUsages(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	tb := openTabAt(t, path, "package x\nfunc Foo() {}\n")
	usePath := filepath.Join(root, "main.go")
	if err := os.WriteFile(usePath, []byte("package main\nFoo()\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}

	e := &Editor{
		screen: scr,
		root:   root,
		symProvider: &stubProvider{
			ready: true,
			defs:  map[string][]Loc{"Foo": {{File: path, Line: 1, Col: 5}}},
			uses:  map[string][]Loc{"Foo": {{File: usePath, Line: 1, Col: 0}}},
		},
	}
	e.tabs = []*Tab{tb}
	e.cur = 0
	tb.cur = Pos{Line: 1, Col: 6}

	// first press: unique definition jumps in place
	e.gotoSymbol()
	if tb.cur.Line != 1 || tb.cur.Col != 5 {
		t.Fatalf("after def jump cursor = %+v", tb.cur)
	}

	// second press on the same symbol: toggle to usages (async)
	e.gotoSymbol()
	if e.symLast.view != viewUsages {
		t.Fatalf("view = %v want usages", e.symLast.view)
	}

	var se symbolEvent
	deadline := time.After(2 * time.Second)
	for {
		ev := scr.PollEvent()
		if ie, ok := ev.(*tcell.EventInterrupt); ok {
			if v, ok := ie.Data().(symbolEvent); ok {
				se = v
				break
			}
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for usages event")
		default:
		}
	}
	if se.view != viewUsages || len(se.locs) != 1 {
		t.Fatalf("event = %+v", se)
	}
	e.onSymbolEvent(se)
	// a single usage jumps directly into the usage file
	if e.results != nil {
		t.Fatalf("expected direct jump, got results overlay: %+v", e.results)
	}
	if len(e.tabs) != 2 || e.cur != 1 {
		t.Fatalf("tabs = %d cur = %d", len(e.tabs), e.cur)
	}
	act := e.active()
	if act.path != usePath || act.cur.Line != 1 || act.cur.Col != 0 {
		t.Fatalf("active = %s cursor %+v", act.path, act.cur)
	}
}

func TestGotoSymbolQueuesUntilIndexBuilt(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	tb := openTabAt(t, path, "package x\nfunc Foo() {}\n")
	tb.cur = Pos{Line: 1, Col: 6}

	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	stub := &stubProvider{ready: false}
	stub.defs = map[string][]Loc{"Foo": {{File: path, Line: 1, Col: 5}}}

	e := &Editor{screen: scr, root: root, symProvider: stub}
	e.tabs = []*Tab{tb}
	e.cur = 0

	e.gotoSymbol()
	if e.symPending == nil {
		t.Fatal("lookup should be queued while index is building")
	}
	if !strings.Contains(e.msg, "Building symbol index") {
		t.Fatalf("msg = %q", e.msg)
	}

	// index build finishes; the queued lookup runs and must clear the message
	stub.ready = true
	e.onBuildDone(buildDone{})
	if e.symPending != nil {
		t.Fatal("pending lookup should be cleared after build")
	}
	if tb.cur.Line != 1 || tb.cur.Col != 5 {
		t.Fatalf("cursor = %+v want line 1 col 5", tb.cur)
	}
	if e.msg != "" {
		t.Fatalf("msg = %q want empty after same-file jump", e.msg)
	}
}

func TestBuildDoneReportsError(t *testing.T) {
	e := &Editor{}
	e.symPending = &symbolEvent{sym: "Foo", view: viewDefs}
	e.onBuildDone(buildDone{err: errTestBuild})
	if !strings.Contains(e.msg, "Symbol lookup failed") {
		t.Fatalf("msg = %q", e.msg)
	}
	if e.symPending != nil {
		t.Fatal("pending should be dropped on build error")
	}
}

// errTestBuild is a sentinel for build-failure tests.
var errTestBuild = &exec.Error{Name: "ctags", Err: os.ErrNotExist}

func TestResultsKeyNavAndClose(t *testing.T) {
	e := &Editor{}
	e.results = &Results{sym: "Foo", view: viewDefs, locs: []Loc{{File: "a"}, {File: "b"}}}
	e.resultsKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if e.results.sel != 1 {
		t.Fatalf("sel = %d want 1", e.results.sel)
	}
	e.resultsKey(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
	if e.results != nil {
		t.Fatal("Esc should close results")
	}
}

func TestCtagsLive(t *testing.T) {
	if _, err := exec.LookPath("ctags"); err != nil {
		t.Skip("universal-ctags not installed")
	}
	root := t.TempDir()
	path := filepath.Join(root, "greet.go")
	if err := os.WriteFile(path, []byte("package greet\n\n// Greet says hello.\nfunc Greet(name string) string {\n\treturn \"hi \" + name\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := newCtagsIndex(root)
	if err := c.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !c.Ready() {
		t.Fatal("index not ready")
	}

	defs := c.Definitions("Greet")
	if len(defs) != 1 {
		t.Fatalf("Greet defs = %+v", defs)
	}
	d := defs[0]
	if d.Line != 3 {
		t.Fatalf("Greet loc = %+v want line 3", d)
	}
	if !samePath(d.File, path) {
		t.Fatalf("file = %q want %q", d.File, path)
	}

	uses := c.Usages("Greet")
	if len(uses) == 0 {
		t.Fatal("no usages found")
	}
}
