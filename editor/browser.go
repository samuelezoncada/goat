package editor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Entry is one visible row in the file browser.
type Entry struct {
	name    string
	path    string // absolute
	isDir   bool
	symlink bool
	depth   int
}

// Browser is a full-screen file browser rooted at a folder. Directories
// expand in place (lazily) to form a tree; only expanded folders are read
// from disk, so heavy directories cost nothing until opened.
type Browser struct {
	e          *Editor
	open       bool
	root       string // the tree root; navigation never goes above it
	expanded   map[string]bool
	entries    []Entry
	sel        int
	top        int
	showHidden bool
	err        string // last directory read error, shown in the header
	// double-click detection
	lastClick    int64
	lastClickRow int
}

func NewBrowser(e *Editor) *Browser {
	b := &Browser{e: e, expanded: map[string]bool{}, showHidden: true}
	b.root, _ = os.Getwd()
	return b
}

// rebuild recomputes the flat visible list from the root and the expanded set.
func (b *Browser) rebuild() {
	b.entries = b.entries[:0]
	b.err = ""
	b.appendChildren(b.root, 0)
	b.clamp()
}

// appendChildren appends the children of dirPath (at depth) and, recursively,
// the children of any expanded subdirectories.
func (b *Browser) appendChildren(dirPath string, depth int) {
	if depth > 32 {
		return // guard against a symlink loop
	}
	for _, en := range b.readDir(dirPath) {
		en.path = filepath.Join(dirPath, en.name)
		en.depth = depth
		b.entries = append(b.entries, en)
		if en.isDir && b.expanded[en.path] {
			b.appendChildren(en.path, depth+1)
		}
	}
}

