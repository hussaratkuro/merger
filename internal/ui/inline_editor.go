package ui

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"merger/internal/core"
)

const compareHistoryLimit = 500

type cellCursor struct {
	Show       bool
	RuneColumn int
	Offset     int
}

func (a *App) resetInlineEditing() {
	a.inlineCursors = [2]inlineCursor{}
	a.horizontalOffset = [2]int{}
	a.compareUndo = nil
	a.compareRedo = nil
	if a.changeCursor < 0 || a.changeCursor >= len(a.compareChange) {
		return
	}
	change := a.compareChange[a.changeCursor]
	if change.Metadata {
		for side := range 2 {
			a.inlineCursors[side].Line = max(0, len(a.docs[side].Lines)-1)
		}
		return
	}
	a.inlineCursors[0].Line = clamp(change.LeftStart, 0, maxCursorLine(a.docs[0]))
	a.inlineCursors[1].Line = clamp(change.RightStart, 0, maxCursorLine(a.docs[1]))
}

func (a *App) captureCompareSnapshot() compareSnapshot {
	return compareSnapshot{
		Docs:             [2]core.Document{cloneDocument(a.docs[0]), cloneDocument(a.docs[1])},
		Cursors:          a.inlineCursors,
		HorizontalOffset: a.horizontalOffset,
		FileFocus:        a.fileFocus,
		RowOffset:        a.rowOffset,
		ChangeCursor:     a.changeCursor,
		AnchorRow:        a.compareAnchorRow,
	}
}

func appendCompareHistory(history []compareSnapshot, snapshot compareSnapshot) []compareSnapshot {
	history = append(history, snapshot)
	if len(history) > compareHistoryLimit {
		history = history[len(history)-compareHistoryLimit:]
	}
	return history
}

func (a *App) pushCompareUndo() {
	a.compareUndo = appendCompareHistory(a.compareUndo, a.captureCompareSnapshot())
	a.compareRedo = nil
}

func (a *App) undoCompare() {
	if len(a.compareUndo) == 0 {
		a.setStatus("Nothing to undo.", false)
		return
	}
	a.compareRedo = appendCompareHistory(a.compareRedo, a.captureCompareSnapshot())
	snapshot := a.compareUndo[len(a.compareUndo)-1]
	a.compareUndo = a.compareUndo[:len(a.compareUndo)-1]
	a.restoreCompareSnapshot(snapshot)
	a.setStatus("Undid last edit.", false)
}

func (a *App) redoCompare() {
	if len(a.compareRedo) == 0 {
		a.setStatus("Nothing to redo.", false)
		return
	}
	a.compareUndo = appendCompareHistory(a.compareUndo, a.captureCompareSnapshot())
	snapshot := a.compareRedo[len(a.compareRedo)-1]
	a.compareRedo = a.compareRedo[:len(a.compareRedo)-1]
	a.restoreCompareSnapshot(snapshot)
	a.setStatus("Redid last edit.", false)
}

func (a *App) restoreCompareSnapshot(snapshot compareSnapshot) {
	a.docs = [2]core.Document{cloneDocument(snapshot.Docs[0]), cloneDocument(snapshot.Docs[1])}
	for side := range 2 {
		a.docs[side].Dirty = !documentContentsEqual(a.docs[side], a.originalDocs[side])
	}
	a.inlineCursors = snapshot.Cursors
	a.horizontalOffset = snapshot.HorizontalOffset
	a.fileFocus = snapshot.FileFocus
	a.rebuildCompare()
	for side := range 2 {
		a.normalizeInlineCursor(side)
	}
	a.changeCursor = snapshot.ChangeCursor
	if a.changeCursor >= len(a.compareChange) {
		a.changeCursor = -1
	}
	a.compareAnchorRow = snapshot.AnchorRow
	if a.compareAnchorRow >= len(a.compareRows) {
		a.compareAnchorRow = max(0, len(a.compareRows)-1)
	}
	a.rowOffset = clamp(snapshot.RowOffset, 0, max(0, len(a.compareRows)-a.fileVisibleRows()))
}

func documentContentsEqual(left, right core.Document) bool {
	if left.Binary || right.Binary {
		return left.Binary == right.Binary && bytes.Equal(left.Data, right.Data)
	}
	return slices.Equal(left.Lines, right.Lines) && documentFormatSignature(left) == documentFormatSignature(right)
}

