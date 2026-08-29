package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJumpStackReturnsToOrigin(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	os.WriteFile(a, []byte("package a\n\nfunc Caller() { Target() }\n"), 0o644)
	os.WriteFile(b, []byte("package b\n\nfunc Target() {}\n"), 0o644)

	e := &Editor{cfg: DefaultConfig()}
	e.openPath(a)
	e.tabs[0].cur = Pos{2, 16}
	origin := e.tabs[0].cur

	e.jumpTo("Target", Loc{File: b, Line: 2, Col: 5})
	if len(e.tabs) != 2 || e.cur != 1 {
		t.Fatalf("jump should open the target file: tabs=%d cur=%d", len(e.tabs), e.cur)
	}
	e.jumpBack()
	if e.cur != 0 {
		t.Fatalf("jump back should return to the first tab, cur=%d", e.cur)
	}
	if e.tabs[0].cur != origin {
		t.Fatalf("cursor %v want %v", e.tabs[0].cur, origin)
	}
}

func TestJumpBackWithoutHistory(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.newTab()
	e.jumpBack()
	if e.msg == "" {
		t.Fatal("jumping back with no history should say so")
	}
}

func TestJumpToExistingTabDoesNotDuplicate(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	os.WriteFile(a, []byte("package a\n\nfunc Target() {}\n"), 0o644)
	e := &Editor{cfg: DefaultConfig()}
	e.openPath(a)
	e.newTab() // focus elsewhere
	e.jumpTo("Target", Loc{File: a, Line: 2, Col: 5})
	count := 0
	for _, tb := range e.tabs {
		if samePath(tb.path, a) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d tabs for the same file", count)
	}
	if e.cur != 0 {
		t.Fatalf("cur = %d, should switch to the existing tab", e.cur)
	}
}

// TestSaveUpdatesOnlyTheSavedFile: saving used to re-run ctags over the whole
// project.
func TestSaveUpdatesOnlyTheSavedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	os.WriteFile(path, []byte("package a\n"), 0o644)
	stub := &stubProvider{ready: true}
	e := &Editor{cfg: DefaultConfig(), symProvider: stub}
	e.openPath(path)
	tb := e.tabs[0]
	tb.insertRune('/')
	if !e.writeTab(tb, path) {
		t.Fatalf("write failed: %s", e.msg)
	}
	// The update runs in a goroutine; wait for it.
	var updated []string
	for i := 0; i < 200; i++ {
		if updated = stub.updatedFiles(); len(updated) > 0 {
			break
		}
		sleepMillis(5)
	}
	if len(updated) != 1 || !samePath(updated[0], tb.path) {
		t.Fatalf("updated = %v, want just the saved file", updated)
	}
	if e.symBuilding {
		t.Fatal("a save must not trigger a full index build")
	}
}

func TestUsagesCancellation(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 50; i++ {
		os.WriteFile(filepath.Join(dir, sprintf("f%d.go", i)), []byte("Target\n"), 0o644)
	}
	c := newCtagsIndex(dir)
	cancel := make(chan struct{})
	close(cancel) // cancelled before it starts
	locs := c.Usages("Target", cancel)
	if len(locs) > 5 {
		t.Fatalf("a cancelled scan should stop early, got %d hits", len(locs))
	}
}

func TestUsagesSortedAndComplete(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("x\nTarget\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("Target\n"), 0o644)
	c := newCtagsIndex(dir)
	locs := c.Usages("Target", nil)
	if len(locs) != 2 {
		t.Fatalf("locs %v", locs)
	}
	if filepath.Base(locs[0].File) != "a.go" {
		t.Fatalf("results are not sorted: %v", locs)
	}
	if locs[1].Line != 1 {
		t.Fatalf("line = %d want 1", locs[1].Line)
	}
}

func TestUsagesSkipsBinaryAndHugeFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ok.go"), []byte("Target\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "bin.dat"), []byte("Target\x00\n"), 0o644)
	c := newCtagsIndex(dir)
	locs := c.Usages("Target", nil)
	if len(locs) != 1 || filepath.Base(locs[0].File) != "ok.go" {
		t.Fatalf("locs %v, binary files must be skipped", locs)
	}
}

func TestResolveColUsesOpenBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	os.WriteFile(path, []byte("package a\nvar Old = 1\n"), 0o644)
	e := &Editor{cfg: DefaultConfig()}
	e.openPath(path)
	tb := e.tabs[0]
	// Edit the buffer without saving: the column must come from the buffer.
	tb.cur = Pos{1, 0}
	tb.insertRunes(1, 0, []rune("   "))
	loc := e.resolveCol("Old", Loc{File: path, Line: 1})
	if loc.Col != 7 {
		t.Fatalf("col = %d want 7 (from the unsaved buffer)", loc.Col)
	}
}
