# Changelog

All notable changes to goat are documented here.

## v0.1.1 - 2026-08-20

### Fixed

- Undoing a newline now also removes the copied auto-indent instead of leaving
  stray whitespace behind.
- Undo/redo now correctly clears the modified (`*`) flag when the buffer is
  reverted to its last saved content, so exiting no longer prompts to save.
- Saving seals the undo group, so typing after a save undoes back to the saved
  content instead of merging with pre-save edits.
- Moving up/down or paging across lines of different lengths now preserves the
  cursor column (the intended `destCol` logic was previously dead code).
- `^P` is documented as Find file (the dead "move up" binding is removed) and
  `^Y` as Redo, matching the implementation.
- `Alt+Up` / `Alt+Down` now scroll the text viewport as advertised in the help
  page (previously unimplemented).
- Pasting or reading in multi-line text no longer leaves the cursor at an
  out-of-range column; typing right after such a paste previously crashed with
  a slice-bounds panic.
- Replace-all no longer loops forever when the replacement text contains the
  search text (e.g. replacing `a` with `aa`).
- Search and replace now match at the correct column on lines containing
  multi-byte (non-ASCII) characters; previously the cursor landed at the wrong
  position because byte offsets were used as rune columns.
- Cutting an empty buffer or a zero-width selection no longer marks the buffer
  modified.
- Long search/filename input now scrolls in the prompt and file picker instead
  of hiding the caret off-screen.
- Closing the last tab now stops its background highlighter goroutine instead
  of leaking it.
- Reading an empty file no longer marks the buffer modified.
- Launching `goat` with no arguments now opens the file browser as documented.
- `Alt+G` (go to line) is now listed in the help page.

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
