package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"merger/internal/core"
)

func TestCompareAltArrowsPushInMeldDirection(t *testing.T) {
	a := New(Config{Mode: ModeCompare})
	a.screen = screenCompare
	a.docs = [2]core.Document{
		{Label: "LEFT", Lines: []string{"left"}, EOL: "\n", Mode: 0o644},
		{Label: "RIGHT", Lines: []string{"right"}, EOL: "\n", Mode: 0o644},
	}
	a.rebuildCompare()
	a.pushCompareChange(1)
	if !slices.Equal(a.docs[1].Lines, []string{"left"}) || !a.docs[1].Dirty {
		t.Fatalf("Alt+Right result = %#v, dirty=%t", a.docs[1].Lines, a.docs[1].Dirty)
	}

	a.docs[0].Lines = []string{"new left"}
	a.docs[0].Dirty = false
	a.rebuildCompare()
	a.pushCompareChange(-1)
	if !slices.Equal(a.docs[0].Lines, []string{"left"}) || a.docs[0].Dirty {
		t.Fatalf("Alt+Left result = %#v, dirty=%t", a.docs[0].Lines, a.docs[0].Dirty)
	}
}

func TestComparePushStaysAtAppliedDiffAndMarksTargetAsStaged(t *testing.T) {
	base := make([]string, 30)
	for index := range base {
		base[index] = fmt.Sprintf("line-%02d", index)
	}
	left := slices.Clone(base)
	left = append(left[:5], append([]string{"new-left-line"}, left[5:]...)...)
	left[26] = "later-left-change"

	a := New(Config{Mode: ModeCompare})
	a.screen = screenCompare
	a.height = 10
	a.docs = [2]core.Document{
		{Label: "LEFT", Lines: left, EOL: "\n", Mode: 0o644},
		{Label: "RIGHT", Lines: slices.Clone(base), EOL: "\n", Mode: 0o644},
	}
	a.rebuildCompare()
	if len(a.compareChange) != 2 || a.changeCursor != 0 {
		t.Fatalf("initial changes = %d, cursor=%d", len(a.compareChange), a.changeCursor)
	}
	anchor := a.compareChange[0].RowStart
	a.pushCompareChange(1)

	if a.changeCursor != -1 {
		t.Fatalf("push selected the next diff: cursor=%d", a.changeCursor)
	}
	if a.compareAnchorRow != anchor || a.rowOffset != anchor {
		t.Fatalf("push moved away from applied diff: anchor=%d rowOffset=%d, want %d", a.compareAnchorRow, a.rowOffset, anchor)
	}
	if got := a.stagedLineKinds[1][5]; got != core.ChangeAdded {
		t.Fatalf("staged target kind = %v, want added", got)
	}
	if got, want := a.docs[1].Lines[5], "new-left-line"; got != want {
		t.Fatalf("staged target line = %q, want %q", got, want)
	}

	a.moveCompareChange(1)
	if a.changeCursor != 0 || a.compareChange[0].RowStart <= anchor {
		t.Fatalf("Alt+Down did not select the later diff: cursor=%d change=%#v", a.changeCursor, a.compareChange)
	}
}

func TestOverviewSeparatesChangeMapFromScrollbar(t *testing.T) {
	overview := renderOverview([]core.ChangeKind{core.ChangeSame, core.ChangeAdded, core.ChangeModified}, 6, 0, 2)
	if got := lipgloss.Width(overview); got != 3 {
		t.Fatalf("overview width = %d, want 3", got)
	}
	thumbRows := 0
	for number, line := range strings.Split(ansi.Strip(overview), "\n") {
		runes := []rune(line)
		if len(runes) != 3 {
			t.Fatalf("row %d = %q, want three cells", number, line)
		}
		if changeMap := string(runes[:2]); changeMap != "··" && changeMap != "██" {
			t.Fatalf("row %d change map = %q", number, changeMap)
		}
		switch runes[2] {
		case '█':
			thumbRows++
		case '│':
		default:
			t.Fatalf("row %d scrollbar cell = %q", number, string(runes[2]))
		}
	}
	if thumbRows == 0 {
		t.Fatal("the scrollbar never marked the visible part of the file")
	}
	scrolled := ansi.Strip(renderOverview(make([]core.ChangeKind, 60), 6, 54, 6))
	if first := strings.Split(scrolled, "\n")[0]; strings.HasSuffix(first, "█") {
		t.Fatalf("a scrolled view still marked the first row as visible: %q", first)
	}
}

