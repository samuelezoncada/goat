package editor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one visible row in the file browser.
type Entry struct {
	name  string
	path  string // absolute
	isDir bool
	depth int
}

// Browser is a full-screen file browser rooted at a folder. Directories
// expand in place (lazily) to form a tree; only expanded folders are read
// from disk, so heavy directories cost nothing until opened.
type Browser struct {
	e        *Editor
	open     bool
	root     string // the tree root; navigation never goes above it
	expanded map[string]bool
	entries  []Entry
	sel      int
	top      int
	// double-click detection
	lastClick    int64
	lastClickRow int
}

func NewBrowser(e *Editor) *Browser {
	b := &Browser{e: e, expanded: map[string]bool{}}
	b.root, _ = os.Getwd()
	b.rebuild()
	return b
}

// rebuild recomputes the flat visible list from the root and the expanded set.
func (b *Browser) rebuild() {
	b.entries = b.entries[:0]
	b.appendChildren(b.root, 0)
	b.clamp()
}

// appendChildren appends the children of dirPath (at depth) and, recursively,
// the children of any expanded subdirectories.
func (b *Browser) appendChildren(dirPath string, depth int) {
	for _, en := range b.readDir(dirPath) {
		en.path = filepath.Join(dirPath, en.name)
		en.depth = depth
		b.entries = append(b.entries, en)
		if en.isDir && b.expanded[en.path] {
			b.appendChildren(en.path, depth+1)
		}
	}
}

// readDir lists one directory, dirs before files, hidden after visible.
func (b *Browser) readDir(dir string) []Entry {
	f, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs, files []Entry
	for _, de := range f {
		en := Entry{name: de.Name(), isDir: de.IsDir()}
		if en.isDir {
			dirs = append(dirs, en)
		} else {
			files = append(files, en)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return entryLess(dirs[i].name, dirs[j].name) })
	sort.Slice(files, func(i, j int) bool { return entryLess(files[i].name, files[j].name) })
	return append(dirs, files...)
}

// entryLess orders hidden (dot-prefixed) names after visible ones,
// alphabetically (case-insensitive) within each group.
func entryLess(a, b string) bool {
	ah, bh := strings.HasPrefix(a, "."), strings.HasPrefix(b, ".")
	if ah != bh {
		return !ah
	}
	return strings.ToLower(a) < strings.ToLower(b)
}

func (b *Browser) clamp() {
	if b.sel >= len(b.entries) {
		b.sel = len(b.entries) - 1
	}
	if b.sel < 0 {
		b.sel = 0
	}
}

func (b *Browser) selEntry() *Entry {
	if b.sel < 0 || b.sel >= len(b.entries) {
		return nil
	}
	return &b.entries[b.sel]
}

func (b *Browser) toggle() {
	b.open = !b.open
	if b.open {
		b.e.focus = FocusBrowser
		b.rebuild()
	} else {
		b.e.focusText()
	}
}

func (e *Editor) drawBrowser() {
	// The browser is only shown while it has focus; editing hides it.
	if e.focus != FocusBrowser {
		e.browser.open = false
		return
	}
	e.browser.draw()
}

func (b *Browser) moveUp() {
	if b.sel > 0 {
		b.sel--
	}
}

func (b *Browser) moveDown() {
	if b.sel+1 < len(b.entries) {
		b.sel++
	}
}

func (b *Browser) home() { b.sel = 0 }

func (b *Browser) end() { b.sel = len(b.entries) - 1 }

// enter opens a file, or toggles a directory's expansion.
func (b *Browser) enter() {
	en := b.selEntry()
	if en == nil {
		return
	}
	if en.isDir {
		b.toggleExpand()
		return
	}
	b.e.openPath(en.path)
	b.e.focusText()
}

func (b *Browser) toggleExpand() {
	en := b.selEntry()
	if en == nil || !en.isDir {
		return
	}
	b.expanded[en.path] = !b.expanded[en.path]
	b.rebuild()
}

// expand opens the selected directory.
func (b *Browser) expand() {
	en := b.selEntry()
	if en == nil || !en.isDir || b.expanded[en.path] {
		return
	}
	b.expanded[en.path] = true
	b.rebuild()
}

// collapseOrUp collapses the selected expanded directory, or moves the
// selection to its parent row.
func (b *Browser) collapseOrUp() {
	en := b.selEntry()
	if en == nil {
		return
	}
	if en.isDir && b.expanded[en.path] {
		b.expanded[en.path] = false
		b.rebuild()
		return
	}
	if en.depth > 0 {
		parent := filepath.Dir(en.path)
		for i := b.sel - 1; i >= 0; i-- {
			if b.entries[i].isDir && b.entries[i].path == parent {
				b.sel = i
				return
			}
		}
	}
}

// OpenDir points the browser at a directory, resets the tree, and focuses it.
func (b *Browser) OpenDir(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			b.root = abs
		}
	}
	b.expanded = map[string]bool{}
	b.sel = 0
	b.top = 0
	b.rebuild()
	b.open = true
	b.e.focus = FocusBrowser
}

// draw renders the browser full-screen over the main area.
func (b *Browser) draw() {
	bg := b.e.theme.Plain
	headerStyle := bg.Reverse(true)
	selStyle := bg.Reverse(true)
	dirStyle := b.e.theme.Type
	heavyStyle := b.e.theme.Comment
	fileStyle := bg

	w := b.e.width
	y0 := b.e.mainTop()
	viewH := b.e.mainHeight() - 1

	b.e.fillRect(0, w, y0, y0+b.e.mainHeight(), bg)
	b.e.fillRow(0, w, y0, headerStyle)
	root := filepath.Base(b.root)
	if root == "" || root == "." || root == string(filepath.Separator) {
		root = b.root
	}
	b.e.putStr(1, y0, truncateRunes(" "+root+string(filepath.Separator), w-1), headerStyle)

	if b.sel < b.top {
		b.top = b.sel
	}
	if b.sel >= b.top+viewH {
		b.top = b.sel - viewH + 1
	}
	if b.top < 0 {
		b.top = 0
	}

	for row := 0; row < viewH; row++ {
		idx := b.top + row
		y := y0 + 1 + row
		if idx >= len(b.entries) {
			b.e.fillRow(0, w, y, bg)
			continue
		}
		en := b.entries[idx]
		style := fileStyle
		if en.isDir {
			style = dirStyle
			if skipDirs[en.name] {
				style = heavyStyle
			}
		}
		if idx == b.sel {
			style = selStyle
		}
		b.e.fillRow(0, w, y, style)
		marker := " "
		if en.isDir {
			if b.expanded[en.path] {
				marker = "\u25be" // ▾
			} else {
				marker = "\u25b8" // ▸
			}
		}
		label := strings.Repeat(" ", en.depth*2) + marker + " " + en.name
		if en.isDir {
			label += "/"
		}
		b.e.putStr(1, y, truncateRunes(label, w-2), style)
	}
}
