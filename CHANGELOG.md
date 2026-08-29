# Changelog

All notable changes to goat are documented here.

## Unreleased

### Fixed — data loss

- **Saving a file that ends with a newline no longer appends a blank line.**
  The trailing newline was counted twice (once as an empty last line from the
  split, once from the `trailingNL` flag), so every open/save cycle grew the
  file by one line. Buffers no longer show that phantom last line either.
- **Files that are not valid UTF-8 are preserved byte for byte.** Undecodable
  bytes used to be replaced with U+FFFD and written back that way, silently
  corrupting Latin-1 and mixed-encoding files. They are now escaped into a
  private-use rune range, shown as `·`, flagged in the status bar, and written
  back unchanged.
- **Files containing NUL bytes are refused** instead of being opened and
  corrupted on save, as are files above 64 MB.
- **Saving is atomic**: goat writes a temporary file in the same directory,
  fsyncs it and renames it into place, so an interrupted save (crash, full
  disk) can no longer truncate the original. The file mode is preserved (an
  executable script stays executable) and a symlink is followed rather than
  replaced.
- **Saving warns when the file changed on disk** since it was read, instead of
  silently discarding the other writer's changes.
- **A crash or `SIGTERM`/`SIGHUP` no longer loses unsaved work or wrecks the
  terminal**: the event loop recovers from a panic, writes every modified
  buffer to `<file>.save`, and restores the terminal before reporting.

### Fixed — editing

- **Multi-line pastes keep their line breaks.** Only printable keys were
  collected during a bracketed paste, so newlines and tabs were dropped and the
  pasted lines ran together.
- **Mouse clicks and pastes no longer bypass an open overlay.** A paste during
  the search prompt went into the document, and a click while a prompt, the
  picker, the results list or the help page was open moved the hidden cursor or
  discarded the pending prompt. Every event type is now routed through the
  current mode.
- **Typing, Enter, Backspace, Delete and paste replace the active selection**
  instead of writing inside it and leaving it highlighted.
- **Undo/redo group whole actions.** Justify, replace-all and block indent used
  to need one undo per line or per occurrence; a run of typing or backspacing
  now collapses into a single step as well.
- **Case-insensitive and regex replacements undo correctly.** The undo record
  stored the typed pattern instead of the text actually matched, so undoing a
  replacement of `FOO` restored `foo`.
- **Replace-all covers the whole buffer.** It used to start at the cursor and
  silently skip everything above it. It also terminates on zero-width matches.
- **Cutting an empty line no longer wipes the clipboard**, and a stray
  zero-width selection no longer turns `^K` into a line cut.
- Recorded undo operations now own their text, so a later in-place edit cannot
  rewrite history; the buffer always keeps at least one line; the cursor and
  the mark are clamped after every edit and undo.
- Opening the same file twice switches to the tab that already holds it,
  instead of creating a second buffer that could overwrite the first.
- Replacing the pristine start buffer no longer leaks its highlighter
  goroutine.
- The mouse wheel no longer scrolls past the end of the buffer.
- Dismissing a prompt with `Esc` now unwinds the action that opened it, so a
  cancelled exit or replace no longer leaves the editor half way through it.
- The viewport follows the cursor again after a terminal resize.

### Performance

- **Typing in a large file is no longer dominated by the highlighter.** The
  whole buffer was re-serialized on every keystroke; snapshots now happen at
  most once every 25 ms and reuse the unchanged lines of the previous one. On a
  5000-line Go file a keystroke went from ~1.5 ms to ~3 µs, and the snapshot
  itself from ~1.5 ms to ~0.6 ms. Highlighting switches off above 4 MB, where a
  whole-buffer re-lex is not affordable.
- Rune widths are cached with an ASCII fast path (~7x faster per cell) instead
  of allocating a string per rune for every cell of every frame.
- The renderer starts at the horizontal scroll offset instead of walking each
  line from column 0.
- The file picker builds its index in the background (the UI no longer freezes
  on `^P` in a large tree), caches it between invocations, and filters ~2x
  faster with no per-candidate allocations.
