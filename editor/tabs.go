package editor

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
			e.tabs[0] = t
			return
		}
	}
	e.tabs = append(e.tabs, t)
	e.cur = len(e.tabs) - 1
	e.clearMsg()
}

func (e *Editor) newTab() {
	e.openTab(NewTab())
}

// openPath loads a file (or creates a new tab) into a tab.
func (e *Editor) openPath(path string) {
	t, err := OpenTab(path)
	if err != nil {
		e.statusf("Error reading %s: %v", path, err)
		return
	}
	e.remember(path)
	e.openTab(t)
	e.statusf("Read %d lines", t.lineCount())
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
					if err := t.saveTo(name); err != nil {
						e.statusf("Error writing: %v", err)
						return
					}
					e.doCloseTab()
				})
				return
			}
			if err := t.saveTo(t.path); err != nil {
				e.statusf("Error writing: %v", err)
				return
			}
			e.doCloseTab()
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
