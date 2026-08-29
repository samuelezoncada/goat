package editor

import (
	"os"
	"path/filepath"
	"strings"
)

// HasTabs reports whether any buffers are open.
func (e *Editor) HasTabs() bool { return len(e.tabs) > 0 }

// OpenDir opens a directory in the file browser.
func (e *Editor) OpenDir(path string) {
	e.browser.OpenDir(path)
	if !e.HasTabs() {
		e.newTab()
	}
}

// OpenBrowser opens and focuses the file browser at its current root. Used by
// the CLI when goat is launched without any file arguments.
func (e *Editor) OpenBrowser() {
	if e.browser == nil {
		return
	}
	e.browser.open = true
	e.focus = FocusBrowser
	e.browser.rebuild()
}

// NewTab adds an empty buffer.
func (e *Editor) NewTab() { e.newTab() }

// OpenPath opens a file into a tab (exported for main).
func (e *Editor) OpenPath(path string) { e.openPath(path) }

// projectRoot is the folder goat was pointed at: an explicit directory
// argument, else the browser's root (the launch directory). It is what paths
// are shown relative to, and what the symbol index covers.
func (e *Editor) projectRoot() string {
	if e.root != "" {
		return e.root
	}
	if e.browser != nil && e.browser.root != "" {
		return e.browser.root
	}
	cwd, _ := os.Getwd()
	return cwd
}

// displayPath renders a buffer's name for the status bar: relative to the
// project root when the file lives inside it, absolute otherwise, so a file
// opened from elsewhere is never shown as a confusing "../../..".
func (e *Editor) displayPath(t *Tab) string {
	if t == nil {
		return ""
	}
	if t.path == "" {
		return t.name
	}
	root := e.projectRoot()
	if root == "" {
		return t.path
	}
	rel, err := filepath.Rel(root, t.path)
	if err != nil || rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return t.path
	}
	return rel
}

// tabRect tracks a tab's on-screen hit areas for mouse clicks.
type tabRect struct {
	index            int
	tabX0, tabX1     int // whole tab label area
	closeX0, closeX1 int // the close (x) button area
}

func (e *Editor) tabLabels() []string {
	labels := make([]string, len(e.tabs))
	for i, t := range e.tabs {
		n := t.name
		if t.dirty {
			n = "* " + n
		}
		if t.readOnly {
			n += " [ro]"
		}
		labels[i] = " " + n
	}
	return labels
}

func runeLen(s string) int { return len([]rune(s)) }

func (e *Editor) drawTabBar() {
	style := e.theme.Plain
	y := 0
	e.fillRow(0, e.width, y, style)
	e.tabRects = e.tabRects[:0]
	labels := e.tabLabels()
	if len(labels) == 0 {
		e.putStr(1, y, " goat ", style.Reverse(true))
		return
	}

	total := 0
	for _, l := range labels {
		total += runeLen(l) + 3
	}
	start := 0
	if total > e.width {
		// walk backwards from the active tab so it stays visible
		w := 0
		start = len(labels)
		for i := e.cur; i >= 0; i-- {
			w += runeLen(labels[i]) + 3
			if w > e.width {
				break
			}
			start = i
		}
		if start > e.cur {
			start = e.cur
		}
	}

	x := 0
	for i := start; i < len(labels) && x < e.width; i++ {
		lab := labels[i]
		if x+runeLen(lab)+3 > e.width {
			lab = truncateRunes(lab, e.width-x-3)
		}
		ls := style
		if i == e.cur {
			ls = style.Reverse(true)
		}
		tabX0 := x
		x = e.putStr(x, y, lab, ls)
		tabX1 := x
		e.drawCell(x, y, ' ', ls)
		x++
		closeX0 := x
		e.drawCell(x, y, '×', ls)
		x++
		closeX1 := x
		e.drawCell(x, y, ' ', ls)
		x++
		e.tabRects = append(e.tabRects, tabRect{index: i, tabX0: tabX0, tabX1: tabX1, closeX0: closeX0, closeX1: closeX1})
	}
}

// handleTabBarClick handles a mouse click on the tab bar.
func (e *Editor) handleTabBarClick(x int) bool {
	for _, r := range e.tabRects {
		if x >= r.closeX0 && x < r.closeX1 {
			e.cur = r.index
			e.closeCurrentTab()
			return true
		}
	}
	for _, r := range e.tabRects {
		if x >= r.tabX0 && x < r.tabX1 {
			e.cur = r.index
			return true
		}
	}
	return false
}

