package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"merger/internal/core"
)

func (a *App) fileVisibleRows() int {
	return max(3, a.height-7)
}

func (a *App) updateCompare(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if a.saving {
		a.setStatus("Saving; please wait...", false)
		return a, nil
	}
	if a.confirmKey != "" {
		if key == a.confirmKey {
			a.confirmKey = ""
			switch key {
			case "esc":
				a.docs[0].Dirty, a.docs[1].Dirty = false, false
				if a.compareFromPicker {
					a.screen = screenPicker
				} else if a.compareFromDir {
					a.screen = screenDirectory
				} else {
					return a, tea.Quit
				}
				return a, nil
			case "f5":
				a.screen, a.loadingLabel = screenLoading, "Reloading files..."
				return a, loadCompareCmd(a.docs[0].Path, a.docs[1].Path, pathExists(a.docs[0].Path), pathExists(a.docs[1].Path), a.compareFromDir)
			}
		}
		a.confirmKey = ""
	}

	switch key {
	case "esc":
		if a.compareFromDir || a.compareFromPicker {
			if a.hasUnsavedCompare() {
				a.confirmKey = "esc"
				a.setStatus("Unsaved changes. Press Esc again to discard them and return.", true)
				return a, nil
			}
			if a.compareFromPicker {
				a.screen = screenPicker
			} else {
				a.screen = screenDirectory
			}
			return a, nil
		}
		if a.hasUnsavedCompare() {
			a.confirmKey = "esc"
			a.setStatus("Unsaved changes. Press Esc again to discard them and quit.", true)
			return a, nil
		}
		return a, tea.Quit
	case "f1":
		a.returnScreen, a.screen = screenCompare, screenHelp
	case "tab":
		a.fileFocus = 1 - a.fileFocus
		a.normalizeInlineCursor(a.fileFocus)
	case "left", "right", "up", "down", "pgup", "pgdown", "home", "end", "ctrl+home", "ctrl+end":
		a.moveInlineCursor(key)
	case "alt+up":
		a.moveCompareChange(-1)
	case "alt+down":
		a.moveCompareChange(1)
	case "alt+right":
		a.pushCompareChange(1)
	case "alt+left":
		a.pushCompareChange(-1)
	case "alt+delete":
		a.deleteFocusedChange()
	case "enter":
		a.insertInlineText("\n")
	case "backspace":
		a.backspaceInline()
	case "delete":
		a.deleteInline()
	case "ctrl+z":
		a.undoCompare()
	case "ctrl+shift+z", "ctrl+y":
		a.redoCompare()
	case "ctrl+s":
		cmd := a.saveCompareCmd()
		if cmd != nil {
			a.saving = true
		}
		return a, cmd
	case "f5":
		if a.hasUnsavedCompare() {
			a.confirmKey = "f5"
			a.setStatus("Press F5 again to discard edits and reload both files.", true)
			return a, nil
		}
		a.screen, a.loadingLabel = screenLoading, "Reloading files..."
		return a, loadCompareCmd(a.docs[0].Path, a.docs[1].Path, pathExists(a.docs[0].Path), pathExists(a.docs[1].Path), a.compareFromDir)
	}
	if key == " " {
		a.insertInlineText(" ")
	} else if msg.Type == tea.KeyRunes && !msg.Alt {
		a.insertInlineText(string(msg.Runes))
	}
	return a, nil
}

func (a *App) scrollFileRows(delta, total int) {
	a.rowOffset = clamp(a.rowOffset+delta, 0, max(0, total-a.fileVisibleRows()))
}

