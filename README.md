# goat

A nano-inspired terminal text editor written in Go, built on
[tcell](https://github.com/gdamore/tcell).

## Features

- Nano-compatible keybindings (`^X` exit, `^O`/`^S` save, `^W` search, `^\` replace,
  `^K`/`^U` cut/paste, `^Z`/`^Y` undo/redo, `^G` help)
- Syntax highlighting for 200+ languages via
  [chroma](https://github.com/alecthomas/chroma) (a Go port of Pygments)
- Multiple tabs, full-screen file browser with tree browsing, fuzzy file finder
- Go to definition / usages via [universal-ctags](https://github.com/universal-ctags/ctags)
- Mouse support: click, click+drag selection, double-click word select
- Undo/redo, search with replace, bracketed paste, auto-indent

## Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Keybindings](#keybindings)
- [Find definition / usages](#find-definition--usages)
- [Architecture](#architecture)
- [Development](#development)
- [License](#license)

## Installation

### Requirements

- **Go 1.24+** to build from source
- A **256-color terminal with mouse support** (most do — see
  [Terminal setup](#terminal-setup))
- **[universal-ctags](https://github.com/universal-ctags/ctags)** — optional,
  only needed for `Alt+D` (find definition / usages)

### From source

```sh
make build        # builds bin/goat with the version stamped in
# or
go build -o goat .
```

`go install .` (or `make install`) installs goat onto your Go path. Prebuilt
binaries for Linux, macOS, and Windows are attached to every
[GitHub release](https://github.com/samuelezoncada/goat/releases).

### Terminal setup

- Most 256-color terminals work out of the box.
- Under **tmux**, enable the mouse with `set -g mouse on` so click and drag
  selection work.
- Files are saved with **LF** line endings (CRLF files are normalized on save).

## Quick start

```sh
./goat                    # empty buffer + file browser
./goat main.go            # open a file (browser closed)
./goat a.go b.go c.go     # multiple tabs
./goat ./src              # open a folder (file browser opens)
./goat --version          # print the version
```

## Keybindings

```
Movement            Editing
^A/^E  Home/End     ^K / ^X  Cut line/selection
^B/^F  Left/Right   ^U / ^V  Paste
^N     Down         ^O / ^S  Save
Arrows Move         ^R       Read file
Alt+Up/Down Scroll  ^W       Search
^Left/^Right  Word  ^\       Replace
                    ^D       Delete forward
                    ^H       Backspace
                    ^J       Justify
                    ^I       Tab / auto-indent
                    Enter    Newline

Select
Shift+Arrows / Home/End/PgUp/PgDn   Extend selection
Alt+Space     Mark on / off         Esc   Clear
^C            Copy selection
^K / ^X       Cut selection
Alt+A         Select all

Tabs
Ctrl+Tab / Ctrl+Shift+Tab   Next / previous tab
Alt+T    New tab            Alt+W   Close tab (prompts to save)
Click ×  Close tab          ^Q      Exit (Alt+Q also exits)

File browser
Ctrl+B / Alt+S  Toggle browser (full-screen)
Alt+Tab         Move focus between text and browser
Enter           Open file (closes browser) / expand dir
Right or +      Expand dir
Left or -       Collapse / jump to parent
Backspace       Collapse / jump to parent

Other
^G  Help                ^L  Refresh screen
^P  Find file           ^Z / Alt+Z  Undo
Alt+D  Go to definition / usages    ^Y / Alt+Y  Redo
Alt+G  Go to line
```

In the search prompt: `^W` next, `Alt+Q` reverse, `Alt+C` toggles case
sensitivity, `Esc`/`^X` cancels. During replace: `y` yes, `n` no, `a` all.

## Find definition / usages

Press `Alt+D` on a symbol to jump to its definition; press it again (or while
the results list is open) to toggle to all usages. This needs
[universal-ctags](https://github.com/universal-ctags/ctags) on your `PATH` —
everything else in goat works without it.

Install universal-ctags:

| Platform  | Command / steps                                                                                     |
|-----------|-----------------------------------------------------------------------------------------------------|
| Debian/Ubuntu | `apt install universal-ctags`                                                                     |
| macOS     | `brew install universal-ctags`                                                                      |
| Windows   | `choco install universal-ctags -y`, **or** download the prebuilt zip from the [`ctags-win32`](https://github.com/universal-ctags/ctags-win32) releases, extract it, and add that folder to `PATH` |

## Architecture

```
main.go          CLI entry
editor/          editor core (buffers, tabs, keymap, browser, rendering)
  buffer.go      Text interface + line-slice implementation
  tab.go         per-document state (cursor, viewport, editing ops)
  undo.go        op-log undo/redo stack
  search.go      forward/reverse search + replace
  render.go      cell-buffered renderer with dirty-cell diffing
  keymap.go      nano keybindings, event loop dispatch
  browser.go     file browser (full-screen tree)
  picker.go      fuzzy file finder
  symbols.go     find definition/usages (ctags-backed SymbolProvider)
  results.go     definitions/usages overlay
  prompt.go      prompt input state machine
syntax/          syntax highlighting backed by chroma lexers
  syntax.go      chroma-backed Highlighter (async re-lex, per-line span cache)
  theme.go       token-type → color theme
```

The buffer is exposed through a small `Text` interface so the line-slice
implementation can be swapped for a rope/piece-table later. Highlighting uses
the maintained [chroma](https://github.com/alecthomas/chroma) lexers: the whole
buffer is re-lexed in a background goroutine (debounced) and per-line spans are
cached, so keystrokes stay instant and the renderer only rewrites changed
cells. Language detection uses chroma's filename/shebang matching.

## Development

```sh
make test        # go test ./...
make test-race   # go test -race ./...
make vet         # go vet ./...
make build       # build bin/goat with the version stamped in
make release     # cross-compile release tarballs into dist/
```

Tests cover buffer ops, undo/redo, search/replace, mouse selection, the file
browser, the ctags parser, and chroma highlighting. CI runs vet + tests on
Linux, macOS, and Windows (including the live ctags test).

## License

MIT — see [LICENSE](LICENSE). Copyright (c) 2026 Samuele Zoncada.