func TestSuccessfulCompareSaveClearsStagedHighlight(t *testing.T) {
	a := New(Config{Mode: ModeCompare})
	a.screen = screenCompare
	a.docs = [2]core.Document{
		{Label: "LEFT", Lines: []string{"new", "same"}, EOL: "\n", Mode: 0o644},
		{Label: "RIGHT", Lines: []string{"same"}, EOL: "\n", Mode: 0o644},
	}
	a.rebuildCompare()
	a.pushCompareChange(1)
	if a.stagedLineKinds[1][0] != core.ChangeAdded {
		t.Fatal("push did not create a staged target highlight")
	}
	updated, _ := a.Update(compareSavedMsg{})
	a = updated.(*App)
	for side := range 2 {
		for _, kind := range a.stagedLineKinds[side] {
			if kind != core.ChangeSame {
				t.Fatalf("side %d retained staged kind %v after save", side, kind)
			}
		}
		if len(a.stagedDeleteAt[side]) != 0 {
			t.Fatalf("side %d retained staged deletions after save: %#v", side, a.stagedDeleteAt[side])
		}
	}
}

func TestPickerChoosesTwoFilesAndStartsComparison(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "a-left.txt")
	right := filepath.Join(root, "b-right.txt")
	if err := os.WriteFile(left, []byte("left\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(right, []byte("right\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := core.ListDirectory(root, false)
	if err != nil {
		t.Fatal(err)
	}
	a := New(Config{Mode: ModePicker, BrowseRoot: root})
	updated, _ := a.Update(directoryPickerMessage(root, entries))
	a = updated.(*App)
	updated, cmd := a.updatePicker(tea.KeyMsg{Type: tea.KeyEnter})
	a = updated.(*App)
	if cmd != nil || a.browserFirst != left {
		t.Fatalf("first selection = %q, command = %v", a.browserFirst, cmd)
	}
	a.browserCursor = 1
	updated, cmd = a.updatePicker(tea.KeyMsg{Type: tea.KeyEnter})
	a = updated.(*App)
	if cmd == nil || a.config.Mode != ModeCompare || a.screen != screenLoading {
		t.Fatalf("comparison was not started: mode=%v screen=%v cmd=%v", a.config.Mode, a.screen, cmd)
	}
	updated, _ = a.Update(cmd())
	a = updated.(*App)
	if a.screen != screenCompare || a.docs[0].Path != left || a.docs[1].Path != right {
		t.Fatalf("loaded comparison = screen %v, paths %q / %q", a.screen, a.docs[0].Path, a.docs[1].Path)
	}
	updated, cmd = a.updateCompare(tea.KeyMsg{Type: tea.KeyEsc})
	a = updated.(*App)
	if cmd != nil || a.screen != screenPicker {
		t.Fatalf("Esc did not return to picker: screen=%v cmd=%v", a.screen, cmd)
	}
}

func TestPickerChoosesTwoDirectoriesAndStartsLazyComparison(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "a-left")
	right := filepath.Join(root, "b-right")
	for _, path := range []string{left, right} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := New(Config{Mode: ModePicker, BrowseRoot: root})
	a.screen = screenPicker
	updated, _ := a.chooseBrowserPath(left)
	a = updated.(*App)
	updated, cmd := a.chooseBrowserPath(right)
	a = updated.(*App)
	if cmd == nil || a.config.Mode != ModeDirectory {
		t.Fatalf("directory comparison was not started: mode=%v cmd=%v", a.config.Mode, cmd)
	}
	updated, _ = a.Update(cmd())
	a = updated.(*App)
	if a.screen != screenDirectory || a.dirLeft != left || a.dirRight != right {
		t.Fatalf("loaded directories = screen %v, paths %q / %q", a.screen, a.dirLeft, a.dirRight)
	}
	updated, cmd = a.updateDirectory(tea.KeyMsg{Type: tea.KeyEsc})
	a = updated.(*App)
	if cmd != nil || a.screen != screenPicker {
		t.Fatalf("Esc did not return to picker: screen=%v cmd=%v", a.screen, cmd)
	}
}

