package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"merger/internal/core"
)

func (a *App) View() string {
	if a.width <= 0 || a.height <= 0 {
		return "Loading merger..."
	}
	if a.commands.open {
		return a.viewCommandPalette()
	}
	switch a.screen {
	case screenLoading:
		return a.viewLoading()
	case screenPicker:
		return a.viewPicker()
	case screenDirectory:
		return a.viewDirectory()
	case screenCompare:
		return a.viewCompare()
	case screenMerge:
		return a.viewMerge()
	case screenHelp:
		return a.viewHelp()
	default:
		return ""
	}
}

func (a *App) header(title string) string {
	left := styleLogo.Render("merger") + styleMuted.Render("  ·  ") + styleTitle.Render(title)
	return padVisual(left, a.width)
}

func (a *App) footer(hints ...string) string {
	controls := strings.Join(hints, styleMuted.Render("  "))
	if a.status != "" {
		status := styleSuccess.Render(a.status)
		if a.statusError {
			status = styleError.Render(a.status)
		}
		controls = status + styleMuted.Render("  │  ") + controls
	}
	return truncateVisual(controls, max(1, a.width))
}

func (a *App) panel(content string, height int) string {
	return styleBorder.Width(max(1, a.width-2)).Height(max(1, height)).Render(content)
}

func (a *App) viewLoading() string {
	content := styleTitle.Render(a.loadingLabel) + "\n\n" + styleMuted.Render("Only the requested files or current directory level are being read.")
	box := styleBorder.Padding(1, 3).Render(content)
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, box)
}

func (a *App) viewPicker() string {
	visible := a.browserVisibleRows()
	a.browserOffset = adjustOffset(a.browserOffset, a.browserCursor, visible)
	end := min(len(a.browserEntries), a.browserOffset+visible)
	contentWidth := max(20, a.width-4)

	first := "not selected"
	if a.browserFirst != "" {
		first = a.browserFirst
	}
	var body strings.Builder
	body.WriteString(styleMuted.Render("Current: ") + styleText.Render(truncateVisual(a.browserPath, max(1, contentWidth-9))) + "\n")
	body.WriteString(styleMuted.Render("First:   ") + styleTitle.Render(truncateVisual(first, max(1, contentWidth-9))) + "\n")
	query := styleMuted.Render("type a name to jump")
	if a.browserQuery != "" {
		query = styleModified.Render(a.browserQuery)
	}
	body.WriteString(styleMuted.Render("Find:    ") + truncateVisual(query, max(1, contentWidth-9)) + "\n")
	body.WriteString(styleMuted.Render(strings.Repeat("─", contentWidth)) + "\n")
	for i := a.browserOffset; i < end; i++ {
		entry := a.browserEntries[i]
		name := entryGlyph(entry.Kind) + entry.Name
		if entry.Kind == core.EntryDirectory {
			name += "/"
		}
		line := padVisual(name, contentWidth)
		if i == a.browserCursor {
			body.WriteString(styleFocus.Render(line) + "\n")
		} else if entry.Kind == core.EntryDirectory {
			body.WriteString(styleTitle.Render(line) + "\n")
		} else {
			body.WriteString(styleText.Render(line) + "\n")
		}
	}
	for i := end - a.browserOffset; i < visible; i++ {
		body.WriteString(strings.Repeat(" ", contentWidth) + "\n")
	}
	hidden := "hidden files off"
	if a.browserShowHidden {
		hidden = "hidden files on"
	}
	body.WriteString(styleMuted.Render(fmt.Sprintf("%d item(s) · %s · folders are loaded one level at a time", len(a.browserEntries), hidden)))

	return a.header("choose two files or directories") + "\n" + a.panel(body.String(), visible+5) + "\n" +
		a.footer(hint("↑↓", "move"), hint("Enter", "open/select file"), hint("Space", "select item"),
			hint("Tab", "select item"), hint("type", "jump"), hint("Backspace", "erase/parent"), hint(".", "hidden"), hint("Esc", "clear/back"))
}

