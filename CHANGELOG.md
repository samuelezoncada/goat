# Changelog

All notable changes to goat are documented here.

## v0.1.0 - 2026-08-16

Initial release. A nano-inspired terminal text editor.

### Features

- Nano-compatible keybindings (`^X` exit, `^O`/`^S` save, `^W` search, `^\` replace,
  `^K`/`^U` cut/paste, `^Z`/`^Y` undo/redo, `^G` help, ...)
- Syntax highlighting for 200+ languages via chroma
- Multiple tabs
- Full-screen file browser with tree browsing
- Fuzzy file finder (`^P`)
- Go to definition / usages via universal-ctags (`Alt+D`)
- Mouse support: click, click+drag selection, double-click word select
- Block cursor (visible in all terminals)
- Scrollable, color-coded help page

### Known limits

- `Alt+D` (find definition/usages) requires universal-ctags to be installed
- Files are normalized to LF line endings when saved
- Undo history is capped at 2000 operations
