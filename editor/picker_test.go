package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFuzzyMatchBasics(t *testing.T) {
	cases := []struct {
		query, s string
		want     bool
	}{
		{"main", "src/main.go", true},
		{"mcl", "src/main/conn/log.go", true},
		{"", "anything", true},
		{"xyz", "abc", false},
		{"abcdef", "abc", false},
		{"MAIN", "src/main.go", true},
	}
	for _, c := range cases {
		_, _, ok := fuzzyScore(c.query, c.s)
		if ok != c.want {
			t.Errorf("fuzzyScore(%q, %q) ok=%v want %v", c.query, c.s, ok, c.want)
		}
	}
}

func TestFuzzyRanking(t *testing.T) {
	// basename matches outrank path-only matches
	s1, _, ok1 := fuzzyScore("main", "src/main.go")
	if !ok1 {
		t.Fatal("no match")
	}
	s2, _, ok2 := fuzzyScore("main", "main/src/x.go")
	if !ok2 {
		t.Fatal("no match")
	}
	if s1 <= s2 {
		t.Errorf("basename match (%d) should outrank dir match (%d)", s1, s2)
	}

	// consecutive runs outrank scattered matches
	c1, _, _ := fuzzyScore("ab", "xx/a_b")
	c2, _, _ := fuzzyScore("ab", "axx/b")
	if c1 <= c2 {
		t.Errorf("consecutive match (%d) should outrank scattered (%d)", c1, c2)
	}
}

func TestFuzzyMatchPositions(t *testing.T) {
	_, pos, ok := fuzzyScore("ab", "xab")
	if !ok {
		t.Fatal("no match")
	}
	if len(pos) != 2 || pos[0] != 1 || pos[1] != 2 {
		t.Fatalf("positions %v", pos)
	}
}

// newTestPicker builds the file index synchronously (production builds it in
// the background) and returns a picker over it.
func newTestPicker(t *testing.T, e *Editor, root string) *Picker {
	t.Helper()
	idx := &fileIndex{root: root, expanded: map[string]bool{}}
	entries := walkFileIndex(root, idx.expanded)
	sortEntries(entries)
	idx.entries = entries
	idx.ready = true
	e.fileIndex = idx
	p := &Picker{e: e}
	e.picker = p
	p.refilter()
	return p
}

// reindex re-walks after an expand, mirroring what startIndex does async.
func (p *Picker) reindexForTest(t *testing.T) {
	t.Helper()
	idx := p.e.fileIndex
	entries := walkFileIndex(idx.root, idx.expanded)
	sortEntries(entries)
	idx.entries = entries
	p.refilter()
}

func indexKinds(idx *fileIndex) (files, dirs map[string]bool) {
	files, dirs = map[string]bool{}, map[string]bool{}
	for _, m := range idx.entries {
		if m.isDir {
			dirs[m.rel] = true
		} else {
			files[m.rel] = true
		}
	}
	return files, dirs
}

func TestBuildIndexShowsHeavyDirsUnexpanded(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "src", "pkg"))
	mustMkdir(t, filepath.Join(root, "node_modules", "dep"))
	mustMkdir(t, filepath.Join(root, ".git", "objects"))
	mustMkdir(t, filepath.Join(root, "vendor", "x"))
	mustMkdir(t, filepath.Join(root, ".config"))
	mustWrite(t, filepath.Join(root, "main.go"), "")
	mustWrite(t, filepath.Join(root, "src", "pkg", "a.go"), "")
	mustWrite(t, filepath.Join(root, "node_modules", "dep", "i.js"), "")
	mustWrite(t, filepath.Join(root, ".git", "objects", "x"), "")
	mustWrite(t, filepath.Join(root, "vendor", "x", "v.go"), "")
	mustWrite(t, filepath.Join(root, ".hidden"), "")
	mustWrite(t, filepath.Join(root, ".config", "settings.json"), "")

	e := &Editor{}
	newTestPicker(t, e, root)

	files, dirs := indexKinds(e.fileIndex)
	// hidden files are indexed, heavy dirs are listed but not walked
	for _, want := range []string{
		"main.go",
		filepath.Join("src", "pkg", "a.go"),
		".hidden",
		filepath.Join(".config", "settings.json"),
	} {
		if !files[want] {
			t.Errorf("file %s missing: %v", want, files)
		}
	}
	for _, want := range []string{"node_modules", ".git", "vendor"} {
		if !dirs[want] {
			t.Errorf("heavy dir %s not listed: %v", want, dirs)
		}
	}
	for _, bad := range []string{
		filepath.Join("node_modules", "dep", "i.js"),
		filepath.Join(".git", "objects", "x"),
		filepath.Join("vendor", "x", "v.go"),
	} {
		if files[bad] {
			t.Errorf("should not index %q", bad)
		}
	}
}

