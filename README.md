# goat

A nano-inspired terminal text editor written in Go, built on
[tcell](https://github.com/gdamore/tcell). Features:

- Nano-compatible keybindings (`^X` exit, `^O` save, `^W` search, `^\` replace,
  `^K`/`^U` cut/paste, `^G` help, ...)
- Syntax highlighting for 200+ languages via [chroma](https://github.com/alecthomas/chroma)
  (a Go port of Pygments), including Go, Rust, Python, JS/TS, C/C++, C#,
  Java, Ruby, PHP, Perl, Lua, Bash, PowerShell, SQL, HTML, CSS, JSON, YAML,
  TOML, Markdown, and more
- Multiple tabs
- Full-screen file browser with tree browsing
- Undo/redo, search with replace, mouse support, bracketed paste, auto-indent

## Build & run

```sh
go build -o goat .
./goat                    # empty buffer + file browser
./goat main.go            # open a file (browser closed)
./goat a.go b.go c.go     # multiple tabs
./goat ./src             # open a folder (file browser opens)
```

## Keybindings

```
Movement              Editing
^A/^E   Home/End      ^K / ^X   Cut line/selection
^B/^F   Left/Right    ^U / ^V   Paste
^N/^P   Down/Up       ^O / ^S   Save (Write Out)
^Y      Page Up       ^R        Read File
^W      Search        ^D        Delete forward
^\      Replace       ^H        Backspace
Alt+Arrows or Ctrl+Arrows  Word   ^J  Justify
                      ^I        Tab / auto-indent

Select
Shift+Arrows / Home/End/PgUp/PgDn  Extend selection
Alt+Space      Set/clear selection mark at cursor
^C             Copy selection
^K / ^X        Cut selection      Esc   Clear selection
Meta+A         Select all

Tabs
Ctrl+Tab / Ctrl+Shift+Tab   Next/Prev tab
Alt+T   New tab             Alt+W  Close tab (prompts to save)
Click the × on a tab to close it with the mouse
^Q      Exit (prompts to save; Alt+Q also exits)

File browser
Ctrl+B / Alt+S  Toggle browser (full-screen)
Alt+Tab     Move focus between text and browser
Enter       Open file (closes browser) / expand dir
Right or +  Expand dir       Left or -   Collapse / jump to parent
Backspace   Collapse / jump to parent

Other
^G          Help            Meta+A  Select all
^P          Find file       ^L      Refresh
Meta+Z      Undo            Meta+Y  Redo
Meta+D      Go to definition / usages (toggle), needs universal-ctags
```

In the search prompt: `^W` next, `Alt+Q` reverse, `Alt+C` toggles case
sensitivity, `Esc`/`^X` cancels. During replace: `y` = yes, `n` = no,
`a` = all.

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
  browser.go      file browser (full-screen tree)
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

## Tests

```sh
go test ./...   # buffer ops, undo/redo, search/replace, chroma highlighting, detection
```

## License

MIT — see [LICENSE](LICENSE). Copyright (c) 2026 Samuele Zoncada.