func (a *App) viewDirectory() string {
	entries := a.visibleDirectoryEntries()
	visible := a.directoryVisibleRows()
	a.dirOffset = adjustOffset(a.dirOffset, a.dirCursor, visible)
	end := min(len(entries), a.dirOffset+visible)
	contentWidth := max(20, a.width-4)
	statusWidth := 14
	nameSpace := max(8, contentWidth-statusWidth-6)
	leftWidth := nameSpace / 2
	rightWidth := nameSpace - leftWidth

	leftHeader := "LEFT  " + a.dirLeft
	rightHeader := "RIGHT  " + a.dirRight
	if a.dirFocus == 0 {
		leftHeader = styleFocus.Render(padVisual(leftHeader, leftWidth))
	} else {
		leftHeader = styleTitle.Render(padVisual(leftHeader, leftWidth))
	}
	if a.dirFocus == 1 {
		rightHeader = styleFocus.Render(padVisual(rightHeader, rightWidth))
	} else {
		rightHeader = styleTitle.Render(padVisual(rightHeader, rightWidth))
	}

	var body strings.Builder
	query := styleMuted.Render("type a name to jump")
	if a.dirQuery != "" {
		query = styleModified.Render(a.dirQuery)
	}
	body.WriteString(styleMuted.Render("Find: ") + truncateVisual(query, max(1, contentWidth-6)) + "\n")
	body.WriteString(leftHeader + styleMuted.Render(" │ ") + rightHeader + styleMuted.Render(" │ ") +
		styleTitle.Render(padVisual("STATUS", statusWidth)) + "\n")
	body.WriteString(styleMuted.Render(strings.Repeat("─", contentWidth)) + "\n")
	for i := a.dirOffset; i < end; i++ {
		entry := entries[i]
		leftName, rightName := "", ""
		if entry.Left.Exists {
			leftName = entryGlyph(entry.Left.Kind) + entry.Name
		}
		if entry.Right.Exists {
			rightName = entryGlyph(entry.Right.Kind) + entry.Name
		}
		plain := padVisual(leftName, leftWidth) + " │ " + padVisual(rightName, rightWidth) + " │ " + padVisual(entry.Status.String(), statusWidth)
		if i == a.dirCursor {
			body.WriteString(styleFocus.Render(padVisual(plain, contentWidth)) + "\n")
			continue
		}
		style := directoryStatusStyle(entry.Status)
		body.WriteString(style.Render(padVisual(leftName, leftWidth)) + styleMuted.Render(" │ ") +
			style.Render(padVisual(rightName, rightWidth)) + styleMuted.Render(" │ ") +
			style.Render(padVisual(entry.Status.String(), statusWidth)) + "\n")
	}
	for i := end - a.dirOffset; i < visible; i++ {
		body.WriteString(strings.Repeat(" ", contentWidth) + "\n")
	}

	shown := "all entries"
	if !a.showSame {
		shown = "differences only"
	}
	body.WriteString(styleMuted.Render(fmt.Sprintf("%d item(s) · %s · directories are collapsed and loaded on demand", len(entries), shown)))

	hints := []string{hint("↑↓", "move"), hint("type", "jump"), hint("Enter", "open"), hint("Backspace", "erase/parent"),
		hint("Alt+←/→", "copy"), hint("Alt+Del", "delete focused"), hint("Tab", "focus"), hint("Ctrl+S", "same"), hint("F5", "refresh"), hint("?", "help")}
	if a.directoryFromPicker {
		hints = append(hints, hint("Esc", "picker"))
	} else {
		hints = append(hints, hint("Esc", "quit"))
	}
	return a.header("directory compare") + "\n" + a.panel(body.String(), visible+4) + "\n" + a.footer(hints...)
}

func entryGlyph(kind core.EntryKind) string {
	switch kind {
	case core.EntryDirectory:
		return "▸ "
	case core.EntrySymlink:
		return "↗ "
	default:
		return "· "
	}
}

func directoryStatusStyle(status core.EntryStatus) lipgloss.Style {
	switch status {
	case core.EntryLeftOnly:
		return styleDeleted
	case core.EntryRightOnly:
		return styleAdded
	case core.EntryDifferent, core.EntryTypeMismatch, core.EntryMetadataDifferent:
		return styleModified
	case core.EntryFolder:
		return styleTitle
	default:
		return styleSubtle
	}
}

