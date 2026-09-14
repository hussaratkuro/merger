package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"merger/internal/core"
)

type screen uint8

const (
	screenLoading screen = iota
	screenPicker
	screenDirectory
	screenCompare
	screenMerge
	screenHelp
)

type directoryLocation struct {
	left, right string
	cursor      int
	offset      int
}

type pendingDirectoryAction struct {
	kind      string
	name      string
	direction int
}

type inlineCursor struct {
	Line int
	Col  int
}

type compareSnapshot struct {
	Docs             [2]core.Document
	Cursors          [2]inlineCursor
	HorizontalOffset [2]int
	FileFocus        int
	RowOffset        int
	ChangeCursor     int
	AnchorRow        int
}

type App struct {
	config Config

	width, height int
	screen        screen
	returnScreen  screen
	loadingLabel  string
	status        string
	statusError   bool
	fatalErr      error

	browserPath       string
	browserEntries    []core.BrowserEntry
	browserCursor     int
	browserOffset     int
	browserShowHidden bool
	browserFirst      string
	browserQuery      string
	browserQueryID    int

	dirLeft, dirRight string
	dirEntries        []core.DirectoryEntry
	dirCursor         int
	dirOffset         int
	dirHistory        []directoryLocation
	dirFocus          int
	showSame          bool
	dirPending        pendingDirectoryAction
	dirBusy           bool
	dirQuery          string
	dirQueryID        int

	docs                [2]core.Document
	originalDocs        [2]core.Document
	compareBaselineSet  bool
	compareRows         []core.CompareRow
	compareChange       []core.CompareChange
	stagedLineKinds     [2][]core.ChangeKind
	stagedDeleteAt      [2]map[int]bool
	stagedFormat        [2]bool
	changeCursor        int
	compareAnchorRow    int
	rowOffset           int
	fileFocus           int
	inlineCursors       [2]inlineCursor
	horizontalOffset    [2]int
	compareUndo         []compareSnapshot
	compareRedo         []compareSnapshot
	compareFromDir      bool
	compareFromPicker   bool
	directoryFromPicker bool

	mine, base, theirs core.Document
	mergePlan          core.MergePlan
	mergeRows          []core.MergeRow
	mergeChanges       []core.CompareChange
	mergeBinary        bool
	mergeBinaryResult  []byte
	mergeBinaryChoice  core.Resolution

	mergeResult           []string
	mergeCursor           inlineCursor
	mergeHorizontalOffset int
	mergeUndo             []mergeSnapshot
	mergeRedo             []mergeSnapshot

	confirmKey string
	saved      bool
	cancelled  bool
	saving     bool
}

func New(config Config) *App {
	a := &App{
		config: config, screen: screenLoading, loadingLabel: "Opening comparison...",
		showSame: true, dirFocus: 0, fileFocus: 0, changeCursor: -1, compareAnchorRow: -1,
	}
	if config.Mode == ModeDirectory {
		a.dirLeft, a.dirRight = config.Left, config.Right
		a.loadingLabel = "Reading this directory level..."
	} else if config.Mode == ModePicker {
		a.browserPath = config.BrowseRoot
		a.loadingLabel = "Opening file explorer..."
	}
	return a
}

func (a *App) Init() tea.Cmd {
	switch a.config.Mode {
	case ModePicker:
		return loadBrowserCmd(a.browserPath, a.browserShowHidden)
	case ModeDirectory:
		return loadDirectoryCmd(a.dirLeft, a.dirRight)
	case ModeMerge:
		return loadMergeCmd(a.config)
	default:
		return loadCompareCmd(a.config.Left, a.config.Right, true, true, false)
	}
}

func (a *App) SuccessfulOutput() bool {
	return a.config.Mode != ModeMerge || a.saved
}

func (a *App) Cancelled() bool { return a.cancelled }

func (a *App) FatalError() error { return a.fatalErr }

type directoryLoadedMsg struct {
	left, right string
	entries     []core.DirectoryEntry
	err         error
}

type browserLoadedMsg struct {
	path    string
	entries []core.BrowserEntry
	err     error
}

type compareLoadedMsg struct {
	left, right core.Document
	fromDir     bool
	err         error
}