func (a *App) moveCompareChange(direction int) {
	if len(a.compareChange) == 0 {
		a.setStatus("The files are identical.", false)
		return
	}
	if a.changeCursor < 0 {
		if a.compareAnchorRow >= 0 {
			candidate := -1
			if direction > 0 {
				for index, change := range a.compareChange {
					if change.RowStart >= a.compareAnchorRow {
						candidate = index
						break
					}
				}
			} else {
				for index := len(a.compareChange) - 1; index >= 0; index-- {
					if a.compareChange[index].RowStart < a.compareAnchorRow {
						candidate = index
						break
					}
				}
			}
			if candidate < 0 {
				a.setStatus("There is no change in that direction.", false)
				return
			}
			a.changeCursor = candidate
		} else if direction > 0 {
			a.changeCursor = 0
		} else {
			a.changeCursor = len(a.compareChange) - 1
		}
	} else {
		a.changeCursor = clamp(a.changeCursor+direction, 0, len(a.compareChange)-1)
	}
	a.compareAnchorRow = -1
	a.rowOffset = a.compareChange[a.changeCursor].RowStart
	a.positionInlineCursorsAtChange()
	a.setStatus(fmt.Sprintf("Change %d/%d", a.changeCursor+1, len(a.compareChange)), false)
}

func (a *App) pushCompareChange(direction int) {
	if len(a.compareChange) == 0 || a.changeCursor < 0 {
		a.setStatus("There is no change to push.", false)
		return
	}
	a.pushCompareUndo()
	if a.docs[0].Binary || a.docs[1].Binary {
		anchor := a.rowOffset
		source, target := 0, 1
		if direction < 0 {
			source, target = 1, 0
		}
		path, label := a.docs[target].Path, a.docs[target].Label
		doc := a.docs[source]
		doc.Path, doc.Label, doc.Dirty = path, label, true
		a.docs[target] = doc
		a.rebuildCompareAfterPush(anchor)
		a.setStatus("Binary content staged in memory at the current location; Ctrl+S writes it.", false)
		return
	}
	change := a.compareChange[a.changeCursor]
	anchor := change.RowStart
	if change.Metadata {
		source, target := 0, 1
		arrow := "left → right"
		if direction < 0 {
			source, target = 1, 0
			arrow = "right → left"
		}
		a.docs[target].EOL = a.docs[source].EOL
		a.docs[target].FinalNewline = a.docs[source].FinalNewline
		a.docs[target].Dirty = true
		a.setStatus("Staged line-ending format "+arrow+"; Ctrl+S writes it.", false)
		a.rebuildCompareAfterPush(anchor)
		return
	}
	if direction > 0 {
		replacement := slices.Clone(a.docs[0].Lines[change.LeftStart:change.LeftEnd])
		targetWasEmpty := len(a.docs[1].Lines) == 0
		a.docs[1].Lines = replaceLines(a.docs[1].Lines, change.RightStart, change.RightEnd, replacement)
		a.docs[1].Dirty = true
		if targetWasEmpty && len(replacement) > 0 {
			a.docs[1].EOL = a.docs[0].EOL
			a.docs[1].FinalNewline = a.docs[0].FinalNewline
		}
		if a.docs[1].Mode == 0 {
			a.docs[1].Mode = a.docs[0].Mode
		}
		a.setStatus("Staged current change left → right; Ctrl+S writes it.", false)
	} else {
		replacement := slices.Clone(a.docs[1].Lines[change.RightStart:change.RightEnd])
		targetWasEmpty := len(a.docs[0].Lines) == 0
		a.docs[0].Lines = replaceLines(a.docs[0].Lines, change.LeftStart, change.LeftEnd, replacement)
		a.docs[0].Dirty = true
		if targetWasEmpty && len(replacement) > 0 {
			a.docs[0].EOL = a.docs[1].EOL
			a.docs[0].FinalNewline = a.docs[1].FinalNewline
		}
		if a.docs[0].Mode == 0 {
			a.docs[0].Mode = a.docs[1].Mode
		}
		a.setStatus("Staged current change right → left; Ctrl+S writes it.", false)
	}
	a.rebuildCompareAfterPush(anchor)
}

func (a *App) rebuildCompareAfterPush(anchor int) {
	a.refreshCompareDirty()
	a.rebuildCompare()
	for side := range 2 {
		a.normalizeInlineCursor(side)
	}
	a.compareAnchorRow = clamp(anchor, 0, max(0, len(a.compareRows)-1))
	a.changeCursor = -1
	a.rowOffset = clamp(anchor, 0, max(0, len(a.compareRows)-a.fileVisibleRows()))
}