func (a *App) viewCompare() string {
	if a.docs[0].Path == "" && a.statusError {
		return a.viewFatal("File comparison failed")
	}
	if a.docs[0].Binary || a.docs[1].Binary {
		return a.viewBinaryCompare()
	}

	visible := a.fileVisibleRows()
	a.rowOffset = clamp(a.rowOffset, 0, max(0, len(a.compareRows)-visible))
	contentWidth := max(20, a.width-6)
	numWidth := len(fmt.Sprintf("%d", max(len(a.docs[0].Lines), len(a.docs[1].Lines), 1)))
	separatorWidth := lipgloss.Width(" │ Δ │ ")
	cellSpace := max(4, contentWidth-separatorWidth-2*(numWidth+3))
	leftWidth := cellSpace / 2
	rightWidth := cellSpace - leftWidth

	leftTitle := a.config.LeftLabel + "  " + a.docs[0].Path
	rightTitle := a.config.RightLabel + "  " + a.docs[1].Path
	if a.docs[0].Dirty {
		leftTitle += "  ●"
	}
	if a.docs[1].Dirty {
		rightTitle += "  ●"
	}
	leftTitle = renderPaneTitle(leftTitle, leftWidth+numWidth+3, a.fileFocus == 0)
	rightTitle = renderPaneTitle(rightTitle, rightWidth+numWidth+3, a.fileFocus == 1)
	a.ensureInlineCursorVisible(0, leftWidth)
	a.ensureInlineCursorVisible(1, rightWidth)

	var body strings.Builder
	body.WriteString(leftTitle + styleMuted.Render(" │ Δ │ ") + rightTitle + "\n")
	for rowIndex := a.rowOffset; rowIndex < a.rowOffset+visible; rowIndex++ {
		if rowIndex >= len(a.compareRows) {
			body.WriteString(strings.Repeat(" ", contentWidth) + "\n")
			continue
		}
		row := a.compareRows[rowIndex]
		selected := rowIndex == a.searchRow || row.Change >= 0 && row.Change == a.changeCursor
		leftStage, rightStage := a.stagedKindForRow(row)
		leftCursor := a.cursorForCompareRow(0, rowIndex, row.LeftNo, row.Metadata)
		rightCursor := a.cursorForCompareRow(1, rowIndex, row.RightNo, row.Metadata)
		body.WriteString(renderCompareRow(row, numWidth, leftWidth, rightWidth, selected, leftStage, rightStage,
			leftCursor, rightCursor, a.config.Syntax, a.docs[0].Path, a.docs[1].Path) + "\n")
	}
	kinds := make([]core.ChangeKind, len(a.compareRows))
	for i, row := range a.compareRows {
		leftStage, rightStage := a.stagedKindForRow(row)
		kinds[i] = strongerKind(row.Kind, strongerKind(leftStage, rightStage))
	}
	overview := renderOverview(kinds, visible+1, a.rowOffset, visible)
	content := lipgloss.JoinHorizontal(lipgloss.Top, body.String(), " ", overview)

	options := []string{hint("arrows", "cursor"), hint("Alt+↑/↓", "change"), hint("Ctrl+F", "search"), hint("Alt+W/E", "ignore"), hint("Alt+S", "syntax")}
	if !a.config.ReadOnly {
		options = append(options, hint("type/click", "edit"), hint("Alt+←/→", "push"), hint("Ctrl+Z/Y", "undo/redo"), hint("Ctrl+S", "save"))
	} else {
		options = append(options, hint("mode", "read-only"))
	}
	options = append(options, hint("Tab", "side"), hint("Esc", "back"), hint("F1", "help"))
	return a.header("file compare") + "\n" + a.panel(content, visible+1) + "\n" + a.footer(options...)
}

func renderPaneTitle(title string, width int, focused bool) string {
	title = padVisual(truncateVisual(title, width), width)
	if focused {
		return styleFocus.Render(title)
	}
	return styleTitle.Render(title)
}

func (a *App) stagedKindForRow(row core.CompareRow) (core.ChangeKind, core.ChangeKind) {
	if row.Metadata {
		left, right := core.ChangeSame, core.ChangeSame
		if a.stagedFormat[0] {
			left = core.ChangeModified
		}
		if a.stagedFormat[1] {
			right = core.ChangeModified
		}
		return left, right
	}
	return a.stagedKindForSide(0, row.LeftNo), a.stagedKindForSide(1, row.RightNo)
}