type mergeLoadedMsg struct {
	mine, base, theirs core.Document
	err                error
}

type directoryOperationMsg struct {
	description string
	err         error
}

type compareSavedMsg struct {
	err error
}

type mergeSavedMsg struct {
	err error
}

type browserQueryExpiredMsg struct {
	id int
}

type directoryQueryExpiredMsg struct {
	id int
}

func loadDirectoryCmd(left, right string) tea.Cmd {
	return func() tea.Msg {
		entries, err := core.CompareDirectory(left, right)
		return directoryLoadedMsg{left: left, right: right, entries: entries, err: err}
	}
}

func loadBrowserCmd(path string, showHidden bool) tea.Cmd {
	return func() tea.Msg {
		entries, err := core.ListDirectory(path, showHidden)
		return browserLoadedMsg{path: path, entries: entries, err: err}
	}
}

func loadCompareCmd(leftPath, rightPath string, leftExists, rightExists, fromDir bool) tea.Cmd {
	return func() tea.Msg {
		read := func(path, label string, exists bool) (core.Document, error) {
			if !exists {
				return core.Document{Path: path, Label: label, EOL: "\n", Mode: 0o644}, nil
			}
			return core.ReadDocument(path, label)
		}
		left, err := read(leftPath, "LEFT", leftExists)
		if err != nil {
			return compareLoadedMsg{err: err, fromDir: fromDir}
		}
		right, err := read(rightPath, "RIGHT", rightExists)
		return compareLoadedMsg{left: left, right: right, fromDir: fromDir, err: err}
	}
}