func TestPickerExpandDirIndexesContents(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "node_modules", "dep", "nested"))
	mustWrite(t, filepath.Join(root, "main.go"), "")
	mustWrite(t, filepath.Join(root, "node_modules", "dep", "i.js"), "")
	mustWrite(t, filepath.Join(root, "node_modules", "dep", "nested", "deep.js"), "")

	e := &Editor{}
	p := newTestPicker(t, e, root)

	idx := -1
	for i, m := range p.matches {
		if m.isDir && m.rel == "node_modules" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("node_modules dir match missing: %v", p.matches)
	}
	p.sel = idx
	p.expandDir(p.matches[idx])
	p.reindexForTest(t)

	files, dirs := indexKinds(e.fileIndex)
	if !files[filepath.Join("node_modules", "dep", "i.js")] {
		t.Error("i.js should be indexed after expand")
	}
	if !files[filepath.Join("node_modules", "dep", "nested", "deep.js")] {
		t.Error("deep.js should be indexed after expand")
	}
	if dirs["node_modules"] {
		t.Error("expanded dir should be removed from matches")
	}
}

func TestPickerMRUOrdering(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"zeta.go", "alpha.go", "beta.go", "main.go"} {
		mustWrite(t, filepath.Join(root, f), "")
	}
	e := &Editor{}
	e.recent = []string{
		filepath.Join(root, "main.go"),
		filepath.Join(root, "beta.go"),
	}
	p := newTestPicker(t, e, root)

	var order []string
	for _, m := range p.matches {
		order = append(order, m.rel)
	}
	// recent files first, then the rest alphabetically
	want := []string{"main.go", "beta.go", "alpha.go", "zeta.go"}
	if len(order) != len(want) {
		t.Fatalf("order %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v want %v", order, want)
		}
	}
}

func TestPickerScoreTiebreakByMRU(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "xo1.go"), "")
	mustWrite(t, filepath.Join(root, "xo2.go"), "")
	e := &Editor{}
	e.recent = []string{filepath.Join(root, "xo2.go")}
	p := newTestPicker(t, e, root)
	p.input = []rune("o") // equal score (both 'o' at index 1) -> MRU wins
	p.refilter()
	if len(p.matches) < 2 {
		t.Fatalf("matches %v", p.matches)
	}
	if p.matches[0].rel != "xo2.go" {
		t.Fatalf("first match %v want xo2.go", p.matches[0].rel)
	}
}

func TestRememberDedupCap(t *testing.T) {
	e := &Editor{}
	// remember stores the absolute path, so mirror that in expectations
	absJoin := func(parts ...string) string {
		p, _ := filepath.Abs(filepath.Join(parts...))
		return p
	}
	for i := 0; i < 40; i++ {
		e.remember(filepath.Join("/a", itoa(i)))
	}
	if len(e.recent) != 30 {
		t.Fatalf("cap: got %d want 30", len(e.recent))
	}
	// most recent is first
	if e.recent[0] != absJoin("/a", "39") {
		t.Fatalf("first %v", e.recent[0])
	}
	// re-remembering moves to front without duplicating
	e.remember(filepath.Join("/a", "5"))
	if len(e.recent) != 30 {
		t.Fatalf("dup: got %d", len(e.recent))
	}
	if e.recent[0] != absJoin("/a", "5") {
		t.Fatalf("front %v", e.recent[0])
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPasteIntoPickerFiltersQuery(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.fileIndex = &fileIndex{root: ".", expanded: map[string]bool{}, ready: true}
	e.picker = &Picker{e: e}
	e.mode = ModePicker
	pasteText(e, "abc")
	if string(e.picker.input) != "abc" {
		t.Fatalf("picker input = %q", string(e.picker.input))
	}
}
