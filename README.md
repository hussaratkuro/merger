# merger

`merger` is a keyboard-first Catppuccin Mocha diff, merge, and directory
comparison TUI written in Go. It is a standalone companion to `svntui`, but it
also works directly from a shell.

## Build and install

```bash
go build -o merger .
go install .
```

`svntui` finds `merger` either through `PATH` or beside its own executable.

## Usage

Launch without arguments (for example from Rofi) to open the built-in file
picker:

```bash
merger
```

The picker starts in the home directory. `Enter` opens a directory or selects a
file, while `Space` or `Tab` selects the highlighted file or directory without
opening it. `Backspace` moves to the parent. After two
files or two directories are selected, their comparison opens automatically.
Start typing to jump to the first entry whose name begins with the typed text.
`Backspace` edits an active search before navigating upward, `Esc` clears it,
and the search resets automatically after a short pause. Press `.` to show or
hide dotfiles.

Compare two files:

```bash
merger path/to/left.txt path/to/right.txt
```

Compare two directories:

```bash
merger path/to/left-directory path/to/right-directory
```

Perform a three-way merge using Meld-compatible positional argument order:

```bash
merger mine.txt base.txt theirs.txt --output=result.txt
```

The explicit equivalent is:

```bash
merger --mine mine.txt --base base.txt --theirs theirs.txt --output result.txt
```

## File comparison

- The complete files stay available; unchanged sections are not collapsed.
- Added, deleted, modified, and conflicting regions use the Catppuccin
  green/red/yellow/red convention.
- `Alt+Up` and `Alt+Down` select the previous or next change.
- `Alt+Right` pushes the selected left change to the right; `Alt+Left` pushes
  right to left, matching Meld's arrow direction.
- A pushed change stays at its original screen location instead of selecting the
  next diff. Pending target lines remain highlighted with a `◆` marker until a
  successful save.
- The right-side whole-file overview uses prominent double-width blocks for
  changes and the visible viewport.
- The comparison panes are directly editable: click a character to place the
  active cursor, or move it with the arrow keys, then type immediately. `Enter`,
  `Backspace`, and `Delete` edit the document without opening another screen.
- `Tab` changes the focused pane. `Alt+Delete` removes the selected change from
  that pane.
- `Ctrl+Z` undoes pushes and edits. `Ctrl+Shift+Z` redoes them; `Ctrl+Y` is a
  terminal-compatible redo fallback.
- Changes remain in memory until `Ctrl+S`, and writes replace files atomically.
- Binary files support whole-file left/right pushes.

## Three-way merge

The main view shows `MINE | MERGED RESULT | THEIRS` across the entire file.
Disjoint and identical edits merge automatically. Overlapping edits remain
explicit conflicts.

- `Alt+Right` or `m`: MINE into the result
- `Alt+Left` or `t`: THEIRS into the result
- `b`: BASE into the result
- `a`: MINE followed by THEIRS
- `u`: restore conflict markers
- `e`: manually edit the selected result chunk
- `Ctrl+S`: save, only after every conflict is resolved

In three-way mode an unsaved exit returns status 2. `svntui` uses this to leave
the SVN conflict unresolved after a cancellation. A successful save returns 0.

## Lazy directory comparison

Directory mode intentionally reads only the current directory level. Child
directories are displayed collapsed (`▸`) and are not traversed, hashed, or
indexed until opened. This keeps startup fast even when the trees below are
large.

- `Enter` opens the selected directory or compares the selected file.
- `Backspace` moves both sides one directory upward.
- `Alt+Right` copies left to right; `Alt+Left` copies right to left. Press the
  same shortcut twice to confirm the filesystem write.
- `Tab` changes the focused side and `Alt+Delete` deletes its selected entry
  after a second confirmation.
- Typing jumps to the first entry whose name starts with the query. `Backspace`
  edits an active query before moving to the parent.
- `Ctrl+S` hides or shows entries with matching metadata; `F5` refreshes the
  current level.

The directory list uses a fast metadata comparison and does not read every file.
`different` means the sizes differ; `metadata` means size is equal but timestamp
or permissions differ; `same metadata` is still a quick metadata result. Opening
a file performs the exact text comparison, including CRLF/LF and final-newline
differences.

## Rofi launcher

The desktop launcher can invoke `merger` without paths; the built-in picker
collects them interactively. A typical launcher command is:

```bash
kitty --title "Merger" --class "merger" bash -lc '~/.local/bin/merger'
```