func loadMergeCmd(config Config) tea.Cmd {
	return func() tea.Msg {
		mine, err := core.ReadDocument(config.Mine, "MINE")
		if err != nil {
			return mergeLoadedMsg{err: err}
		}
		base, err := core.ReadDocument(config.Base, "BASE")
		if err != nil {
			return mergeLoadedMsg{err: err}
		}
		theirs, err := core.ReadDocument(config.Theirs, "THEIRS")
		return mergeLoadedMsg{mine: mine, base: base, theirs: theirs, err: err}
	}
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return a, nil
	case browserQueryExpiredMsg:
		if msg.id == a.browserQueryID {
			a.browserQuery = ""
		}
		return a, nil
	case directoryQueryExpiredMsg:
		if msg.id == a.dirQueryID {
			a.dirQuery = ""
		}
		return a, nil
	case browserLoadedMsg:
		if msg.err != nil {
			a.setStatus(msg.err.Error(), true)
			a.screen = screenPicker
			return a, nil
		}
		a.browserPath, a.browserEntries = msg.path, msg.entries
		a.browserCursor = clamp(a.browserCursor, 0, max(0, len(a.browserEntries)-1))
		a.browserOffset = adjustOffset(a.browserOffset, a.browserCursor, a.browserVisibleRows())
		a.screen = screenPicker
		return a, nil
	case directoryLoadedMsg:
		a.dirBusy = false
		if msg.err != nil {
			if len(a.dirHistory) == 0 && len(a.dirEntries) == 0 {
				a.fatalErr = msg.err
			}
			a.setStatus(msg.err.Error(), true)
			if len(a.dirHistory) > 0 {
				previous := a.dirHistory[len(a.dirHistory)-1]
				a.dirHistory = a.dirHistory[:len(a.dirHistory)-1]
				a.dirLeft, a.dirRight = previous.left, previous.right
				a.dirCursor, a.dirOffset = previous.cursor, previous.offset
			}
			a.screen = screenDirectory
			return a, nil
		}
		a.dirLeft, a.dirRight, a.dirEntries = msg.left, msg.right, msg.entries
		a.dirCursor = clamp(a.dirCursor, 0, max(0, len(a.visibleDirectoryEntries())-1))
		a.screen = screenDirectory
		return a, nil
	case compareLoadedMsg:
		if msg.err != nil {
			if !msg.fromDir {
				a.fatalErr = msg.err
			}
			a.setStatus(msg.err.Error(), true)
			if msg.fromDir {
				a.screen = screenDirectory
			} else {
				a.screen = screenCompare
			}
			return a, nil
		}
		a.docs = [2]core.Document{msg.left, msg.right}
		a.originalDocs = [2]core.Document{cloneDocument(msg.left), cloneDocument(msg.right)}
		a.compareBaselineSet = true
		a.compareFromDir = msg.fromDir
		a.status, a.statusError = "", false
		a.rebuildCompare()
		a.resetInlineEditing()
		if msg.fromDir && len(a.compareChange) == 0 {
			a.setStatus("Exact contents and line endings match; the directory status reflected metadata only.", false)
		}
		a.screen = screenCompare
		return a, nil
	case mergeLoadedMsg:
		if msg.err != nil {
			a.fatalErr = msg.err
			a.setStatus(msg.err.Error(), true)
			a.screen = screenMerge
			return a, nil
		}
		a.mine, a.base, a.theirs = msg.mine, msg.base, msg.theirs
		a.initializeMerge()
		a.screen = screenMerge
		return a, nil
	case directoryOperationMsg:
		a.dirBusy = true
		if msg.err != nil {
			a.setStatus(msg.err.Error(), true)
		} else {
			a.setStatus(msg.description, false)
		}
		return a, loadDirectoryCmd(a.dirLeft, a.dirRight)
	case compareSavedMsg:
		a.saving = false
		if msg.err != nil {
			a.setStatus(msg.err.Error(), true)
		} else {
			offset := a.rowOffset
			a.docs[0].Dirty, a.docs[1].Dirty = false, false
			a.originalDocs = [2]core.Document{cloneDocument(a.docs[0]), cloneDocument(a.docs[1])}
			a.compareBaselineSet = true
			a.rebuildCompare()
			a.rowOffset = clamp(offset, 0, max(0, len(a.compareRows)-a.fileVisibleRows()))
			a.setStatus("Saved changed file(s).", false)
		}
		return a, nil
	case mergeSavedMsg:
		a.saving = false
		if msg.err != nil {
			a.setStatus(msg.err.Error(), true)
		} else {
			a.saved = true
			a.setStatus("Merge result saved to "+a.config.Output, false)
		}
		return a, nil
	case tea.MouseMsg:
		return a.updateMouse(msg)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if a.saving {
				a.setStatus("Saving; wait for the write to finish before quitting.", false)
				return a, nil
			}
			a.cancelled = !a.saved
			return a, tea.Quit
		}
		switch a.screen {
		case screenDirectory:
			return a.updateDirectory(msg)
		case screenPicker:
			return a.updatePicker(msg)
		case screenCompare:
			return a.updateCompare(msg)
		case screenMerge:
			return a.updateMerge(msg)
		case screenHelp:
			a.screen = a.returnScreen
			return a, nil
		}
	}
	return a, nil
}

func (a *App) setStatus(message string, isError bool) {
	a.status, a.statusError = message, isError
}