- The symbol index is updated one file at a time on save instead of re-running
  `ctags -R` over the whole project; the usages scan runs in parallel, skips
  binaries and oversized files, and is cancelled when a newer lookup starts.
- The undo cap no longer reallocates the whole stack on every keystroke once
  the limit is reached.

### Added

- **The status line shows the current git branch** next to the filename
  (`git:main`, or the short commit for a detached HEAD). It is read directly
  from `.git/HEAD` rather than by running `git`, cached for two seconds, and
  resolves the `gitdir:` indirection used by linked worktrees and submodules.
  Disable it with `gitbranch = false`.
- **File names in the status line are relative to the folder goat was opened
  on** (`editor/render.go` instead of `/home/me/projects/goat/editor/render.go`),
  falling back to the absolute path for a file outside that folder. The project
  root now has a single definition, shared by the status line, the symbol index
  and the results overlay.
- On a narrow status line the path is now shortened from the front, so the
  filename stays visible, and the language/encoding/line-ending fields give way
  before the branch and cursor position do.
- **Configuration file** (`~/.config/goat/config`, `$GOAT_CONFIG` to override):
  `tabwidth`, `expandtab`, `autoindent`, `wrap`, `theme`, `clipboard`,
  `gitbranch`. Unknown
  keys and bad values are reported instead of ignored.
- **Light theme**, selectable with `theme = light`.
- **Soft wrap** (`wrap = true`), with cursor movement, scrolling and mouse
  mapping all working in wrapped space.
- **Grapheme cluster support**: combining accents, variation selectors, ZWJ
  emoji sequences and flags count as one character for the cursor, for
  deletion and for column arithmetic, and render in a single cell.
- **Search with regular expressions** (`Alt+R`, with `$1` capture references in
  the replacement), **whole-word matching** (`Alt+U`), highlighting of every
  match in the viewport, and replace-all restricted to the selection.
- **Block indent and dedent** with `Tab`/`Shift+Tab` over a selection, honouring
  `tabwidth` and `expandtab`.
- **System clipboard integration** (OSC 52) for cut and copy.
- **`Alt+B` jumps back** to the position before the last `Alt+D` or `Alt+G`.
- **`Alt+1`…`Alt+9`** select a tab directly, for terminals that swallow
  `Ctrl+Tab`.
- **File operations in the browser**: new file (`Alt+N`), new directory
  (`Alt+D`), rename (`Alt+R`), delete (`Alt+X`, empty directories only, with
  confirmation), hidden-file toggle (`^H`), and jump-to-letter. Unreadable
  directories now say why they are empty, and symlinked directories can be
  expanded.
- **Prompt history** (`↑`/`↓`) and **filename completion** (`Tab`).
- **Read-only buffers**: detected from the file permissions or requested with
  `--view`, shown in the tab bar and status line, and refused for edits rather
  than failing at save time.
- The file picker honours `.gitignore` (via `git ls-files`) when the project is
  a git work tree, and can be re-indexed with `^R`.
- The status line shows the encoding, the line-ending style and the display
  column alongside the rune column; `^J` justify now re-wraps the paragraph to
  the pane width instead of joining it into one long line.
- Middle-click pastes and Shift+click extends the selection.

### Changed

- The keybinding table is now the single source of truth: the help page, the
  status hints and a README consistency test are generated from it, so the
  documentation cannot drift from the implementation again.
- `^J` justify re-wraps rather than concatenating, and reports "Already
  justified" when there is nothing to do.
- A status message clears on the next keystroke, so it stops hiding the
  filename and cursor position.

### Development

- CI now runs `gofmt`, `go vet`, the tests, **and the race detector** on Linux,
  macOS and Windows, plus a 60 s fuzz run; the release workflow runs the tests
  before publishing and attaches `SHA256SUMS`.
- New tests cover the save round-trip (including permissions, symlinks,
  external changes and non-UTF-8 files), bracketed paste, event routing per
  mode, undo transactions, grapheme clusters, soft wrap, the config file,
  regex search and replace, the browser file operations, prompts, and rendering
  against a simulated screen. A model-based fuzz test compares the buffer
  against a reference implementation.

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