func (a *App) stagedKindForSide(side, lineNumber int) core.ChangeKind {
	if lineNumber > 0 && lineNumber-1 < len(a.stagedLineKinds[side]) {
		if kind := a.stagedLineKinds[side][lineNumber-1]; kind != core.ChangeSame {
			return kind
		}
	}
	if len(a.stagedDeleteAt[side]) == 0 {
		return core.ChangeSame
	}
	if lineNumber == 0 && len(a.docs[side].Lines) == 0 && a.stagedDeleteAt[side][0] {
		return core.ChangeDeleted
	}
	if lineNumber > 0 {
		index := lineNumber - 1
		if a.stagedDeleteAt[side][index] || (index == len(a.docs[side].Lines)-1 && a.stagedDeleteAt[side][len(a.docs[side].Lines)]) {
			return core.ChangeDeleted
		}
	}
	return core.ChangeSame
}

func renderCompareRow(row core.CompareRow, numWidth, leftWidth, rightWidth int, selected bool, leftStage, rightStage core.ChangeKind, leftCursor, rightCursor cellCursor, syntax bool, leftPath, rightPath string) string {
	baseStyle := changeStyle(row.Kind)
	if selected {
		baseStyle = baseStyle.Background(lipgloss.Color(moSurface0)).Bold(true)
	}
	leftStyle, rightStyle := baseStyle, baseStyle
	if leftStage != core.ChangeSame {
		leftStyle = stagedChangeStyle(leftStage)
	}
	if rightStage != core.ChangeSame {
		rightStyle = stagedChangeStyle(rightStage)
	}
	leftNo, rightNo := strings.Repeat(" ", numWidth), strings.Repeat(" ", numWidth)
	if row.LeftNo > 0 {
		leftNo = fmt.Sprintf("%*d", numWidth, row.LeftNo)
	}
	if row.RightNo > 0 {
		rightNo = fmt.Sprintf("%*d", numWidth, row.RightNo)
	}
	marker := "="
	switch row.Kind {
	case core.ChangeAdded:
		marker = "+"
	case core.ChangeDeleted:
		marker = "-"
	case core.ChangeModified:
		marker = "~"
	}
	markerStyle := baseStyle
	if leftStage != core.ChangeSame || rightStage != core.ChangeSame {
		marker = "◆"
		markerStyle = stagedChangeStyle(strongerKind(leftStage, rightStage))
	}
	leftHighlight, rightHighlight := inlineChangeRanges(row.Left, row.Right, row.Kind == core.ChangeModified)
	return styleMuted.Render(leftNo+" │ ") + renderDecoratedCell(row.Left, leftWidth, leftStyle, leftCursor, leftHighlight, syntaxDecorations(row.Left, leftPath, syntax && row.Kind == core.ChangeSame)) +
		styleMuted.Render(" │ ") + markerStyle.Render(marker) + styleMuted.Render(" │ ") +
		styleMuted.Render(rightNo+" │ ") + renderDecoratedCell(row.Right, rightWidth, rightStyle, rightCursor, rightHighlight, syntaxDecorations(row.Right, rightPath, syntax && row.Kind == core.ChangeSame))
}

func renderEditableCell(value string, width int, textStyle lipgloss.Style, cursor cellCursor) string {
	return renderDecoratedCell(value, width, textStyle, cursor, visualRange{}, nil)
}

type visualRange struct{ start, end int }

type syntaxClass uint8

const (
	syntaxNone syntaxClass = iota
	syntaxKeyword
	syntaxString
	syntaxNumber
	syntaxComment
)

func renderDecoratedCell(value string, width int, textStyle lipgloss.Style, cursor cellCursor, highlight visualRange, syntax []syntaxClass) string {
	expanded := expandTabs(value)
	if width <= 0 {
		return ""
	}
	cursorColumn := cursorVisualColumn(value, cursor.RuneColumn)
	var styled strings.Builder
	visual := 0
	for index, character := range []rune(expanded) {
		cellWidth := max(1, lipgloss.Width(string(character)))
		style := textStyle
		if index < len(syntax) {
			switch syntax[index] {
			case syntaxKeyword:
				style = style.Foreground(lipgloss.Color(moMauve)).Bold(true)
			case syntaxString:
				style = style.Foreground(lipgloss.Color(moGreen))
			case syntaxNumber:
				style = style.Foreground(lipgloss.Color(moPeach))
			case syntaxComment:
				style = style.Foreground(lipgloss.Color(moOverlay1)).Italic(true)
			}
		}
		if visual < highlight.end && visual+cellWidth > highlight.start {
			style = style.Background(lipgloss.Color(moSurface1)).Underline(true).Bold(true)
		}
		if cursor.Show && visual <= cursorColumn && cursorColumn < visual+cellWidth {
			style = styleCursor
		}
		styled.WriteString(style.Render(string(character)))
		visual += cellWidth
	}
	visible := ansi.Cut(styled.String(), cursor.Offset, cursor.Offset+width)
	return visible + textStyle.Render(strings.Repeat(" ", max(0, width-lipgloss.Width(visible))))
}

