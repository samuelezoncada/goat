# goat

A nano-inspired terminal text editor written in Go, built on
[tcell](https://github.com/gdamore/tcell).

## Features

- Nano-compatible keybindings (`^Q` exit, `^O`/`^S` save, `^W` search, `^\` replace,
  `^K`/`^U` cut/paste, `^Z`/`^Y` undo/redo, `^G` help)
- Syntax highlighting for 200+ languages via
  [chroma](https://github.com/alecthomas/chroma) (a Go port of Pygments)
- Multiple tabs, full-screen file browser with tree browsing and file
  operations, fuzzy file finder that respects `.gitignore`
- Go to definition / usages via [universal-ctags](https://github.com/universal-ctags/ctags),
  with `Alt+B` to jump back
- Search with regular expressions, whole-word matching and match highlighting
- Mouse support: click, shift+click, click+drag selection, double-click word
  select, middle-click paste, wheel scrolling
- Undo/redo with grouped actions, block indent/dedent, optional soft wrap,
  bracketed paste, system clipboard integration (OSC 52)
- Status line with the current git branch, encoding, line endings and position
- Safe saving: atomic writes, preserved file mode, symlinks followed, a warning
  when the file changed on disk, and emergency `.save` files on a crash or
  `SIGTERM`
- Lossless handling of files that are not valid UTF-8, and a refusal to open
  binaries rather than corrupting them

## Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Keybindings](#keybindings)
- [Status line](#status-line)
- [Configuration](#configuration)
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
- **git** — optional; when present, the file finder uses it to honour
  `.gitignore`

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
- If `^S` does nothing, your terminal is using it for flow control. Disable
  that with `stty -ixon` (add it to your shell profile), or use `^O` to save.
- `Ctrl+Tab`, `Alt+Tab` and `Alt+Space` are intercepted by many terminals and
  window managers. `Alt+1`…`Alt+9` select a tab directly, `Alt+T`/`Alt+W` open
  and close tabs, and `Alt+S` toggles the browser, so nothing depends on the
  intercepted combinations.
- Files are saved with **LF** line endings (CRLF files are normalized on save).

## Quick start

```sh
./goat                    # empty buffer + file browser
./goat main.go            # open a file (browser closed)
./goat a.go b.go c.go     # multiple tabs
./goat ./src              # open a folder (file browser opens)
./goat --view notes.txt   # open read-only
./goat --version          # print the version
```

## Keybindings

```
Movement                        Editing
^A / ^E   Home / End            ^K / ^X  Cut line/selection
^B / ^F   Left / Right          ^U / ^V  Paste
^N / ^P   Down / Up             ^C       Copy selection
Arrows    Move                  ^O / ^S  Save
Alt+Up / Alt+Down   Scroll      ^R       Read file
^Left / ^Right      Word        ^W       Search
Alt+Left / Alt+Right  Word      ^\       Replace
Alt+G     Go to line            ^D       Delete forward
                                ^H       Backspace
Select                          ^J       Justify paragraph
Shift+Arrows / Home / End       ^I / Tab Indent (block if selected)
PgUp / PgDn   Extend selection  Shift+Tab  Dedent
Alt+Space Mark on / off         Enter    Newline
Alt+A     Select all            ^Z / Alt+Z  Undo
Esc       Clear selection       ^Y / Alt+Y  Redo

Files                           Tabs
^P        Find file (fuzzy)     Ctrl+Tab        Next tab
Ctrl+B / Alt+S  Toggle browser  Ctrl+Shift+Tab  Previous tab
Alt+Tab   Focus browser / text  Alt+1..9        Go to tab N
Alt+D     Definition / usages   Alt+T           New tab
Alt+B     Jump back             Alt+W           Close tab
                                Click ×         Close tab
Other
^G        This help
^L        Refresh screen
^Q / Alt+Q  Quit (prompts to save)
```

In the search prompt: `^W` next match, `Alt+Q` reverse, `Alt+C` case
sensitivity, `Alt+R` regular expression, `Alt+U` whole word, `Esc`/`^X`
cancels. During replace: `y` yes, `n` no, `a` all. With regular expressions
on, the replacement may reference capture groups (`$1`, `${name}`). If a
Selection is active when replace starts, `a` only replaces inside it.

In the file browser:

```
↑ / ↓            Move                 Enter    Open file / expand dir
Right or +       Expand dir           Left or -  Collapse / jump to parent
Backspace        Collapse / up        a-z      Jump to next matching name
^H               Toggle hidden files  Esc      Close browser
Alt+N / Alt+D    New file / directory
Alt+R            Rename               Alt+X / Del  Delete (empty dirs only)
```

Prompts keep a history (`↑`/`↓`), and filename prompts complete paths with
`Tab`.

## Status line

The row above the shortcut hints shows, from the left: a `*` when the buffer
has unsaved changes, the file's path **relative to the folder goat was opened
on**, the current **git branch** (`git:main`, or the short commit when HEAD is
detached), and `[read-only]` where it applies.
On the right: the language, the encoding, the line-ending style and the cursor
position. On a narrow terminal the path is shortened from the front so the
filename stays visible, and the descriptive fields give way before the branch
and the position do.

Paths are shown relative to the project root — the directory passed on the
command line, or the launch directory when goat was started on a file. A file
opened from outside that folder keeps its absolute path, so nothing is ever
displayed as `../../..`.

The branch is read straight from `.git/HEAD` (no `git` process), cached for two
seconds, and works inside linked worktrees and submodules. Turn it off with
`gitbranch = false`.

## Configuration

goat reads an optional config file from `$XDG_CONFIG_HOME/goat/config`
(`~/.config/goat/config` by default; override with `$GOAT_CONFIG`). Run
`goat --help` to see the resolved path. The format is `key = value`, with `#`
comments:

```ini
# ~/.config/goat/config
tabwidth   = 4       # display width of a tab, and one indent step (1..32)
expandtab  = false   # insert spaces instead of a tab character
autoindent = true    # copy the previous line's indentation on Enter
wrap       = false   # soft-wrap long lines instead of scrolling sideways
theme      = dark    # dark | light
clipboard  = true    # copy to the terminal clipboard as well (OSC 52)
gitbranch  = true    # show the current git branch in the status bar
```

Unknown keys and bad values are reported in the status bar and leave the
default in place.

## Find definition / usages

Press `Alt+D` on a symbol to jump to its definition; press it again (or while
the results list is open) to toggle to all usages. `Alt+B` jumps back to where
you were. This needs
[universal-ctags](https://github.com/universal-ctags/ctags) on your `PATH` —
everything else in goat works without it.

The index is built once in the background and then updated one file at a time
as you save, so saving stays instant in a large project.

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
  buffer.go      Text interface, line-slice implementation, UTF-8 escaping
  tab.go         per-document state (cursor, viewport, editing, saving)
  undo.go        op-log undo/redo stack with grouped actions
  config.go      config file parsing
  bindings.go    the keybinding table (help page + hint bar are generated)
  search.go      search/replace: plain, whole-word, regular expressions
  render.go      cell-buffered renderer with dirty-cell diffing and soft wrap
  keymap.go      keybindings, event loop dispatch
  browser.go     file browser (full-screen tree, file operations)
  picker.go      fuzzy file finder (background index, .gitignore aware)
  symbols.go     find definition/usages (ctags-backed SymbolProvider)
  results.go     definitions/usages overlay
  prompt.go      prompt input, history, path completion
  util.go        rune widths and grapheme cluster stepping
syntax/          syntax highlighting backed by chroma lexers
  syntax.go      chroma-backed Highlighter (async re-lex, per-line span cache)
  theme.go       token-type → color themes (dark and light)
```

The buffer is exposed through a small `Text` interface so the line-slice
implementation can be swapped for a rope/piece-table later. Highlighting uses
the maintained [chroma](https://github.com/alecthomas/chroma) lexers: the
buffer is snapshotted at most once every 25 ms (reusing the unchanged lines of
the previous snapshot), re-lexed in a background goroutine, and cached as
per-line spans, so keystrokes stay instant and the renderer only rewrites
changed cells. Highlighting switches itself off above 4 MB, where a whole-buffer
re-lex stops being affordable.

Editing and rendering step by **grapheme cluster**, so a combining accent, a
variation selector, a ZWJ emoji sequence or a flag counts as one character for
the cursor, for deletion and for column arithmetic.

Bytes that are not valid UTF-8 are escaped into a private-use rune range on
load and written back verbatim on save, so goat round-trips a Latin-1 or
mixed-encoding file byte for byte instead of replacing what it cannot decode.
Files containing NUL bytes are refused rather than corrupted.

## Development

```sh
make test        # go test ./...
make test-race   # go test -race ./...
make vet         # go vet ./...
make check       # gofmt check + vet + race tests
make build       # build bin/goat with the version stamped in
make release     # cross-compile release tarballs into dist/
```

Tests cover the buffer and undo model (including transactions and grouped
actions), saving (round-trip, atomicity, permissions, symlinks, external
changes, non-UTF-8 files), search and replace, event routing per mode,
bracketed paste, mouse selection, soft wrap, grapheme clusters, the file
browser and its file operations, the fuzzy finder, the ctags layer, prompts,
rendering against a simulated screen, and a randomized editing fuzz test. CI
runs `gofmt`, `vet`, and the test suite with and without the race detector on
Linux, macOS, and Windows (including the live ctags test).

## License

MIT — see [LICENSE](LICENSE). Copyright (c) 2026 Samuele Zoncada.