func (a *App) rebuildCompare() {
	a.rowOffset = 0
	a.compareAnchorRow = -1
	if !a.compareBaselineSet {
		a.originalDocs = [2]core.Document{cloneDocument(a.docs[0]), cloneDocument(a.docs[1])}
		a.compareBaselineSet = true
	}
	a.rebuildStagedChanges()
	if a.docs[0].Binary || a.docs[1].Binary {
		a.compareRows = nil
		a.compareChange = nil
		if !bytes.Equal(a.docs[0].Data, a.docs[1].Data) {
			a.compareChange = []core.CompareChange{{}}
			a.changeCursor = 0
		} else {
			a.changeCursor = -1
		}
		return
	}
	a.compareRows, a.compareChange = core.AlignDocuments(a.docs[0].Lines, a.docs[1].Lines)
	if a.docs[0].FinalNewline || a.docs[1].FinalNewline {
		endRow := core.CompareRow{Change: -1}
		if a.docs[0].FinalNewline {
			endRow.LeftNo = len(a.docs[0].Lines) + 1
		}
		if a.docs[1].FinalNewline {
			endRow.RightNo = len(a.docs[1].Lines) + 1
		}
		a.compareRows = append(a.compareRows, endRow)
	}
	formatDifferent := documentFormatSignature(a.docs[0]) != documentFormatSignature(a.docs[1])
	if formatDifferent || a.stagedFormat[0] || a.stagedFormat[1] {
		changeIndex := -1
		kind := core.ChangeSame
		if formatDifferent {
			changeIndex = len(a.compareChange)
			kind = core.ChangeModified
		}
		rowStart := len(a.compareRows)
		a.compareRows = append(a.compareRows, core.CompareRow{
			Left: lineEndingDescription(a.docs[0]), Right: lineEndingDescription(a.docs[1]),
			Kind: kind, Change: changeIndex, Metadata: true,
		})
		if formatDifferent {
			a.compareChange = append(a.compareChange, core.CompareChange{
				RowStart: rowStart, RowEnd: rowStart + 1,
				LeftStart: len(a.docs[0].Lines), LeftEnd: len(a.docs[0].Lines),
				RightStart: len(a.docs[1].Lines), RightEnd: len(a.docs[1].Lines),
				Metadata: true,
			})
		}
	}
	if len(a.compareRows) == 0 {
		a.compareRows = []core.CompareRow{{Change: -1}}
	}
	if len(a.compareChange) > 0 {
		a.changeCursor = clamp(a.changeCursor, 0, len(a.compareChange)-1)
		if a.changeCursor < 0 {
			a.changeCursor = 0
		}
		a.rowOffset = a.compareChange[a.changeCursor].RowStart
	} else {
		a.changeCursor = -1
	}
}

func cloneDocument(document core.Document) core.Document {
	document.Lines = slices.Clone(document.Lines)
	document.Data = slices.Clone(document.Data)
	return document
}

func (a *App) rebuildStagedChanges() {
	for side := range 2 {
		a.stagedLineKinds[side], a.stagedDeleteAt[side] = stagedChanges(a.originalDocs[side], a.docs[side])
		a.stagedFormat[side] = documentFormatSignature(a.originalDocs[side]) != documentFormatSignature(a.docs[side])
	}
}

func documentFormatSignature(document core.Document) string {
	if len(document.Lines) <= 1 && !document.FinalNewline {
		return "no-line-break"
	}
	return document.EOL + fmt.Sprintf("|final=%t", document.FinalNewline)
}

func lineEndingDescription(document core.Document) string {
	if len(document.Lines) <= 1 && !document.FinalNewline {
		return "∅ no line break / no final newline"
	}
	ending := "LF"
	if document.EOL == "\r\n" {
		ending = "CRLF"
	}
	final := "no final newline"
	if document.FinalNewline {
		final = "final newline"
	}
	return "⏎ " + ending + " · " + final
}

func stagedChanges(original, current core.Document) ([]core.ChangeKind, map[int]bool) {
	kinds := make([]core.ChangeKind, len(current.Lines))
	deletions := make(map[int]bool)
	basePosition, currentPosition := 0, 0
	for _, edit := range core.DiffEdits(original.Lines, current.Lines) {
		currentPosition += edit.BaseStart - basePosition
		removed := edit.BaseEnd - edit.BaseStart
		kind := core.ChangeModified
		switch {
		case removed == 0:
			kind = core.ChangeAdded
		case len(edit.NewLines) == 0:
			kind = core.ChangeDeleted
		}
		if len(edit.NewLines) == 0 {
			deletions[currentPosition] = true
		} else {
			for index := currentPosition; index < currentPosition+len(edit.NewLines) && index < len(kinds); index++ {
				kinds[index] = kind
			}
		}
		currentPosition += len(edit.NewLines)
		basePosition = edit.BaseEnd
	}
	return kinds, deletions
}