func TestPickerRejectsMixedPathTypesAndKeepsFirstSelection(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	directory := filepath.Join(root, "directory")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	a := New(Config{Mode: ModePicker, BrowseRoot: root})
	a.screen = screenPicker
	updated, _ := a.chooseBrowserPath(file)
	a = updated.(*App)
	updated, cmd := a.chooseBrowserPath(directory)
	a = updated.(*App)
	if cmd != nil || a.browserFirst != file || !a.statusError || a.screen != screenPicker {
		t.Fatalf("mixed selection = first %q, error=%t, screen=%v, cmd=%v", a.browserFirst, a.statusError, a.screen, cmd)
	}
}

func TestPickerEnterBackspaceAndSpaceSelectsHighlightedDirectory(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := core.ListDirectory(root, false)
	if err != nil {
		t.Fatal(err)
	}
	a := New(Config{Mode: ModePicker, BrowseRoot: root})
	updated, _ := a.Update(directoryPickerMessage(root, entries))
	a = updated.(*App)
	updated, cmd := a.updatePicker(tea.KeyMsg{Type: tea.KeyEnter})
	a = updated.(*App)
	if cmd == nil {
		t.Fatal("Enter on a directory did not start navigation")
	}
	updated, _ = a.Update(cmd())
	a = updated.(*App)
	if a.browserPath != child || a.screen != screenPicker {
		t.Fatalf("opened directory = %q, screen=%v", a.browserPath, a.screen)
	}
	updated, cmd = a.updatePicker(tea.KeyMsg{Type: tea.KeyBackspace})
	a = updated.(*App)
	if cmd == nil {
		t.Fatal("Backspace did not open the parent")
	}
	updated, _ = a.Update(cmd())
	a = updated.(*App)
	if a.browserPath != root {
		t.Fatalf("parent directory = %q, want %q", a.browserPath, root)
	}
	updated, cmd = a.updatePicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	a = updated.(*App)
	if cmd != nil || a.browserFirst != child {
		t.Fatalf("Space selection = %q, command=%v", a.browserFirst, cmd)
	}
}