// readDir lists one directory, dirs before files, hidden after visible. A
// symlink to a directory is listed as a directory so the tree can follow it.
func (b *Browser) readDir(dir string) []Entry {
	f, err := os.ReadDir(dir)
	if err != nil {
		// Surface the reason instead of showing an empty folder.
		b.err = cleanErr(err)
		return nil
	}
	var dirs, files []Entry
	for _, de := range f {
		name := de.Name()
		if !b.showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		en := Entry{name: name, isDir: de.IsDir()}
		if de.Type()&os.ModeSymlink != 0 {
			en.symlink = true
			if fi, err := os.Stat(filepath.Join(dir, name)); err == nil {
				en.isDir = fi.IsDir()
			}
		}
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

// toggleHidden shows or hides dot files.
func (b *Browser) toggleHidden() {
	b.showHidden = !b.showHidden
	b.rebuild()
	b.e.statusf("Hidden files: %v", b.showHidden)
}

func (e *Editor) drawBrowser() {
	// The browser is only shown while it has focus; editing hides it.
	if e.focus != FocusBrowser {
		e.browser.open = false
		return
	}
	// The browser covers the text pane, so the display rows of the previous
	// frame no longer describe what is on screen.
	e.rows = nil
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

// scroll moves the view by n rows, keeping the selection inside it.
func (b *Browser) scroll(n int) {
	b.top += n
	if b.top > len(b.entries)-1 {
		b.top = len(b.entries) - 1
	}
	if b.top < 0 {
		b.top = 0
	}
}

// jumpToPrefix moves the selection to the next entry starting with r.
func (b *Browser) jumpToPrefix(r rune) {
	if len(b.entries) == 0 {
		return
	}
	target := unicode.ToLower(r)
	for i := 1; i <= len(b.entries); i++ {
		idx := (b.sel + i) % len(b.entries)
		name := b.entries[idx].name
		if name != "" && unicode.ToLower([]rune(name)[0]) == target {
			b.sel = idx
			return
		}
	}
}

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
// selection to its parent row. At the top level it moves the root up one
// directory, so the tree is not a dead end.
func (b *Browser) collapseOrUp() {
	en := b.selEntry()
	if en == nil {
		b.rootUp()
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
		return
	}
	b.rootUp()
}

// rootUp re-roots the tree at the parent directory, keeping the old root
// selected and expanded.
func (b *Browser) rootUp() {
	parent := filepath.Dir(b.root)
	if parent == b.root || parent == "" {
		return
	}
	old := b.root
	b.root = parent
	b.expanded[old] = true
	b.rebuild()
	for i, en := range b.entries {
		if en.path == old {
			b.sel = i
			break
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

// --- file operations -----------------------------------------------------

// newFile prompts for a name and creates an empty file next to the selection.
func (b *Browser) newFile() {
	dir := b.targetDir()
	b.e.beginPrompt("New file: ", "", nil, func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			b.e.errorf("%s already exists", name)
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			b.e.errorf("Error creating %s: %v", name, err)
			return
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
		if err != nil {
			b.e.errorf("Error creating %s: %v", name, err)
			return
		}
		f.Close()
		b.rebuild()
		b.selectPath(path)
		b.e.statusf("Created %s", name)
	})
}

// newDir prompts for a name and creates a directory.
func (b *Browser) newDir() {
	dir := b.targetDir()
	b.e.beginPrompt("New directory: ", "", nil, func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		path := filepath.Join(dir, name)
		if err := os.Mkdir(path, 0755); err != nil {
			b.e.errorf("Error creating %s: %v", name, err)
			return
		}
		b.rebuild()
		b.selectPath(path)
		b.e.statusf("Created %s/", name)
	})
}

// rename prompts for a new name for the selected entry.
func (b *Browser) rename() {
	en := b.selEntry()
	if en == nil {
		return
	}
	old := en.path
	b.e.beginPrompt(sprintf("Rename %s to: ", en.name), en.name, nil, func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || name == filepath.Base(old) {
			return
		}
		dst := filepath.Join(filepath.Dir(old), name)
		if _, err := os.Stat(dst); err == nil {
			b.e.errorf("%s already exists", name)
			return
		}
		if err := os.Rename(old, dst); err != nil {
			b.e.errorf("Error renaming: %v", err)
			return
		}
		// Follow the rename in any tab that had the file open.
		for _, t := range b.e.tabs {
			if samePath(t.path, old) {
				abs, _ := filepath.Abs(dst)
				t.path = abs
				t.name = filepath.Base(dst)
			}
		}
		b.rebuild()
		b.selectPath(dst)
		b.e.statusf("Renamed to %s", name)
	})
}

// remove deletes the selected entry after confirmation. Directories must be
// empty, so a mis-key cannot wipe a tree.
func (b *Browser) remove() {
	en := b.selEntry()
	if en == nil {
		return
	}
	path, name, isDir := en.path, en.name, en.isDir
	label := sprintf("Delete %s? [y/N] ", name)
	if isDir {
		label = sprintf("Delete directory %s (must be empty)? [y/N] ", name)
	}
	b.e.beginPrompt(label, "", nil, func(ans string) {
		if ans != "y" && ans != "Y" {
			b.e.statusf("Not deleted")
			return
		}
		var err error
		if isDir {
			err = os.Remove(path) // fails unless empty, on purpose
		} else {
			err = os.Remove(path)
		}
		if err != nil {
			b.e.errorf("Error deleting %s: %v", name, cleanErr(err))
			return
		}
		delete(b.expanded, path)
		b.rebuild()
		b.e.InvalidateIndex()
		b.e.statusf("Deleted %s", name)
	})
}

// targetDir is the directory new entries are created in: the selection itself
// when it is an expanded directory, else its parent.
func (b *Browser) targetDir() string {
	en := b.selEntry()
	if en == nil {
		return b.root
	}
	if en.isDir {
		return en.path
	}
	return filepath.Dir(en.path)
}

func (b *Browser) selectPath(path string) {
	for i, en := range b.entries {
		if en.path == path {
			b.sel = i
			return
		}
	}
}

// draw renders the browser full-screen over the main area.
func (b *Browser) draw() {
	bg := b.e.theme.Plain
	headerStyle := bg.Reverse(true)
	selStyle := bg.Reverse(true)
	dirStyle := b.e.theme.Type
	heavyStyle := b.e.theme.Comment
	linkStyle := b.e.theme.Builtin
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
	header := " " + root + string(filepath.Separator)
	if b.err != "" {
		header += "   [" + b.err + "]"
	}
	b.e.putStr(1, y0, truncateRunes(header, w-1), headerStyle)

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
		switch {
		case en.isDir && skipDirs[en.name]:
			style = heavyStyle
		case en.isDir:
			style = dirStyle
		case en.symlink:
			style = linkStyle
		}
		if idx == b.sel {
			style = selStyle
		}
		b.e.fillRow(0, w, y, style)
		marker := " "
		if en.isDir {
			if b.expanded[en.path] {
				marker = "▾" // ▾
			} else {
				marker = "▸" // ▸
			}
		}
		label := strings.Repeat(" ", en.depth*2) + marker + " " + en.name
		if en.isDir {
			label += "/"
		}
		if en.symlink {
			label += " →"
		}
		b.e.putStr(1, y, truncateRunes(label, w-2), style)
	}
}