func (a *App) deleteFocusedChange() {
	if len(a.compareChange) == 0 || a.changeCursor < 0 || a.docs[a.fileFocus].Binary {
		a.setStatus("There is no text change to delete.", false)
		return
	}
	change := a.compareChange[a.changeCursor]
	if change.Metadata {
		a.setStatus("Use Alt+Left or Alt+Right to copy the line-ending format.", false)
		return
	}
	a.pushCompareUndo()
	if a.fileFocus == 0 {
		a.docs[0].Lines = replaceLines(a.docs[0].Lines, change.LeftStart, change.LeftEnd, nil)
		a.docs[0].Dirty = true
	} else {
		a.docs[1].Lines = replaceLines(a.docs[1].Lines, change.RightStart, change.RightEnd, nil)
		a.docs[1].Dirty = true
	}
	a.setStatus("Deleted the current change from the focused side in memory.", false)
	a.refreshCompareDirty()
	a.rebuildCompare()
	for side := range 2 {
		a.normalizeInlineCursor(side)
	}
}

func (a *App) saveCompareCmd() tea.Cmd {
	docs := a.docs
	return func() tea.Msg {
		for _, doc := range docs {
			if !doc.Dirty {
				continue
			}
			if err := core.AtomicWrite(doc.Path, []byte(doc.Text()), doc.Mode); err != nil {
				return compareSavedMsg{err: err}
			}
		}
		return compareSavedMsg{}
	}
}

func (a *App) updateMerge(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if a.saving {
		a.setStatus("Saving merge result; please wait...", false)
		return a, nil
	}
	if a.confirmKey != "" {
		if key == a.confirmKey {
			a.confirmKey = ""
			switch key {
			case "q", "esc":
				a.cancelled = !a.saved
				return a, tea.Quit
			case "r":
				a.screen, a.loadingLabel = screenLoading, "Reloading merge inputs..."
				return a, loadMergeCmd(a.config)
			}
		}
		a.confirmKey = ""
	}

	switch key {
	case "q", "esc":
		if a.dirtyMerge() {
			a.confirmKey = key
			a.setStatus("Merge output has not been saved. Press the same key again to cancel and quit.", true)
			return a, nil
		}
		return a, tea.Quit
	case "?":
		a.returnScreen, a.screen = screenMerge, screenHelp
	case "up", "k":
		a.scrollFileRows(-1, len(a.mergeRows))
	case "down", "j":
		a.scrollFileRows(1, len(a.mergeRows))
	case "pgup":
		a.scrollFileRows(-a.fileVisibleRows(), len(a.mergeRows))
	case "pgdown":
		a.scrollFileRows(a.fileVisibleRows(), len(a.mergeRows))
	case "home", "g":
		a.rowOffset = 0
	case "end", "G":
		a.rowOffset = max(0, len(a.mergeRows)-a.fileVisibleRows())
	case "alt+up":
		a.moveMergeChange(-1)
	case "alt+down":
		a.moveMergeChange(1)
	case "alt+right", "m":
		a.resolveMerge(core.ResolutionMine)
	case "alt+left", "t":
		a.resolveMerge(core.ResolutionTheirs)
	case "b":
		a.resolveMerge(core.ResolutionBase)
	case "a":
		a.resolveMerge(core.ResolutionBoth)
	case "u":
		a.resolveMerge(core.ResolutionUnresolved)
	case "e", "enter":
		if a.mergeBinary {
			a.setStatus("Binary chunks cannot be manually text-edited.", true)
			break
		}
		a.openMergeEditor()
	case "ctrl+s":
		cmd := a.saveMergeCmd()
		if cmd != nil {
			a.saving = true
		}
		return a, cmd
	case "r":
		a.confirmKey = "r"
		a.setStatus("Press r again to discard decisions and reload all merge inputs.", true)
	}
	return a, nil
}