func TestDirectoryComparisonTypingJumpsToEntry(t *testing.T) {
	a := New(Config{Mode: ModeDirectory})
	a.screen = screenDirectory
	a.width, a.height = 100, 24
	a.dirEntries = []core.DirectoryEntry{
		{Name: "Alpha", Status: core.EntryFolder},
		{Name: "Beta-file.txt", Status: core.EntryDifferent},
		{Name: "Documents", Status: core.EntryFolder},
	}

	updated, cmd := a.updateDirectory(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	a = updated.(*App)
	if cmd == nil || a.dirQuery != "b" || a.dirCursor != 1 {
		t.Fatalf("first typed directory match = query %q, cursor %d, cmd=%v", a.dirQuery, a.dirCursor, cmd)
	}
	firstID := a.dirQueryID
	updated, cmd = a.updateDirectory(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	a = updated.(*App)
	if cmd == nil || a.dirQuery != "bE" || a.dirCursor != 1 {
		t.Fatalf("second typed directory match = query %q, cursor %d, cmd=%v", a.dirQuery, a.dirCursor, cmd)
	}
	updated, _ = a.Update(directoryQueryExpiredMsg{id: firstID})
	a = updated.(*App)
	if a.dirQuery != "bE" {
		t.Fatalf("stale timeout cleared directory query: %q", a.dirQuery)
	}
	updated, _ = a.Update(directoryQueryExpiredMsg{id: a.dirQueryID})
	a = updated.(*App)
	if a.dirQuery != "" {
		t.Fatalf("current timeout did not clear directory query: %q", a.dirQuery)
	}
}

func TestCRLFAndLFAreShownAsPushableDifference(t *testing.T) {
	a := New(Config{Mode: ModeCompare})
	a.screen = screenCompare
	a.height = 20
	a.docs = [2]core.Document{
		{Label: "LEFT", Lines: []string{"same", "text"}, EOL: "\r\n", FinalNewline: true, Mode: 0o644},
		{Label: "RIGHT", Lines: []string{"same", "text"}, EOL: "\n", FinalNewline: true, Mode: 0o644},
	}
	a.rebuildCompare()
	if len(a.compareChange) != 1 || !a.compareChange[0].Metadata {
		t.Fatalf("line ending changes = %#v", a.compareChange)
	}
	if len(a.compareRows) != 4 || !a.compareRows[3].Metadata || !strings.Contains(a.compareRows[3].Left, "CRLF") || !strings.Contains(a.compareRows[3].Right, "LF") {
		t.Fatalf("line ending row = %#v", a.compareRows)
	}
	a.pushCompareChange(1)
	if a.docs[1].EOL != "\r\n" || !a.stagedFormat[1] || a.changeCursor != -1 {
		t.Fatalf("staged format = EOL %q, staged=%t, cursor=%d", a.docs[1].EOL, a.stagedFormat[1], a.changeCursor)
	}
	updated, _ := a.Update(compareSavedMsg{})
	a = updated.(*App)
	if a.stagedFormat[1] || len(a.compareChange) != 0 || len(a.compareRows) != 3 {
		t.Fatalf("saved format state = staged=%t changes=%#v rows=%#v", a.stagedFormat[1], a.compareChange, a.compareRows)
	}
}

func TestFinalNewlineDifferenceIsShown(t *testing.T) {
	a := New(Config{Mode: ModeCompare})
	a.screen = screenCompare
	a.docs = [2]core.Document{
		{Label: "LEFT", Lines: []string{"same"}, EOL: "\n", FinalNewline: true},
		{Label: "RIGHT", Lines: []string{"same"}, EOL: "\n", FinalNewline: false},
	}
	a.rebuildCompare()
	if len(a.compareChange) != 1 || !a.compareChange[0].Metadata {
		t.Fatalf("final newline changes = %#v", a.compareChange)
	}
	formatRow := a.compareRows[len(a.compareRows)-1]
	if !strings.Contains(formatRow.Left, "final newline") || !strings.Contains(formatRow.Right, "no final newline") {
		t.Fatalf("final newline row = %#v", formatRow)
	}
}

func TestExactDirectoryFileMatchExplainsMetadataOnlyStatus(t *testing.T) {
	a := New(Config{Mode: ModeDirectory})
	document := core.Document{Path: "/tmp/file", Lines: []string{"same"}, EOL: "\n", Mode: 0o644}
	updated, _ := a.Update(compareLoadedMsg{left: document, right: document, fromDir: true})
	a = updated.(*App)
	if len(a.compareChange) != 0 || !strings.Contains(a.status, "metadata only") {
		t.Fatalf("exact match status = %q, changes=%#v", a.status, a.compareChange)
	}
}

func TestPickerTypingJumpsToCaseInsensitivePrefix(t *testing.T) {
	a := New(Config{Mode: ModePicker, BrowseRoot: "/tmp"})
	a.screen = screenPicker
	a.width, a.height = 80, 24
	a.browserEntries = []core.BrowserEntry{
		{Name: "Alpha", Path: "/tmp/Alpha", Kind: core.EntryDirectory},
		{Name: "Beta-file.txt", Path: "/tmp/Beta-file.txt", Kind: core.EntryFile},
		{Name: "Documents", Path: "/tmp/Documents", Kind: core.EntryDirectory},
	}

	updated, cmd := a.updatePicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	a = updated.(*App)
	if cmd == nil || a.browserQuery != "b" || a.browserCursor != 1 {
		t.Fatalf("first typed match = query %q, cursor %d, cmd=%v", a.browserQuery, a.browserCursor, cmd)
	}
	firstID := a.browserQueryID
	updated, cmd = a.updatePicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	a = updated.(*App)
	if cmd == nil || a.browserQuery != "bE" || a.browserCursor != 1 {
		t.Fatalf("second typed match = query %q, cursor %d, cmd=%v", a.browserQuery, a.browserCursor, cmd)
	}

	updated, _ = a.Update(browserQueryExpiredMsg{id: firstID})
	a = updated.(*App)
	if a.browserQuery != "bE" {
		t.Fatalf("stale timeout cleared current query: %q", a.browserQuery)
	}
	updated, _ = a.Update(browserQueryExpiredMsg{id: a.browserQueryID})
	a = updated.(*App)
	if a.browserQuery != "" {
		t.Fatalf("current timeout did not clear query: %q", a.browserQuery)
	}
}

func TestPickerBackspaceEditsQueryBeforeOpeningParent(t *testing.T) {
	a := New(Config{Mode: ModePicker, BrowseRoot: "/tmp/child"})
	a.screen = screenPicker
	a.browserQuery = "be"
	a.browserEntries = []core.BrowserEntry{{Name: "beta", Path: "/tmp/child/beta", Kind: core.EntryFile}}

	updated, cmd := a.updatePicker(tea.KeyMsg{Type: tea.KeyBackspace})
	a = updated.(*App)
	if cmd == nil || a.browserQuery != "b" || a.browserPath != "/tmp/child" {
		t.Fatalf("first Backspace = query %q, path %q, cmd=%v", a.browserQuery, a.browserPath, cmd)
	}
	updated, cmd = a.updatePicker(tea.KeyMsg{Type: tea.KeyBackspace})
	a = updated.(*App)
	if cmd != nil || a.browserQuery != "" || a.browserPath != "/tmp/child" {
		t.Fatalf("second Backspace = query %q, path %q, cmd=%v", a.browserQuery, a.browserPath, cmd)
	}
	updated, cmd = a.updatePicker(tea.KeyMsg{Type: tea.KeyBackspace})
	a = updated.(*App)
	if cmd == nil {
		t.Fatal("Backspace with an empty query did not open the parent")
	}
}

func directoryPickerMessage(path string, entries []core.BrowserEntry) browserLoadedMsg {
	return browserLoadedMsg{path: path, entries: entries}
}

func TestMergeAltArrowsPushOuterPaneIntoResult(t *testing.T) {
	a := New(Config{Mode: ModeMerge, Output: "result"})
	a.screen = screenMerge
	a.mine = core.Document{Lines: []string{"mine"}, EOL: "\n"}
	a.base = core.Document{Lines: []string{"base"}, EOL: "\n"}
	a.theirs = core.Document{Lines: []string{"theirs"}, EOL: "\n"}
	a.initializeMerge()

	updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	a = updated.(*App)
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"mine"}) {
		t.Fatalf("Alt+Right merged = %#v, want mine", got)
	}
	updated, _ = a.updateMerge(tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	a = updated.(*App)
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"theirs"}) {
		t.Fatalf("Alt+Left merged = %#v, want theirs", got)
	}
}

