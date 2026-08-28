package editor

import "strings"

// binding is one documented keybinding. This table is the single source of
// truth: the help page, the status-bar hints and the README consistency test
// all read it, so a binding cannot be documented in one place and forgotten in
// another.
type binding struct {
	section string
	keys    string
	desc    string
}

var bindings = []binding{
	{"Movement", "^A / ^E", "Home / End"},
	{"Movement", "^B / ^F", "Left / Right"},
	{"Movement", "^N / ^P", "Down / Up"},
	{"Movement", "Arrows", "Move"},
	{"Movement", "Alt+Up / Down", "Scroll"},
	{"Movement", "^Left / ^Right", "Word"},
	{"Movement", "Alt+Left / Right", "Word"},
	{"Movement", "Alt+G", "Go to line"},

	{"Editing", "^K / ^X", "Cut"},
	{"Editing", "^U / ^V", "Paste"},
	{"Editing", "^O / ^S", "Save"},
	{"Editing", "^R", "Read file"},
	{"Editing", "^W", "Search"},
	{"Editing", "^\\", "Replace"},
	{"Editing", "^D", "Delete forward"},
	{"Editing", "^H", "Backspace"},
	{"Editing", "^J", "Justify paragraph"},
	{"Editing", "^I / Tab", "Indent (block if selected)"},
	{"Editing", "Shift+Tab", "Dedent"},
	{"Editing", "Enter", "Newline"},
	{"Editing", "^Z / Alt+Z", "Undo"},
	{"Editing", "^Y / Alt+Y", "Redo"},

	{"Select", "Shift+Arrows", "Extend selection"},
	{"Select", "Alt+Space", "Mark on / off"},
	{"Select", "^C", "Copy selection"},
	{"Select", "Alt+A", "Select all"},
	{"Select", "Esc", "Clear selection"},

	{"Tabs", "Ctrl+Tab", "Next tab"},
	{"Tabs", "Ctrl+Shift+Tab", "Previous tab"},
	{"Tabs", "Alt+1..9", "Go to tab N"},
	{"Tabs", "Alt+T", "New tab"},
	{"Tabs", "Alt+W", "Close tab"},
	{"Tabs", "Click ×", "Close tab"},

	{"Files", "^P", "Find file (fuzzy)"},
	{"Files", "Ctrl+B / Alt+S", "Toggle browser"},
	{"Files", "Alt+Tab", "Focus browser / text"},
	{"Files", "Alt+D", "Definition / usages"},
	{"Files", "Alt+B", "Jump back"},

	{"Other", "^G", "This help"},
	{"Other", "^L", "Refresh screen"},
	{"Other", "^Q / Alt+Q", "Quit (prompts to save)"},

	{"Search prompt", "^W", "Next match"},
	{"Search prompt", "Alt+Q", "Reverse"},
	{"Search prompt", "Alt+C", "Case sensitivity"},
	{"Search prompt", "Alt+R", "Regular expression"},
	{"Search prompt", "Alt+U", "Whole word"},
	{"Search prompt", "Esc / ^X", "Cancel"},

	{"Replace", "y / n / a", "Yes / No / All"},
	{"Replace", "Selection", "Replace-all is limited to it"},

	{"Browser", "↑/↓", "Move"},
	{"Browser", "Enter", "Open / expand"},
	{"Browser", "Right / +", "Expand dir"},
	{"Browser", "Left / -", "Collapse / parent"},
	{"Browser", "Backspace", "Collapse / up"},
	{"Browser", "^H", "Toggle hidden files"},
	{"Browser", "Alt+N / Alt+D", "New file / directory"},
	{"Browser", "Alt+R", "Rename"},
	{"Browser", "Alt+X / Del", "Delete (empty dirs only)"},
	{"Browser", "a-z", "Jump to next name"},
	{"Browser", "Esc", "Close browser"},
}

// bindingSections returns the sections in table order with their rows.
func bindingSections() []helpSection {
	var out []helpSection
	index := map[string]int{}
	for _, b := range bindings {
		i, ok := index[b.section]
		if !ok {
			out = append(out, helpSection{title: b.section})
			i = len(out) - 1
			index[b.section] = i
		}
		out[i].rows = append(out[i].rows, helpRow{keys: b.keys, desc: b.desc})
	}
	return out
}

// hintBar is the bottom status line: the keys worth advertising, in the order
// they should appear, with short labels. Every entry must also exist in
// `bindings` (asserted by TestHintBarMatchesBindings), so the bar cannot
// advertise a key the keymap does not implement.
var hintBar = []struct{ keys, label string }{
	{"^Q / Alt+Q", "Exit"},
	{"^O / ^S", "Save"},
	{"^G", "Help"},
	{"^W", "Search"},
	{"^K / ^X", "Cut"},
	{"^U / ^V", "Paste"},
	{"^C", "Copy"},
	{"^\\", "Replace"},
	{"^P", "Files"},
	{"Ctrl+B / Alt+S", "Browser"},
	{"Alt+W", "CloseTab"},
}

// hintLine renders the bottom hint bar.
func hintLine() string {
	s := ""
	for _, h := range hintBar {
		s += h.keys + " " + h.label + "   "
	}
	return strings.TrimRight(s, " ")
}

// bindingKeys returns the set of documented key descriptions.
func bindingKeys() map[string]bool {
	out := make(map[string]bool, len(bindings))
	for _, b := range bindings {
		out[b.keys] = true
	}
	return out
}
