# merger

`merger` is a keyboard-first diff, merge, and directory comparison TUI written
in Go. It follows the active HyDE/Wallbash palette, with Catppuccin Mocha as a
fallback. It is a standalone companion to `svntui`, but it also works directly
from a shell.

Diff colors default to `TUI_DIFF_THEME=auto`: when Wallbash provides a
monochrome or insufficiently distinct semantic palette, only added, deleted,
modified, and conflicting content uses the Catppuccin fallback colors. The
rest of the interface remains on the active theme. The change map also uses
`++`, `--`, `~~`, and `!!`, so it is readable without color. Available
overrides are `wallbash`, `semantic`, and `mono`.

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

Open a labelled comparison that cannot modify either input:

```bash
merger --read-only --label-left BASE --label-right WORKING base.txt working.txt
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
- Added, deleted, modified, and conflicting regions use distinct semantic
  shades from the active palette.
- `Alt+Up` and `Alt+Down` select the previous or next change.
- `Ctrl+F` searches both panes; in read-only mode `/` starts the same search.
- `Alt+W` ignores whitespace-only differences and `Alt+E` ignores line-ending
  differences. Both switches can be changed while the comparison is open.
- `Alt+Right` pushes the selected left change to the right; `Alt+Left` pushes
  right to left, matching Meld's arrow direction.
- A pushed change stays at its original screen location instead of selecting the
  next diff. Pending target lines remain highlighted with a `◆` marker until a
  successful save.
- The right-hand overview draws the whole file as double-width change blocks and
  keeps the scrollbar in its own column beside them. Its bright thumb marks the
  part of the file currently on screen, so the position stays readable even
  where the change map is solid with differences. The same overview and
  scrollbar appear in the three-way merge.
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

The middle `MERGED RESULT` pane is directly editable, exactly like a pane of the
two-file comparison: click a character or move the cursor with the arrow keys,
then type. There is no separate chunk-edit screen and no `e` step — `Enter`,
`Backspace`, `Delete` and `Tab` change the result in place, both inside a
conflict and in the unchanged context lines around it. Because every printable
key types into the result, the change actions are all on `Alt`:

- `Alt+Up` / `Alt+Down`: previous / next change
- `Alt+Right`: MINE into the result
- `Alt+Left`: THEIRS into the result
- `Alt+B`: BASE into the result
- `Alt+A`: MINE followed by THEIRS
- `Alt+U`: restore the conflict markers of the selected change
- `Ctrl+Z` undoes and `Ctrl+Shift+Z` or `Ctrl+Y` redoes, covering typed edits and
  side choices alike
- `F1` opens help; `F5` reloads all three inputs after a confirmation
- `Ctrl+Shift+P` (or `Ctrl+P` where the terminal cannot distinguish Shift)
  opens a fuzzy, screen-aware command palette
- modified lines emphasize the exact changed character span, including in
  monochrome themes; `Alt+S` toggles lightweight source syntax highlighting
  (or start with `--syntax`)
- `Ctrl+S`: save, only after every conflict is resolved
- `Esc`: cancel, with a confirmation while the result is unsaved

Typing inside a conflict resolves it as a manual edit, but a chunk whose text
still contains `<<<<<<<`, `|||||||`, `=======` or `>>>>>>>` stays unresolved.
Conflict markers therefore cannot reach the saved result by accident.

Clicking `MINE` or `THEIRS` selects the change under the pointer without editing
those panes; only the merged result is writable.

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