func (a *App) viewBinaryCompare() string {
	status := styleSuccess.Render("identical")
	if len(a.compareChange) > 0 {
		status = styleModified.Render("different")
	}
	content := styleTitle.Render("Binary comparison") + "\n\n" +
		styleText.Render(describeBinary(a.docs[0])) + "\n" +
		styleText.Render(describeBinary(a.docs[1])) + "\n\n" +
		styleMuted.Render("Status: ") + status + "\n\n" +
		styleMuted.Render("Alt+Right copies the left bytes into the right document; Alt+Left copies right into left. Changes remain in memory until Ctrl+S.")
	return a.header("binary compare") + "\n" + a.panel(content, max(8, a.height-5)) + "\n" +
		a.footer(hint("Alt+←/→", "push"), hint("Ctrl+Z", "undo"), hint("Ctrl+Y", "redo"), hint("Ctrl+S", "save"), hint("Esc", "back/quit"), hint("F1", "help"))
}

func (a *App) viewMerge() string {
	if a.mine.Path == "" && a.statusError {
		return a.viewFatal("Three-way merge failed")
	}
	if a.mergeBinary {
		return a.viewBinaryMerge()
	}
	visible := a.fileVisibleRows()
	a.rowOffset = clamp(a.rowOffset, 0, max(0, len(a.mergeRows)-visible))
	contentWidth := max(30, a.width-6)
	layout := a.mergeLayout()
	a.ensureMergeCursorVisible(layout.resultWidth)

	var body strings.Builder
	body.WriteString(renderPaneTitle("MINE", layout.mineWidth+layout.numWidth+1, false) + styleMuted.Render(" │ ") +
		renderPaneTitle("MERGED RESULT", layout.resultWidth+layout.numWidth+1, true) + styleMuted.Render(" │ ") +
		renderPaneTitle("THEIRS", layout.theirWidth+layout.numWidth+1, false) + "\n")
	for rowIndex := a.rowOffset; rowIndex < a.rowOffset+visible; rowIndex++ {
		if rowIndex >= len(a.mergeRows) {
			body.WriteString(strings.Repeat(" ", contentWidth) + "\n")
			continue
		}
		row := a.mergeRows[rowIndex]
		selected := row.Chunk >= 0 && row.Chunk == a.changeCursor
		cursor := a.cursorForMergeRow(rowIndex, row.ResultNo)
		body.WriteString(renderMergeRow(row, layout, selected, cursor) + "\n")
	}
	kinds := make([]core.ChangeKind, len(a.mergeRows))
	for i, row := range a.mergeRows {
		kinds[i] = row.Kind
	}
	overview := renderOverview(kinds, visible+1, a.rowOffset, visible)
	content := lipgloss.JoinHorizontal(lipgloss.Top, body.String(), " ", overview)

	progress := fmt.Sprintf("%d changes · %d unresolved · output: %s", len(a.mergePlan.Chunks), a.mergePlan.UnresolvedCount(), a.config.Output)
	return a.header("three-way merge") + "\n" + styleMuted.Render(truncateVisual(progress, a.width)) + "\n" +
		a.panel(content, visible+1) + "\n" +
		a.footer(hint("type/click", "edit result"), hint("arrows", "cursor"), hint("Alt+↑/↓", "change"),
			hint("Alt+→", "mine"), hint("Alt+←", "theirs"), hint("Alt+B/A/U", "base/both/unresolve"),
			hint("Ctrl+Z", "undo"), hint("Ctrl+S", "save"), hint("Esc", "cancel"), hint("F1", "help"))
}