// openTab appends a tab; if the current tab is an untouched default buffer it
// is replaced instead.
func (e *Editor) openTab(t *Tab) {
	if len(e.tabs) == 1 {
		cur := e.tabs[0]
		if cur.name == "New Buffer" && !cur.dirty && cur.path == "" {
			// Stop the replaced buffer's highlighter, or its goroutine leaks.
			cur.close()
			e.tabs[0] = t
			e.cur = 0
			return
		}
	}
	e.tabs = append(e.tabs, t)
	e.cur = len(e.tabs) - 1
}

func (e *Editor) newTab() {
	e.openTab(newTabWith(e.config()))
}

// tabForPath returns the index of the tab already holding path, or -1.
func (e *Editor) tabForPath(path string) int {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for i, t := range e.tabs {
		if t.path != "" && samePath(t.path, abs) {
			return i
		}
	}
	return -1
}

// openPath loads a file into a tab, or switches to the tab that already has it
// open so two buffers can never disagree about the same file.
func (e *Editor) openPath(path string) {
	if i := e.tabForPath(path); i >= 0 {
		e.cur = i
		e.remember(path)
		e.statusf("%s", e.displayPath(e.tabs[i]))
		return
	}
	t, err := openTabWith(path, e.config())
	if err != nil {
		e.errorf("Error reading %s: %v", filepath.Base(path), cleanErr(err))
		return
	}
	e.remember(path)
	e.openTab(t)
	msg := sprintf("Read %d line%s", t.lineCount(), plural(t.lineCount()))
	switch {
	case t.readOnly:
		msg += "  [read-only]"
	case t.rawBytes:
		msg += "  [not UTF-8; bytes preserved]"
	}
	e.statusf("%s", msg)
}

func (e *Editor) closeCurrentTab() {
	if len(e.tabs) == 0 {
		return
	}
	// If it's the only tab and it's the pristine empty buffer, exit.
	t := e.tabs[e.cur]
	if len(e.tabs) == 1 && !t.dirty && t.path == "" && t.lineCount() == 1 && len(t.line(0)) == 0 {
		e.quit()
		return
	}
	if t.dirty {
		e.promptCloseTab(t)
		return
	}
	e.doCloseTab()
}

// promptCloseTab asks whether to save a modified buffer before closing it.
func (e *Editor) promptCloseTab(t *Tab) {
	e.beginPrompt(sprintf("Save the buffer for %s? (No will DISCARD changes) [Y/n/c] ", t.name), "", nil, func(ans string) {
		switch ans {
		case "", "y", "Y":
			if t.path == "" {
				e.beginPrompt("File Name to Write: ", t.name, nil, func(name string) {
					if name == "" {
						return
					}
					if e.writeTab(t, name) {
						e.doCloseTab()
					}
				})
				return
			}
			e.writeTabChecked(t, t.path, func(ok bool) {
				if ok {
					e.doCloseTab()
				}
			})
		case "n", "N":
			e.doCloseTab()
		default:
			e.statusf("Close cancelled")
		}
	})
}

func (e *Editor) doCloseTab() {
	if len(e.tabs) == 0 {
		return
	}
	t := e.tabs[e.cur]
	if len(e.tabs) == 1 {
		e.tabs = []*Tab{}
		e.cur = 0
		t.close()
		e.newTab()
		return
	}
	e.tabs = append(e.tabs[:e.cur], e.tabs[e.cur+1:]...)
	t.close()
	if e.cur >= len(e.tabs) {
		e.cur = len(e.tabs) - 1
	}
}

func (e *Editor) switchTab(dir int) {
	if len(e.tabs) < 2 {
		return
	}
	e.cur = (e.cur + dir + len(e.tabs)) % len(e.tabs)
}

// gotoTab activates the nth tab (0-based), if it exists.
func (e *Editor) gotoTab(n int) {
	if n < 0 || n >= len(e.tabs) {
		return
	}
	e.cur = n
}

// SetReadOnly marks every open buffer read-only (the --view flag).
func (e *Editor) SetReadOnly(ro bool) {
	for _, t := range e.tabs {
		t.readOnly = ro
	}
}
