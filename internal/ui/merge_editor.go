package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"merger/internal/core"
)

// The merged result pane is edited exactly like a pane of the two-file
// comparison: a free cursor moved with the arrow keys or placed with the mouse,
// and typing that writes straight into the owning chunk or base context.

type mergeSnapshot struct {
	Plan             core.MergePlan
	Cursor           inlineCursor
	HorizontalOffset int
	RowOffset        int
	ChangeCursor     int
}

func (a *App) resetMergeEditing() {
	a.mergeCursor = inlineCursor{}
	a.mergeHorizontalOffset = 0
	a.mergeUndo = nil
	a.mergeRedo = nil
	if a.changeCursor >= 0 && a.changeCursor < len(a.mergePlan.Chunks) {
		a.mergeCursor.Line = clamp(a.mergePlan.ChunkResultStart(a.changeCursor), 0, a.maxMergeCursorLine())
	}
}

func (a *App) maxMergeCursorLine() int { return max(0, len(a.mergeResult)-1) }

func (a *App) mergeLine(line int) string {
	if line < 0 || line >= len(a.mergeResult) {
		return ""
	}
	return a.mergeResult[line]
}

func (a *App) mergeLineLength(line int) int { return len([]rune(a.mergeLine(line))) }

func (a *App) normalizeMergeCursor() {
	a.mergeCursor.Line = clamp(a.mergeCursor.Line, 0, a.maxMergeCursorLine())
	a.mergeCursor.Col = clamp(a.mergeCursor.Col, 0, a.mergeLineLength(a.mergeCursor.Line))
}

func (a *App) captureMergeSnapshot() mergeSnapshot {
	return mergeSnapshot{
		Plan:             a.mergePlan.Clone(),
		Cursor:           a.mergeCursor,
		HorizontalOffset: a.mergeHorizontalOffset,
		RowOffset:        a.rowOffset,
		ChangeCursor:     a.changeCursor,
	}
}

func (a *App) pushMergeUndo() {
	a.mergeUndo = append(a.mergeUndo, a.captureMergeSnapshot())
	if len(a.mergeUndo) > compareHistoryLimit {
		a.mergeUndo = a.mergeUndo[len(a.mergeUndo)-compareHistoryLimit:]
	}
	a.mergeRedo = nil
}

func (a *App) undoMerge() {
	if len(a.mergeUndo) == 0 {
		a.setStatus("Nothing to undo.", false)
		return
	}
	a.mergeRedo = append(a.mergeRedo, a.captureMergeSnapshot())
	snapshot := a.mergeUndo[len(a.mergeUndo)-1]
	a.mergeUndo = a.mergeUndo[:len(a.mergeUndo)-1]
	a.restoreMergeSnapshot(snapshot)
	a.setStatus("Undid last merge edit.", false)
}

func (a *App) redoMerge() {
	if len(a.mergeRedo) == 0 {
		a.setStatus("Nothing to redo.", false)
		return
	}
	a.mergeUndo = append(a.mergeUndo, a.captureMergeSnapshot())
	snapshot := a.mergeRedo[len(a.mergeRedo)-1]
	a.mergeRedo = a.mergeRedo[:len(a.mergeRedo)-1]
	a.restoreMergeSnapshot(snapshot)
	a.setStatus("Redid last merge edit.", false)
}

func (a *App) restoreMergeSnapshot(snapshot mergeSnapshot) {
	a.mergePlan = snapshot.Plan.Clone()
	a.saved = false
	a.refreshMergeRows()
	a.mergeCursor = snapshot.Cursor
	a.mergeHorizontalOffset = snapshot.HorizontalOffset
	a.normalizeMergeCursor()
	a.changeCursor = snapshot.ChangeCursor
	if a.changeCursor >= len(a.mergeChanges) {
		a.changeCursor = -1
	}
	a.rowOffset = clamp(snapshot.RowOffset, 0, max(0, len(a.mergeRows)-a.fileVisibleRows()))
}