func renderMergeRow(row core.MergeRow, layout mergeLayout, selected bool, cursor cellCursor) string {
	style := changeStyle(row.Kind)
	if selected {
		style = style.Background(lipgloss.Color(moSurface0)).Bold(true)
	}
	number := func(value int) string {
		if value <= 0 {
			return strings.Repeat(" ", layout.numWidth) + " "
		}
		return fmt.Sprintf("%*d ", layout.numWidth, value)
	}
	return styleMuted.Render(number(row.MineNo)) + style.Render(padVisual(expandTabs(row.Mine), layout.mineWidth)) + styleMuted.Render(" │ ") +
		styleMuted.Render(number(row.ResultNo)) + renderEditableCell(row.Result, layout.resultWidth, style, cursor) + styleMuted.Render(" │ ") +
		styleMuted.Render(number(row.TheirNo)) + style.Render(padVisual(expandTabs(row.Theirs), layout.theirWidth))
}

func (a *App) viewBinaryMerge() string {
	choice := a.mergeBinaryChoice.String()
	choiceStyle := styleSuccess
	if a.mergeBinaryChoice == core.ResolutionUnresolved {
		choiceStyle = styleConflict
	}
	content := styleTitle.Render("Binary three-way merge") + "\n\n" +
		styleText.Render("MINE    "+describeBinary(a.mine)) + "\n" +
		styleText.Render("BASE    "+describeBinary(a.base)) + "\n" +
		styleText.Render("THEIRS  "+describeBinary(a.theirs)) + "\n\n" +
		styleMuted.Render("Resolution: ") + choiceStyle.Render(choice) + "\n" +
		styleMuted.Render("Output:     ") + styleText.Render(a.config.Output)
	return a.header("binary three-way merge") + "\n" + a.panel(content, max(9, a.height-5)) + "\n" +
		a.footer(hint("Alt+→", "mine"), hint("Alt+←", "theirs"), hint("Alt+B", "base"), hint("Ctrl+S", "save"), hint("Esc", "cancel"))
}

func (a *App) viewHelp() string {
	help := styleTitle.Render("Navigation") + "\n" +
		"  Alt+Up / Alt+Down     previous / next change\n" +
		"  Up / Down, PgUp/PgDn  scroll\n" +
		"  Home / End             top / bottom\n\n" +
		styleTitle.Render("File picker") + "\n" +
		"  Enter opens a folder or selects a regular file\n" +
		"  Space or Tab selects the highlighted file or folder without opening it\n" +
		"  Typing jumps to the matching name; Backspace erases the query or opens the parent\n" +
		"  . toggles hidden files; Esc clears the query, first choice, or exits\n\n" +
		styleTitle.Render("Meld-compatible change actions") + "\n" +
		"  Alt+Right              push current change to the right\n" +
		"  Alt+Left               push current change to the left\n" +
		"  Alt+Delete             delete current change / focused directory entry\n" +
		"  ◆                      changed on disk only after Ctrl+S\n\n" +
		styleTitle.Render("Inline file editing") + "\n" +
		"  Click a character or use arrows to place the active cursor\n" +
		"  Typing, Enter, Backspace and Delete edit the focused file immediately\n" +
		"  Tab changes sides; Ctrl+Z undoes; Ctrl+Shift+Z or Ctrl+Y redoes\n" +
		"  F1 opens help; F5 reloads; Esc returns or quits\n\n" +
		"  Ctrl+Shift+P / Ctrl+P opens the fuzzy command palette\n\n" +
		styleTitle.Render("Directory compare") + "\n" +
		"  Enter opens a collapsed directory or compares a file\n" +
		"  Backspace moves both sides to their parent\n" +
		"  Typing jumps to a matching entry; Backspace first erases that query\n" +
		"  Tab changes the focused side; Ctrl+S hides/shows identical entries; F5 refreshes\n" +
		"  Directories are read one level at a time, never indexed recursively\n\n" +
		styleTitle.Render("Three-way merge") + "\n" +
		"  Click or use the arrows to place the cursor in MERGED RESULT, then type\n" +
		"  Tab indents; the result is the only editable pane, so it never changes focus\n" +
		"  Typing, Enter, Backspace and Delete edit the result in place; no separate editor\n" +
		"  Alt+Right accepts MINE; Alt+Left accepts THEIRS\n" +
		"  Alt+B accepts BASE; Alt+A concatenates both; Alt+U restores conflict markers\n" +
		"  Ctrl+Z undoes; Ctrl+Shift+Z or Ctrl+Y redoes; F5 reloads all three inputs\n" +
		"  A chunk still holding conflict markers stays unresolved and blocks Ctrl+S\n" +
		"  Ctrl+S saves when every conflict is resolved; Esc cancels with status 2"
	return a.header("help") + "\n" + a.panel(help, max(12, a.height-5)) + "\n" + a.footer(hint("any key", "back"))
}