func TestMergeRefusesToSaveUnresolvedConflict(t *testing.T) {
	a := New(Config{Mode: ModeMerge, Output: filepath.Join(t.TempDir(), "result")})
	a.mine = core.Document{Lines: []string{"mine"}, EOL: "\n", Mode: 0o644}
	a.base = core.Document{Lines: []string{"base"}, EOL: "\n", Mode: 0o644}
	a.theirs = core.Document{Lines: []string{"theirs"}, EOL: "\n", Mode: 0o644}
	a.initializeMerge()
	if cmd := a.saveMergeCmd(); cmd != nil {
		t.Fatal("unresolved merge returned a save command")
	}
	if !a.statusError {
		t.Fatal("unresolved save did not expose an error status")
	}
}

func TestResolvedMergeSavesAtomically(t *testing.T) {
	output := filepath.Join(t.TempDir(), "result.txt")
	a := New(Config{Mode: ModeMerge, Output: output})
	a.mine = core.Document{Lines: []string{"mine"}, EOL: "\n", FinalNewline: true, Mode: 0o640}
	a.base = core.Document{Lines: []string{"base"}, EOL: "\n", FinalNewline: true, Mode: 0o640}
	a.theirs = core.Document{Lines: []string{"theirs"}, EOL: "\n", FinalNewline: true, Mode: 0o640}
	a.initializeMerge()
	a.resolveMerge(core.ResolutionTheirs)
	cmd := a.saveMergeCmd()
	if cmd == nil {
		t.Fatal("resolved merge did not return a save command")
	}
	updated, _ := a.Update(cmd())
	a = updated.(*App)
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "theirs\n" {
		t.Fatalf("saved result = %q, err=%v", data, err)
	}
	if !a.saved {
		t.Fatal("successful save was not recorded")
	}
}