func (a *App) refreshCompareDirty() {
	for side := range 2 {
		a.docs[side].Dirty = !documentContentsEqual(a.docs[side], a.originalDocs[side])
	}
}

func (a *App) insertInlineText(text string) {
	if a.docs[a.fileFocus].Binary || text == "" {
		return
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	a.pushCompareUndo()
	side := a.fileFocus
	document := &a.docs[side]
	oldText := document.LFText()
	offset := cursorTextOffset(*document, a.inlineCursors[side])
	runes := []rune(oldText)
	inserted := []rune(text)
	updated := string(append(append(slices.Clone(runes[:offset]), inserted...), runes[offset:]...))
	document.SetEditorText(updated)
	document.Dirty = !documentContentsEqual(*document, a.originalDocs[side])
	a.inlineCursors[side] = cursorFromTextOffset(updated, offset+len(inserted))
	a.finishInlineEdit(a.rowOffset)
}

func (a *App) backspaceInline() {
	if a.docs[a.fileFocus].Binary {
		return
	}
	side := a.fileFocus
	document := &a.docs[side]
	text := document.LFText()
	offset := cursorTextOffset(*document, a.inlineCursors[side])
	if offset == 0 {
		return
	}
	a.pushCompareUndo()
	runes := []rune(text)
	updated := string(append(slices.Clone(runes[:offset-1]), runes[offset:]...))
	document.SetEditorText(updated)
	document.Dirty = !documentContentsEqual(*document, a.originalDocs[side])
	a.inlineCursors[side] = cursorFromTextOffset(updated, offset-1)
	a.finishInlineEdit(a.rowOffset)
}

func (a *App) deleteInline() {
	if a.docs[a.fileFocus].Binary {
		return
	}
	side := a.fileFocus
	document := &a.docs[side]
	text := document.LFText()
	offset := cursorTextOffset(*document, a.inlineCursors[side])
	runes := []rune(text)
	if offset >= len(runes) {
		return
	}
	a.pushCompareUndo()
	updated := string(append(slices.Clone(runes[:offset]), runes[offset+1:]...))
	document.SetEditorText(updated)
	document.Dirty = !documentContentsEqual(*document, a.originalDocs[side])
	a.inlineCursors[side] = cursorFromTextOffset(updated, offset)
	a.finishInlineEdit(a.rowOffset)
}

func (a *App) finishInlineEdit(previousOffset int) {
	a.rebuildCompare()
	a.changeCursor = -1
	for side := range 2 {
		a.normalizeInlineCursor(side)
	}
	row := a.rowForInlineCursor(a.fileFocus)
	a.compareAnchorRow = row
	a.rowOffset = clamp(previousOffset, 0, max(0, len(a.compareRows)-a.fileVisibleRows()))
	if row < a.rowOffset {
		a.rowOffset = row
	} else if row >= a.rowOffset+a.fileVisibleRows() {
		a.rowOffset = row - a.fileVisibleRows() + 1
	}
	a.setStatus("Editing in memory; Ctrl+S saves.", false)
}

func (a *App) moveInlineCursor(key string) {
	side := a.fileFocus
	cursor := &a.inlineCursors[side]
	document := a.docs[side]
	a.normalizeInlineCursor(side)
	switch key {
	case "left":
		if cursor.Col > 0 {
			cursor.Col--
		} else if cursor.Line > 0 {
			cursor.Line--
			cursor.Col = inlineLineLength(document, cursor.Line)
		}
	case "right":
		length := inlineLineLength(document, cursor.Line)
		if cursor.Col < length {
			cursor.Col++
		} else if cursor.Line < maxCursorLine(document) {
			cursor.Line++
			cursor.Col = 0
		}
	case "up":
		cursor.Line = max(0, cursor.Line-1)
		cursor.Col = min(cursor.Col, inlineLineLength(document, cursor.Line))
	case "down":
		cursor.Line = min(maxCursorLine(document), cursor.Line+1)
		cursor.Col = min(cursor.Col, inlineLineLength(document, cursor.Line))
	case "home":
		cursor.Col = 0
	case "end":
		cursor.Col = inlineLineLength(document, cursor.Line)
	case "ctrl+home":
		cursor.Line, cursor.Col = 0, 0
	case "ctrl+end":
		cursor.Line = maxCursorLine(document)
		cursor.Col = inlineLineLength(document, cursor.Line)
	case "pgup":
		cursor.Line = max(0, cursor.Line-a.fileVisibleRows())
		cursor.Col = min(cursor.Col, inlineLineLength(document, cursor.Line))
	case "pgdown":
		cursor.Line = min(maxCursorLine(document), cursor.Line+a.fileVisibleRows())
		cursor.Col = min(cursor.Col, inlineLineLength(document, cursor.Line))
	}
	row := a.rowForInlineCursor(side)
	if row < a.rowOffset {
		a.rowOffset = row
	} else if row >= a.rowOffset+a.fileVisibleRows() {
		a.rowOffset = row - a.fileVisibleRows() + 1
	}
}

func (a *App) positionInlineCursorsAtChange() {
	if a.changeCursor < 0 || a.changeCursor >= len(a.compareChange) {
		return
	}
	change := a.compareChange[a.changeCursor]
	if change.Metadata {
		return
	}
	a.inlineCursors[0] = inlineCursor{Line: clamp(change.LeftStart, 0, maxCursorLine(a.docs[0]))}
	a.inlineCursors[1] = inlineCursor{Line: clamp(change.RightStart, 0, maxCursorLine(a.docs[1]))}
}

func (a *App) cursorForCompareRow(side, rowIndex, lineNumber int, metadata bool) cellCursor {
	if metadata || side != a.fileFocus {
		return cellCursor{Offset: a.horizontalOffset[side]}
	}
	cursor := a.inlineCursors[side]
	show := lineNumber > 0 && cursor.Line == lineNumber-1
	if lineNumber == 0 && len(a.docs[side].Lines) == 0 && cursor.Line == 0 {
		show = rowIndex == a.rowForInlineCursor(side)
	}
	return cellCursor{Show: show, RuneColumn: cursor.Col, Offset: a.horizontalOffset[side]}
}

func (a *App) ensureInlineCursorVisible(side, width int) {
	if width <= 0 {
		return
	}
	a.normalizeInlineCursor(side)
	cursor := a.inlineCursors[side]
	line := ""
	if cursor.Line < len(a.docs[side].Lines) {
		line = a.docs[side].Lines[cursor.Line]
	}
	visual := cursorVisualColumn(line, cursor.Col)
	if visual < a.horizontalOffset[side] {
		a.horizontalOffset[side] = visual
	} else if visual >= a.horizontalOffset[side]+width {
		a.horizontalOffset[side] = visual - width + 1
	}
	a.horizontalOffset[side] = max(0, a.horizontalOffset[side])
}

func maxCursorLine(document core.Document) int {
	if len(document.Lines) == 0 {
		return 0
	}
	if document.FinalNewline {
		return len(document.Lines)
	}
	return len(document.Lines) - 1
}

func inlineLineLength(document core.Document, line int) int {
	if line < 0 || line >= len(document.Lines) {
		return 0
	}
	return len([]rune(document.Lines[line]))
}

func (a *App) normalizeInlineCursor(side int) {
	cursor := &a.inlineCursors[side]
	cursor.Line = clamp(cursor.Line, 0, maxCursorLine(a.docs[side]))
	cursor.Col = clamp(cursor.Col, 0, inlineLineLength(a.docs[side], cursor.Line))
}

func cursorTextOffset(document core.Document, cursor inlineCursor) int {
	textLength := len([]rune(document.LFText()))
	if cursor.Line >= len(document.Lines) {
		return textLength
	}
	offset := 0
	for line := 0; line < cursor.Line; line++ {
		offset += len([]rune(document.Lines[line])) + 1
	}
	return min(textLength, offset+clamp(cursor.Col, 0, inlineLineLength(document, cursor.Line)))
}

func cursorFromTextOffset(text string, offset int) inlineCursor {
	cursor := inlineCursor{}
	for index, r := range []rune(text) {
		if index >= offset {
			break
		}
		if r == '\n' {
			cursor.Line++
			cursor.Col = 0
		} else {
			cursor.Col++
		}
	}
	return cursor
}

func (a *App) rowForInlineCursor(side int) int {
	target := a.inlineCursors[side].Line + 1
	for index, row := range a.compareRows {
		lineNumber := row.LeftNo
		if side == 1 {
			lineNumber = row.RightNo
		}
		if lineNumber == target {
			return index
		}
	}
	if len(a.docs[side].Lines) == 0 {
		if a.compareAnchorRow >= 0 && a.compareAnchorRow < len(a.compareRows) {
			return a.compareAnchorRow
		}
		if a.changeCursor >= 0 && a.changeCursor < len(a.compareChange) {
			return a.compareChange[a.changeCursor].RowStart
		}
		return 0
	}
	return max(0, len(a.compareRows)-1)
}

func (a *App) clickCompare(event tea.MouseEvent) {
	const firstDataRow = 3
	a.rowOffset = clamp(a.rowOffset, 0, max(0, len(a.compareRows)-a.fileVisibleRows()))
	visibleRow := event.Y - firstDataRow
	if visibleRow < 0 || visibleRow >= a.fileVisibleRows() {
		return
	}
	rowIndex := a.rowOffset + visibleRow
	if rowIndex < 0 || rowIndex >= len(a.compareRows) {
		return
	}
	row := a.compareRows[rowIndex]
	if row.Metadata {
		a.setStatus("Line-ending metadata is changed with Alt+Left or Alt+Right.", false)
		return
	}

	contentWidth := max(20, a.width-6)
	numWidth := len(fmt.Sprintf("%d", max(len(a.docs[0].Lines), len(a.docs[1].Lines), 1)))
	separatorWidth := lipgloss.Width(" │ Δ │ ")
	cellSpace := max(4, contentWidth-separatorWidth-2*(numWidth+3))
	leftWidth := cellSpace / 2
	rightWidth := cellSpace - leftWidth
	leftTextStart := 1 + numWidth + 3
	rightTextStart := leftTextStart + leftWidth + separatorWidth + numWidth + 3
	side := 0
	textStart, textWidth := leftTextStart, leftWidth
	lineNumber := row.LeftNo
	if event.X >= leftTextStart+leftWidth+separatorWidth/2 {
		side = 1
		textStart, textWidth = rightTextStart, rightWidth
		lineNumber = row.RightNo
	}
	if lineNumber == 0 && len(a.docs[side].Lines) > 0 {
		a.setStatus("There is no editable line at that position on this side.", false)
		return
	}

	line := 0
	if lineNumber > 0 {
		line = lineNumber - 1
	} else if row.Change >= 0 && row.Change < len(a.compareChange) {
		change := a.compareChange[row.Change]
		line = change.LeftStart
		if side == 1 {
			line = change.RightStart
		}
	}
	line = clamp(line, 0, maxCursorLine(a.docs[side]))
	value := ""
	if line < len(a.docs[side].Lines) {
		value = a.docs[side].Lines[line]
	}
	displayColumn := clamp(event.X-textStart, 0, max(0, textWidth-1))
	visualColumn := a.horizontalOffset[side] + displayColumn
	a.fileFocus = side
	a.inlineCursors[side] = inlineCursor{Line: line, Col: runeColumnAtVisual(value, visualColumn)}
	a.compareAnchorRow = rowIndex
	if row.Change >= 0 {
		a.changeCursor = row.Change
	}
	a.setStatus(fmt.Sprintf("Editing %s at line %d, column %d.", a.docs[side].Label, line+1, a.inlineCursors[side].Col+1), false)
}

func cursorVisualColumn(value string, runeColumn int) int {
	column := 0
	for index, r := range []rune(value) {
		if index >= runeColumn {
			break
		}
		if r == '\t' {
			column += 4 - column%4
		} else {
			column += max(1, lipgloss.Width(string(r)))
		}
	}
	return column
}

func runeColumnAtVisual(value string, visualColumn int) int {
	column := 0
	for index, r := range []rune(value) {
		width := max(1, lipgloss.Width(string(r)))
		if r == '\t' {
			width = 4 - column%4
		}
		if visualColumn < column+width {
			return index
		}
		column += width
	}
	return len([]rune(value))
}
