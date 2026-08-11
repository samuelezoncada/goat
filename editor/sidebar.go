package editor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Entry struct {
	name  string
	isDir bool
}

// Sidebar is the file-explorer panel.
type Sidebar struct {
	e       *Editor
	open    bool
	root    string // navigation cannot go above this folder
	dir     string
	entries []Entry
	sel     int
	top     int
}

func NewSidebar(e *Editor) *Sidebar {
	s := &Sidebar{e: e}
	s.dir, _ = os.Getwd()
	s.root = s.dir
	s.refresh()
	return s
}

func (s *Sidebar) width() int { return sidebarW }

func (s *Sidebar) refresh() {
	s.entries = s.entries[:0]
	if s.dir != s.root {
		s.entries = append(s.entries, Entry{name: "..", isDir: true})
	}
	f, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	dirs := []Entry{}
	files := []Entry{}
	for _, de := range f {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		e := Entry{name: name, isDir: de.IsDir()}
		if de.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].name) < strings.ToLower(dirs[j].name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].name) < strings.ToLower(files[j].name) })
	s.entries = append(s.entries, dirs...)
	s.entries = append(s.entries, files...)
	if s.sel >= len(s.entries) {
		s.sel = len(s.entries) - 1
	}
	if s.sel < 0 {
		s.sel = 0
	}
}

func (s *Sidebar) selEntry() *Entry {
	if s.sel < 0 || s.sel >= len(s.entries) {
		return nil
	}
	return &s.entries[s.sel]
}

func (s *Sidebar) selectedPath() string {
	en := s.selEntry()
	if en == nil {
		return ""
	}
	return filepath.Join(s.dir, en.name)
}

func (s *Sidebar) toggle() {
	s.open = !s.open
	if s.open {
		s.e.focus = FocusSidebar
		s.refresh()
	} else {
		s.e.focusText()
	}
}

func (e *Editor) drawSidebar() {
	// The sidebar is only shown while it has focus; editing hides it.
	if e.focus != FocusSidebar {
		e.sidebar.open = false
		return
	}
	e.sidebar.draw()
}

func (s *Sidebar) moveUp() {
	if s.sel > 0 {
		s.sel--
	}
}

func (s *Sidebar) moveDown() {
	if s.sel+1 < len(s.entries) {
		s.sel++
	}
}

func (s *Sidebar) home() { s.sel = 0 }

func (s *Sidebar) end() { s.sel = len(s.entries) - 1 }

// enter opens the selected entry.
func (s *Sidebar) enter() {
	en := s.selEntry()
	if en == nil {
		return
	}
	if en.name == ".." {
		s.goUp()
		return
	}
	if en.isDir {
		s.dir = filepath.Join(s.dir, en.name)
		s.refresh()
		return
	}
	s.e.openPath(s.selectedPath())
	s.e.focusText()
}

func (s *Sidebar) goUp() {
	if s.dir == s.root {
		return
	}
	s.dir = filepath.Dir(s.dir)
	s.refresh()
}

// OpenDir points the sidebar at a directory and focuses it.
func (s *Sidebar) OpenDir(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			s.dir = abs
			s.root = abs
		}
	}
	s.refresh()
	s.sel = 0
	s.top = 0
	s.open = true
	s.e.focus = FocusSidebar
}

// draw renders the sidebar panel.
func (s *Sidebar) draw() {
	w := s.width()
	bg := s.e.theme.Plain
	headerStyle := bg.Reverse(true)
	selStyle := bg.Reverse(true)
	dirStyle := s.e.theme.Type
	fileStyle := bg

	x1 := w
	if x1 > s.e.width {
		x1 = s.e.width
	}
	s.e.fillRect(0, x1, s.e.mainTop(), s.e.mainTop()+s.e.mainHeight(), bg)
	s.e.fillRow(0, x1, s.e.mainTop(), headerStyle)
	name := filepath.Base(s.dir)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = s.dir
	}
	name += string(filepath.Separator)
	s.e.putStr(1, s.e.mainTop(), truncateRunes(" "+name, w-1), headerStyle)

	// vertical divider
	for y := s.e.mainTop(); y < s.e.mainTop()+s.e.mainHeight() && y < s.e.height; y++ {
		s.e.drawCell(w, y, ' ', bg.Reverse(true))
	}

	viewH := s.e.mainHeight() - 1
	if s.sel < s.top {
		s.top = s.sel
	}
	if s.sel >= s.top+viewH {
		s.top = s.sel - viewH + 1
	}
	if s.top < 0 {
		s.top = 0
	}

	for row := 0; row < viewH; row++ {
		idx := s.top + row
		y := s.e.mainTop() + 1 + row
		if idx >= len(s.entries) {
			break
		}
		en := s.entries[idx]
		style := fileStyle
		if en.isDir {
			style = dirStyle
		}
		if idx == s.sel {
			style = selStyle
		}
		s.e.fillRow(1, w, y, style)
		label := en.name
		if en.isDir {
			label += "/"
		}
		s.e.putStr(2, y, truncateRunes(label, w-3), style)
	}
}
