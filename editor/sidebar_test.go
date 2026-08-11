package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSidebarRootBoundary(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	e := &Editor{}
	s := NewSidebar(e)
	s.root = base
	s.dir = base
	s.refresh()

	// At the root there must be no ".." entry.
	for _, en := range s.entries {
		if en.name == ".." {
			t.Fatal("root sidebar should not offer '..'")
		}
	}

	// goUp at root is a no-op.
	s.goUp()
	if s.dir != base {
		t.Fatalf("goUp escaped root: %s", s.dir)
	}

	// Inside a subfolder, ".." is present and works, but stops at root.
	s.dir = sub
	s.refresh()
	found := false
	for _, en := range s.entries {
		if en.name == ".." {
			found = true
		}
	}
	if !found {
		t.Fatal("subfolder should offer '..'")
	}
	s.goUp()
	if s.dir != base {
		t.Fatalf("goUp went to %s want %s", s.dir, base)
	}
	s.goUp() // must not escape
	if s.dir != base {
		t.Fatalf("second goUp escaped root: %s", s.dir)
	}
}

func TestSidebarOpenDirSetsRoot(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	e := &Editor{}
	s := NewSidebar(e)
	s.OpenDir(sub)
	if s.root != sub || s.dir != sub {
		t.Fatalf("root=%s dir=%s want both %s", s.root, s.dir, sub)
	}
	if !s.open || e.focus != FocusSidebar {
		t.Fatalf("sidebar not opened/focused")
	}
}

func TestSidebarHiddenWhileEditing(t *testing.T) {
	e := &Editor{}
	e.sidebar = NewSidebar(e)
	e.sidebar.open = true
	e.focus = FocusSidebar
	e.focusText()
	if e.sidebar.open {
		t.Fatal("sidebar should hide when focus returns to text")
	}
	if e.focus != FocusText {
		t.Fatalf("focus=%v want FocusText", e.focus)
	}
}

func TestDrawSidebarHidesWhenTextFocused(t *testing.T) {
	e := &Editor{}
	e.sidebar = NewSidebar(e)
	e.sidebar.open = true
	e.focus = FocusText
	e.drawSidebar()
	if e.sidebar.open {
		t.Fatal("drawSidebar must hide sidebar when text is focused")
	}
}