func (a *App) initializeMerge() {
	a.saved = false
	a.mergeBinary = a.mine.Binary || a.base.Binary || a.theirs.Binary
	if a.mergeBinary {
		switch {
		case bytes.Equal(a.mine.Data, a.theirs.Data):
			a.mergeBinaryResult = slices.Clone(a.mine.Data)
			a.mergeBinaryChoice = core.ResolutionAuto
		case bytes.Equal(a.mine.Data, a.base.Data):
			a.mergeBinaryResult = slices.Clone(a.theirs.Data)
			a.mergeBinaryChoice = core.ResolutionAuto
		case bytes.Equal(a.theirs.Data, a.base.Data):
			a.mergeBinaryResult = slices.Clone(a.mine.Data)
			a.mergeBinaryChoice = core.ResolutionAuto
		default:
			a.mergeBinaryChoice = core.ResolutionUnresolved
		}
		a.changeCursor = 0
		a.mergeResult = nil
		a.resetMergeEditing()
		return
	}
	a.mergePlan = core.BuildMergePlan(a.base.Lines, a.mine.Lines, a.theirs.Lines)
	a.rebuildMergeRows()
	if len(a.mergeChanges) > 0 {
		a.changeCursor = 0
		for i, chunk := range a.mergePlan.Chunks {
			if chunk.Resolution == core.ResolutionUnresolved {
				a.changeCursor = i
				break
			}
		}
		a.rowOffset = a.mergeChanges[a.changeCursor].RowStart
	} else {
		a.changeCursor = -1
	}
	a.resetMergeEditing()
}

// refreshMergeRows re-renders the plan without moving the viewport, so a typed
// edit leaves the cursor and the scroll position where the user put them.
func (a *App) refreshMergeRows() {
	a.mergeRows, a.mergeChanges = a.mergePlan.Rows()
	a.mergeResult = a.mergePlan.ResultLines()
}

func (a *App) rebuildMergeRows() {
	a.refreshMergeRows()
	if len(a.mergeChanges) == 0 {
		a.changeCursor = -1
		return
	}
	a.changeCursor = clamp(a.changeCursor, 0, len(a.mergeChanges)-1)
	a.rowOffset = a.mergeChanges[a.changeCursor].RowStart
}

func (a *App) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	event := tea.MouseEvent(msg)
	if event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionPress {
		switch a.screen {
		case screenCompare:
			a.clickCompare(event)
			return a, nil
		case screenMerge:
			a.clickMerge(event)
			return a, nil
		}
	}
	delta := 0
	switch msg.Type {
	case tea.MouseWheelUp:
		delta = -3
	case tea.MouseWheelDown:
		delta = 3
	default:
		return a, nil
	}
	switch a.screen {
	case screenPicker:
		a.browserCursor = clamp(a.browserCursor+delta, 0, max(0, len(a.browserEntries)-1))
		a.browserOffset = adjustOffset(a.browserOffset, a.browserCursor, a.browserVisibleRows())
	case screenDirectory:
		a.clearDirectoryQuery()
		visible := a.directoryVisibleRows()
		entries := a.visibleDirectoryEntries()
		a.dirCursor = clamp(a.dirCursor+delta, 0, max(0, len(entries)-1))
		a.dirOffset = adjustOffset(a.dirOffset, a.dirCursor, visible)
	case screenCompare:
		a.rowOffset = clamp(a.rowOffset+delta, 0, max(0, len(a.compareRows)-a.fileVisibleRows()))
	case screenMerge:
		a.rowOffset = clamp(a.rowOffset+delta, 0, max(0, len(a.mergeRows)-a.fileVisibleRows()))
	}
	return a, nil
}

func clamp(value, low, high int) int {
	if high < low {
		return low
	}
	return min(max(value, low), high)
}

func adjustOffset(offset, cursor, visible int) int {
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+visible {
		return cursor - visible + 1
	}
	return max(0, offset)
}

func replaceLines(lines []string, start, end int, replacement []string) []string {
	result := make([]string, 0, len(lines)-(end-start)+len(replacement))
	result = append(result, lines[:start]...)
	result = append(result, replacement...)
	result = append(result, lines[end:]...)
	return result
}

func cleanBase(path string) string { return filepath.Base(filepath.Clean(path)) }

func (a *App) hasUnsavedCompare() bool { return a.docs[0].Dirty || a.docs[1].Dirty }

func (a *App) dirtyMerge() bool { return !a.saved }

func modeForOutput(path string, fallback os.FileMode) os.FileMode {
	if info, err := os.Stat(path); err == nil {
		return info.Mode()
	}
	return fallback
}

func describeBinary(doc core.Document) string {
	return fmt.Sprintf("%s  %d bytes", cleanBase(doc.Path), len(doc.Data))
}