func (a *App) viewFatal(title string) string {
	return a.header(title) + "\n" + a.panel(styleError.Render(a.status), max(5, a.height-5)) + "\n" + a.footer(hint("q", "quit"))
}

func changeStyle(kind core.ChangeKind) lipgloss.Style {
	switch kind {
	case core.ChangeAdded:
		return styleAdded
	case core.ChangeDeleted:
		return styleDeleted
	case core.ChangeModified:
		return styleModified
	case core.ChangeConflict:
		return styleConflict
	default:
		return styleSubtle
	}
}

func stagedChangeStyle(kind core.ChangeKind) lipgloss.Style {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(moTeal)).Background(lipgloss.Color(moSurface0)).Bold(true)
	switch kind {
	case core.ChangeAdded:
		return style.Foreground(lipgloss.Color(moGreen))
	case core.ChangeDeleted:
		return style.Foreground(lipgloss.Color(moRed))
	case core.ChangeModified:
		return style.Foreground(lipgloss.Color(moYellow))
	default:
		return style
	}
}

// renderOverview draws a whole-file change map and, in a dedicated column next
// to it, a scrollbar whose thumb marks the visible part of the file. The map
// uses ++, --, ~~ and !! so it remains meaningful without colour.
func renderOverview(kinds []core.ChangeKind, height, offset, visible int) string {
	if height <= 0 {
		return ""
	}
	total := max(1, len(kinds))
	thumbStart := clamp(offset*height/total, 0, height-1)
	thumbEnd := clamp(divCeil((offset+visible)*height, total)-1, thumbStart, height-1)
	var result strings.Builder
	for row := range height {
		from := row * total / height
		to := min(len(kinds), max(from+1, divCeil((row+1)*total, height)))
		kind := core.ChangeSame
		for i := from; i < to; i++ {
			kind = strongerKind(kind, kinds[i])
		}
		glyph := styleMuted.Render("··")
		switch kind {
		case core.ChangeAdded:
			glyph = styleAdded.Bold(true).Render("++")
		case core.ChangeDeleted:
			glyph = styleDeleted.Bold(true).Render("--")
		case core.ChangeModified:
			glyph = styleModified.Bold(true).Render("~~")
		case core.ChangeConflict:
			glyph = styleConflict.Bold(true).Render("!!")
		}
		bar := styleScrollTrack.Render("│")
		if row >= thumbStart && row <= thumbEnd {
			bar = styleScrollThumb.Render("█")
		}
		if row > 0 {
			result.WriteByte('\n')
		}
		result.WriteString(glyph + bar)
	}
	return result.String()
}

func strongerKind(left, right core.ChangeKind) core.ChangeKind {
	if left == core.ChangeSame {
		return right
	}
	if right == core.ChangeSame || left == right {
		return left
	}
	if left == core.ChangeConflict || right == core.ChangeConflict {
		return core.ChangeConflict
	}
	return core.ChangeModified
}

func divCeil(value, divisor int) int {
	if value <= 0 || divisor <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}

func truncateVisual(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	return ansi.Truncate(value, max(0, width-1), "") + "…"
}

func padVisual(value string, width int) string {
	value = truncateVisual(value, width)
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}

func expandTabs(value string) string {
	var result strings.Builder
	column := 0
	for _, r := range value {
		if r == '\t' {
			spaces := 4 - column%4
			result.WriteString(strings.Repeat(" ", spaces))
			column += spaces
			continue
		}
		if unicode.IsControl(r) {
			result.WriteRune('·')
			column++
			continue
		}
		result.WriteRune(r)
		column += max(1, lipgloss.Width(string(r)))
	}
	return result.String()
}

func shortPath(path string, width int) string {
	if lipgloss.Width(path) <= width {
		return path
	}
	return "…/" + truncateVisual(filepath.Base(path), max(1, width-2))
}