func (a *App) insertMergeText(text string) {
	if a.mergeBinary || text == "" {
		return
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	a.pushMergeUndo()
	if len(a.mergeResult) == 0 {
		a.mergePlan.InsertResultLineAfter(-1, "")
		a.refreshMergeRows()
	}
	a.normalizeMergeCursor()
	line, col := a.mergeCursor.Line, a.mergeCursor.Col
	runes := []rune(a.mergeLine(line))
	prefix, suffix := string(runes[:col]), string(runes[col:])
	parts := strings.Split(text, "\n")

	a.mergePlan.SetResultLine(line, prefix+parts[0])
	for index := 1; index < len(parts); index++ {
		a.mergePlan.InsertResultLineAfter(line+index-1, parts[index])
	}
	last := line + len(parts) - 1
	tail := parts[len(parts)-1]
	if len(parts) == 1 {
		tail = prefix + parts[0]
	}
	a.mergePlan.SetResultLine(last, tail+suffix)
	a.mergeCursor = inlineCursor{Line: last, Col: len([]rune(tail))}
	a.finishMergeEdit()
}

func (a *App) backspaceMerge() {
	if a.mergeBinary {
		return
	}
	a.normalizeMergeCursor()
	line, col := a.mergeCursor.Line, a.mergeCursor.Col
	switch {
	case col > 0:
		a.pushMergeUndo()
		runes := []rune(a.mergeLine(line))
		a.mergePlan.SetResultLine(line, string(slices.Delete(runes, col-1, col)))
		a.mergeCursor.Col = col - 1
	case line > 0:
		a.pushMergeUndo()
		previous := a.mergeLine(line - 1)
		a.mergePlan.SetResultLine(line-1, previous+a.mergeLine(line))
		a.mergePlan.DeleteResultLine(line)
		a.mergeCursor = inlineCursor{Line: line - 1, Col: len([]rune(previous))}
	default:
		return
	}
	a.finishMergeEdit()
}

func (a *App) deleteMerge() {
	if a.mergeBinary {
		return
	}
	a.normalizeMergeCursor()
	line, col := a.mergeCursor.Line, a.mergeCursor.Col
	runes := []rune(a.mergeLine(line))
	switch {
	case col < len(runes):
		a.pushMergeUndo()
		a.mergePlan.SetResultLine(line, string(slices.Delete(runes, col, col+1)))
	case line < len(a.mergeResult)-1:
		a.pushMergeUndo()
		a.mergePlan.SetResultLine(line, a.mergeLine(line)+a.mergeLine(line+1))
		a.mergePlan.DeleteResultLine(line + 1)
	default:
		return
	}
	a.finishMergeEdit()
}

func (a *App) finishMergeEdit() {
	a.saved = false
	previousOffset := a.rowOffset
	a.refreshMergeRows()
	a.normalizeMergeCursor()
	a.changeCursor = a.mergePlan.ChunkForResultLine(a.mergeCursor.Line)
	a.rowOffset = clamp(previousOffset, 0, max(0, len(a.mergeRows)-a.fileVisibleRows()))
	a.scrollMergeCursorIntoView()
	a.setStatus("Editing the merged result in memory; Ctrl+S saves.", false)
}

func (a *App) moveMergeCursor(key string) {
	a.normalizeMergeCursor()
	cursor := &a.mergeCursor
	switch key {
	case "left":
		if cursor.Col > 0 {
			cursor.Col--
		} else if cursor.Line > 0 {
			cursor.Line--
			cursor.Col = a.mergeLineLength(cursor.Line)
		}
	case "right":
		if cursor.Col < a.mergeLineLength(cursor.Line) {
			cursor.Col++
		} else if cursor.Line < a.maxMergeCursorLine() {
			cursor.Line++
			cursor.Col = 0
		}
	case "up":
		cursor.Line = max(0, cursor.Line-1)
	case "down":
		cursor.Line = min(a.maxMergeCursorLine(), cursor.Line+1)
	case "home":
		cursor.Col = 0
	case "end":
		cursor.Col = a.mergeLineLength(cursor.Line)
	case "ctrl+home":
		cursor.Line, cursor.Col = 0, 0
	case "ctrl+end":
		cursor.Line = a.maxMergeCursorLine()
		cursor.Col = a.mergeLineLength(cursor.Line)
	case "pgup":
		cursor.Line = max(0, cursor.Line-a.fileVisibleRows())
	case "pgdown":
		cursor.Line = min(a.maxMergeCursorLine(), cursor.Line+a.fileVisibleRows())
	}
	cursor.Col = min(cursor.Col, a.mergeLineLength(cursor.Line))
	a.changeCursor = a.mergePlan.ChunkForResultLine(cursor.Line)
	a.scrollMergeCursorIntoView()
}

func (a *App) scrollMergeCursorIntoView() {
	row := a.rowForMergeCursor()
	visible := a.fileVisibleRows()
	if row < a.rowOffset {
		a.rowOffset = row
	} else if row >= a.rowOffset+visible {
		a.rowOffset = row - visible + 1
	}
	a.rowOffset = clamp(a.rowOffset, 0, max(0, len(a.mergeRows)-visible))
}

func (a *App) rowForMergeCursor() int {
	target := a.mergeCursor.Line + 1
	for index, row := range a.mergeRows {
		if row.ResultNo == target {
			return index
		}
	}
	if a.changeCursor >= 0 && a.changeCursor < len(a.mergeChanges) {
		return a.mergeChanges[a.changeCursor].RowStart
	}
	return max(0, len(a.mergeRows)-1)
}

func (a *App) placeMergeCursorAtChange() {
	if a.changeCursor < 0 || a.changeCursor >= len(a.mergePlan.Chunks) {
		return
	}
	a.mergeCursor = inlineCursor{Line: clamp(a.mergePlan.ChunkResultStart(a.changeCursor), 0, a.maxMergeCursorLine())}
	a.mergeHorizontalOffset = 0
}

func (a *App) cursorForMergeRow(rowIndex, resultNo int) cellCursor {
	cursor := cellCursor{Offset: a.mergeHorizontalOffset}
	if resultNo > 0 {
		cursor.Show = a.mergeCursor.Line == resultNo-1
	} else if len(a.mergeResult) == 0 {
		cursor.Show = rowIndex == a.rowForMergeCursor()
	}
	cursor.RuneColumn = a.mergeCursor.Col
	return cursor
}

func (a *App) ensureMergeCursorVisible(width int) {
	if width <= 0 {
		return
	}
	a.normalizeMergeCursor()
	visual := cursorVisualColumn(a.mergeLine(a.mergeCursor.Line), a.mergeCursor.Col)
	if visual < a.mergeHorizontalOffset {
		a.mergeHorizontalOffset = visual
	} else if visual >= a.mergeHorizontalOffset+width {
		a.mergeHorizontalOffset = visual - width + 1
	}
	a.mergeHorizontalOffset = max(0, a.mergeHorizontalOffset)
}

// mergeLayout mirrors the column arithmetic of viewMerge so a click can be
// resolved back to a pane and a character.
type mergeLayout struct {
	numWidth                           int
	mineWidth, resultWidth, theirWidth int
	mineStart, resultStart, theirStart int
}

func (a *App) mergeLayout() mergeLayout {
	contentWidth := max(30, a.width-6)
	numWidth := len(fmt.Sprintf("%d", max(len(a.mine.Lines), len(a.mergeResult), len(a.theirs.Lines), 1)))
	separatorWidth := 6
	cellSpace := max(6, contentWidth-separatorWidth-3*(numWidth+1))
	layout := mergeLayout{numWidth: numWidth, mineWidth: cellSpace / 3, resultWidth: cellSpace / 3}
	layout.theirWidth = cellSpace - layout.mineWidth - layout.resultWidth
	layout.mineStart = 1 + numWidth + 1
	layout.resultStart = layout.mineStart + layout.mineWidth + 3 + numWidth + 1
	layout.theirStart = layout.resultStart + layout.resultWidth + 3 + numWidth + 1
	return layout
}

func (a *App) clickMerge(event tea.MouseEvent) {
	if a.mergeBinary {
		return
	}
	const firstDataRow = 4
	visible := a.fileVisibleRows()
	a.rowOffset = clamp(a.rowOffset, 0, max(0, len(a.mergeRows)-visible))
	visibleRow := event.Y - firstDataRow
	if visibleRow < 0 || visibleRow >= visible {
		return
	}
	rowIndex := a.rowOffset + visibleRow
	if rowIndex < 0 || rowIndex >= len(a.mergeRows) {
		return
	}
	row := a.mergeRows[rowIndex]
	layout := a.mergeLayout()
	if event.X < layout.resultStart-layout.numWidth-1 || event.X >= layout.theirStart-layout.numWidth-1 {
		if row.Chunk < 0 {
			a.setStatus("MINE and THEIRS are read-only; edit the merged result in the middle pane.", false)
			return
		}
		a.changeCursor = row.Chunk
		a.placeMergeCursorAtChange()
		a.setStatus(fmt.Sprintf("Selected change %d/%d; Alt+←/→ copies a side into the result.", row.Chunk+1, len(a.mergeChanges)), false)
		return
	}

	line := a.mergeCursor.Line
	switch {
	case row.ResultNo > 0:
		line = row.ResultNo - 1
	case row.Chunk >= 0:
		line = a.mergePlan.ChunkResultStart(row.Chunk)
	}
	line = clamp(line, 0, a.maxMergeCursorLine())
	displayColumn := clamp(event.X-layout.resultStart, 0, max(0, layout.resultWidth-1))
	a.mergeCursor = inlineCursor{
		Line: line,
		Col:  runeColumnAtVisual(a.mergeLine(line), a.mergeHorizontalOffset+displayColumn),
	}
	a.changeCursor = a.mergePlan.ChunkForResultLine(line)
	a.setStatus(fmt.Sprintf("Editing the merged result at line %d, column %d.", line+1, a.mergeCursor.Col+1), false)
}