func TestDirectoryEnterAndBackspaceLoadOneLevel(t *testing.T) {
	left, right := filepath.Join(t.TempDir(), "left"), filepath.Join(t.TempDir(), "right")
	for _, root := range []string{left, right} {
		if err := os.MkdirAll(filepath.Join(root, "child"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := New(Config{Mode: ModeDirectory, Left: left, Right: right})
	updated, _ := a.Update(directoryLoadedMsg{
		left: left, right: right,
		entries: []core.DirectoryEntry{{Name: "child", Left: core.EntrySide{Exists: true, Kind: core.EntryDirectory}, Right: core.EntrySide{Exists: true, Kind: core.EntryDirectory}, Status: core.EntryFolder}},
	})
	a = updated.(*App)
	updated, cmd := a.updateDirectory(tea.KeyMsg{Type: tea.KeyEnter})
	a = updated.(*App)
	if cmd == nil || filepath.Base(a.dirLeft) != "child" || len(a.dirHistory) != 1 {
		t.Fatalf("enter state: left=%s history=%d cmd=%v", a.dirLeft, len(a.dirHistory), cmd)
	}
	updated, _ = a.Update(cmd())
	a = updated.(*App)
	updated, cmd = a.updateDirectory(tea.KeyMsg{Type: tea.KeyBackspace})
	a = updated.(*App)
	if cmd == nil || a.dirLeft != left || a.dirRight != right {
		t.Fatalf("backspace state: left=%s right=%s", a.dirLeft, a.dirRight)
	}
}

func TestFooterANSITruncationStaysWithinTerminal(t *testing.T) {
	a := New(Config{})
	a.width = 35
	a.setStatus("a deliberately long status message", true)
	footer := a.footer(hint("Alt+Left", "a long operation"), hint("Ctrl+S", "save"))
	if width := lipgloss.Width(footer); width > a.width {
		t.Fatalf("footer width = %d, terminal width = %d", width, a.width)
	}
}

func TestComparisonViewsStayWithinTerminalWidth(t *testing.T) {
	assertWidth := func(t *testing.T, rendered string, width int) {
		t.Helper()
		for number, line := range strings.Split(rendered, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("line %d width = %d, terminal = %d\n%s", number+1, got, width, line)
			}
		}
	}

	compare := New(Config{Mode: ModeCompare})
	compare.width, compare.height, compare.screen = 100, 28, screenCompare
	compare.docs = [2]core.Document{
		{Path: "/tmp/a/very-long-left-name.txt", Lines: []string{"same", "left"}, EOL: "\n"},
		{Path: "/tmp/b/very-long-right-name.txt", Lines: []string{"same", "right"}, EOL: "\n"},
	}
	compare.rebuildCompare()
	assertWidth(t, compare.View(), compare.width)

	merge := New(Config{Mode: ModeMerge, Output: "/tmp/result"})
	merge.width, merge.height, merge.screen = 120, 30, screenMerge
	merge.mine = core.Document{Lines: []string{"same", "mine"}, EOL: "\n"}
	merge.base = core.Document{Lines: []string{"same", "base"}, EOL: "\n"}
	merge.theirs = core.Document{Lines: []string{"same", "theirs"}, EOL: "\n"}
	merge.initializeMerge()
	assertWidth(t, merge.View(), merge.width)

	directory := New(Config{Mode: ModeDirectory, Left: "/tmp/left", Right: "/tmp/right"})
	directory.width, directory.height, directory.screen = 80, 24, screenDirectory
	directory.dirLeft, directory.dirRight = "/tmp/left", "/tmp/right"
	directory.dirEntries = []core.DirectoryEntry{{
		Name: "a-long-directory-name", Status: core.EntryFolder,
		Left:  core.EntrySide{Exists: true, Kind: core.EntryDirectory},
		Right: core.EntrySide{Exists: true, Kind: core.EntryDirectory},
	}}
	assertWidth(t, directory.View(), directory.width)

	picker := New(Config{Mode: ModePicker, BrowseRoot: "/tmp"})
	picker.width, picker.height, picker.screen = 80, 24, screenPicker
	picker.browserPath = "/tmp/a/deliberately/long/browser/path"
	picker.browserFirst = "/tmp/a/deliberately/long/first/selection.txt"
	picker.browserEntries = []core.BrowserEntry{{Name: "a-very-long-directory-name", Path: "/tmp/a-very-long-directory-name", Kind: core.EntryDirectory}}
	assertWidth(t, picker.View(), picker.width)
}