func (a *App) moveMergeChange(direction int) {
	count := len(a.mergeChanges)
	if a.mergeBinary {
		count = 1
	}
	if count == 0 {
		a.setStatus("All three files are identical.", false)
		return
	}
	a.changeCursor = clamp(a.changeCursor+direction, 0, count-1)
	if !a.mergeBinary {
		a.rowOffset = a.mergeChanges[a.changeCursor].RowStart
	}
	a.setStatus(fmt.Sprintf("Change %d/%d", a.changeCursor+1, count), false)
}

func (a *App) resolveMerge(resolution core.Resolution) {
	if a.mergeBinary {
		switch resolution {
		case core.ResolutionMine:
			a.mergeBinaryResult = slices.Clone(a.mine.Data)
		case core.ResolutionTheirs:
			a.mergeBinaryResult = slices.Clone(a.theirs.Data)
		case core.ResolutionBase:
			a.mergeBinaryResult = slices.Clone(a.base.Data)
		default:
			a.setStatus("Binary merge supports mine, theirs, or base.", true)
			return
		}
		a.mergeBinaryChoice = resolution
		a.saved = false
		a.setStatus("Selected "+resolution.String()+" binary content; Ctrl+S writes it.", false)
		return
	}
	if a.changeCursor < 0 || a.changeCursor >= len(a.mergePlan.Chunks) {
		a.setStatus("There is no change to resolve.", false)
		return
	}
	if a.mergePlan.Resolve(a.changeCursor, resolution, nil) {
		a.saved = false
		a.rebuildMergeRows()
		a.setStatus("Current change resolved as "+resolution.String()+".", false)
	}
}

func (a *App) saveMergeCmd() tea.Cmd {
	if a.mergeBinary {
		if a.mergeBinaryChoice == core.ResolutionUnresolved {
			a.setStatus("Resolve the binary conflict before saving.", true)
			return nil
		}
		data := slices.Clone(a.mergeBinaryResult)
		path := a.config.Output
		mode := modeForOutput(path, a.mine.Mode)
		return func() tea.Msg { return mergeSavedMsg{err: core.AtomicWrite(path, data, mode)} }
	}
	if unresolved := a.mergePlan.UnresolvedCount(); unresolved > 0 {
		a.setStatus(fmt.Sprintf("Resolve all %d conflict(s) before saving.", unresolved), true)
		return nil
	}
	lines := a.mergePlan.ResultLines()
	eol := a.mine.EOL
	if eol == "" {
		eol = "\n"
	}
	text := strings.Join(lines, eol)
	if a.mine.FinalNewline {
		text += eol
	}
	path := a.config.Output
	mode := modeForOutput(path, a.mine.Mode)
	return func() tea.Msg { return mergeSavedMsg{err: core.AtomicWrite(path, []byte(text), mode)} }
}

func (a *App) openMergeEditor() {
	if a.changeCursor < 0 || a.changeCursor >= len(a.mergePlan.Chunks) {
		a.setStatus("There is no merge chunk to edit.", false)
		return
	}
	a.editorTarget = editMergeChunk
	a.editorChunk = a.changeCursor
	a.editor.SetValue(editorText(a.mergePlan.Chunks[a.editorChunk].Result))
	a.editor.Focus()
	a.resizeEditor()
	a.returnScreen, a.screen = screenMerge, screenEditor
}

func (a *App) updateEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.editor.Blur()
		a.screen = a.returnScreen
		a.setStatus("Edit cancelled.", false)
		return a, nil
	case "ctrl+s", "ctrl+enter":
		value := a.editor.Value()
		a.editor.Blur()
		switch a.editorTarget {
		case editMergeChunk:
			a.mergePlan.Resolve(a.editorChunk, core.ResolutionManual, core.SplitEditorLines(value))
			a.changeCursor = a.editorChunk
			a.saved = false
			a.rebuildMergeRows()
		}
		a.screen = a.returnScreen
		a.setStatus("Manual edit applied in memory.", false)
		return a, nil
	}
	var cmd tea.Cmd
	a.editor, cmd = a.editor.Update(msg)
	return a, cmd
}
