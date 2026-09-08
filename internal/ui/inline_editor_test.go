package ui

import (
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

func newEditableComparison(left, right []string) *App {
	a := New(Config{Mode: ModeCompare})
	a.screen = screenCompare
	a.width, a.height = 100, 24
	a.docs = [2]core.Document{
		{Path: "/tmp/left", Label: "LEFT", Lines: slices.Clone(left), EOL: "\n", Mode: 0o644},
		{Path: "/tmp/right", Label: "RIGHT", Lines: slices.Clone(right), EOL: "\n", Mode: 0o644},
	}
	a.rebuildCompare()
	a.resetInlineEditing()
	return a
}

func TestCompareTypingEditsFocusedPaneImmediately(t *testing.T) {
	a := newEditableComparison([]string{"alpha"}, []string{"alpha"})
	a.inlineCursors[0] = inlineCursor{Line: 0, Col: 2}

	updated, cmd := a.updateCompare(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	a = updated.(*App)
	if cmd != nil || a.screen != screenCompare {
		t.Fatalf("typing left comparison screen: screen=%v cmd=%v", a.screen, cmd)
	}
	if got := a.docs[0].Lines; !slices.Equal(got, []string{"alXpha"}) {
		t.Fatalf("typed document = %#v", got)
	}
	if !a.docs[0].Dirty || a.inlineCursors[0] != (inlineCursor{Line: 0, Col: 3}) {
		t.Fatalf("typing state: dirty=%t cursor=%#v", a.docs[0].Dirty, a.inlineCursors[0])
	}
}

func TestCompareEnterBackspaceUndoAndRedo(t *testing.T) {
	a := newEditableComparison([]string{"abcd"}, []string{"abcd"})
	a.inlineCursors[0] = inlineCursor{Line: 0, Col: 2}

	a.updateCompare(tea.KeyMsg{Type: tea.KeyEnter})
	if got := a.docs[0].Lines; !slices.Equal(got, []string{"ab", "cd"}) {
		t.Fatalf("Enter result = %#v", got)
	}
	if a.inlineCursors[0] != (inlineCursor{Line: 1, Col: 0}) {
		t.Fatalf("Enter cursor = %#v", a.inlineCursors[0])
	}

	a.updateCompare(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := a.docs[0].Lines; !slices.Equal(got, []string{"abcd"}) {
		t.Fatalf("Backspace result = %#v", got)
	}
	if a.docs[0].Dirty {
		t.Fatal("editing back to the baseline remained dirty")
	}

	a.updateCompare(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if got := a.docs[0].Lines; !slices.Equal(got, []string{"ab", "cd"}) {
		t.Fatalf("undo result = %#v", got)
	}
	a.updateCompare(tea.KeyMsg{Type: tea.KeyCtrlY})
	if got := a.docs[0].Lines; !slices.Equal(got, []string{"abcd"}) {
		t.Fatalf("redo result = %#v", got)
	}
}

func TestComparePushUndoRedoAndNewEditClearsRedo(t *testing.T) {
	a := newEditableComparison([]string{"left"}, []string{"right"})
	a.pushCompareChange(1)
	if got := a.docs[1].Lines; !slices.Equal(got, []string{"left"}) || !a.docs[1].Dirty {
		t.Fatalf("push result = %#v dirty=%t", got, a.docs[1].Dirty)
	}

	a.undoCompare()
	if got := a.docs[1].Lines; !slices.Equal(got, []string{"right"}) || a.docs[1].Dirty {
		t.Fatalf("undo push result = %#v dirty=%t", got, a.docs[1].Dirty)
	}
	if len(a.compareChange) != 1 {
		t.Fatalf("undo did not restore the difference: %#v", a.compareChange)
	}

	a.redoCompare()
	if got := a.docs[1].Lines; !slices.Equal(got, []string{"left"}) || !a.docs[1].Dirty {
		t.Fatalf("redo push result = %#v dirty=%t", got, a.docs[1].Dirty)
	}
	if len(a.stagedLineKinds[1]) != 1 || a.stagedLineKinds[1][0] == core.ChangeSame {
		t.Fatalf("redo did not restore staged highlighting: %#v", a.stagedLineKinds[1])
	}

	a.undoCompare()
	a.fileFocus = 1
	a.inlineCursors[1] = inlineCursor{Line: 0, Col: 0}
	a.insertInlineText("!")
	if len(a.compareRedo) != 0 {
		t.Fatalf("new edit retained %d redo snapshot(s)", len(a.compareRedo))
	}
}

func TestInlineEditSavesWithOriginalLineEndingAndCanUndoAfterSave(t *testing.T) {
	root := t.TempDir()
	leftPath := filepath.Join(root, "left.txt")
	rightPath := filepath.Join(root, "right.txt")
	for _, path := range []string{leftPath, rightPath} {
		if err := os.WriteFile(path, []byte("alpha\r\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	left, err := core.ReadDocument(leftPath, "LEFT")
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.ReadDocument(rightPath, "RIGHT")
	if err != nil {
		t.Fatal(err)
	}
	a := New(Config{Mode: ModeCompare, Left: leftPath, Right: rightPath})
	a.screen = screenCompare
	a.width, a.height = 100, 24
	a.docs = [2]core.Document{left, right}
	a.rebuildCompare()
	a.resetInlineEditing()
	a.inlineCursors[0] = inlineCursor{Line: 0, Col: 5}
	a.insertInlineText("!")

	cmd := a.saveCompareCmd()
	updated, _ := a.Update(cmd())
	a = updated.(*App)
	data, err := os.ReadFile(leftPath)
	if err != nil || string(data) != "alpha!\r\n" {
		t.Fatalf("saved inline edit = %q, err=%v", data, err)
	}
	if a.docs[0].Dirty {
		t.Fatal("successful save retained the dirty state")
	}

	a.undoCompare()
	if got := a.docs[0].Text(); got != "alpha\r\n" || !a.docs[0].Dirty {
		t.Fatalf("undo after save = %q, dirty=%t", got, a.docs[0].Dirty)
	}
	a.redoCompare()
	if got := a.docs[0].Text(); got != "alpha!\r\n" || a.docs[0].Dirty {
		t.Fatalf("redo after save = %q, dirty=%t", got, a.docs[0].Dirty)
	}
}

func TestCompareMouseClickPlacesCursorAtExactPaneRowAndColumn(t *testing.T) {
	a := newEditableComparison([]string{"zero", "left-row"}, []string{"zero", "right-row"})
	contentWidth := max(20, a.width-6)
	numWidth := 1
	separatorWidth := lipgloss.Width(" │ Δ │ ")
	cellSpace := max(4, contentWidth-separatorWidth-2*(numWidth+3))
	leftWidth := cellSpace / 2
	rightTextStart := 1 + numWidth + 3 + leftWidth + separatorWidth + numWidth + 3

	updated, _ := a.updateMouse(tea.MouseMsg{
		X: rightTextStart + 3, Y: 4,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	a = updated.(*App)
	if a.fileFocus != 1 || a.inlineCursors[1] != (inlineCursor{Line: 1, Col: 3}) {
		t.Fatalf("mouse cursor = side %d, cursor %#v", a.fileFocus, a.inlineCursors[1])
	}
	a.updateCompare(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if got := a.docs[1].Lines[1]; got != "rig!ht-row" {
		t.Fatalf("typed at clicked position = %q", got)
	}
}

func TestEditableCellRendersVisibleCursorWithoutChangingWidth(t *testing.T) {
	rendered := renderEditableCell("abc", 8, styleText, cellCursor{Show: true, RuneColumn: 1})
	if !strings.Contains(rendered, styleCursor.Render("b")) {
		t.Fatalf("rendered cell has no cursor styling: %q", rendered)
	}
	if got := lipgloss.Width(rendered); got != 8 {
		t.Fatalf("rendered cursor cell width = %d", got)
	}
	if got := ansi.Strip(rendered); got != "abc     " {
		t.Fatalf("rendered cursor cell text = %q", got)
	}
}

func TestEmptyAndFinalNewlineFilesKeepAnEditableCursorRow(t *testing.T) {
	empty := newEditableComparison(nil, nil)
	if len(empty.compareRows) != 1 {
		t.Fatalf("empty comparison rows = %#v", empty.compareRows)
	}
	if cursor := empty.cursorForCompareRow(0, 0, empty.compareRows[0].LeftNo, false); !cursor.Show {
		t.Fatal("empty file has no visible editable cursor row")
	}
	empty.insertInlineText("x")
	if got := empty.docs[0].Lines; !slices.Equal(got, []string{"x"}) {
		t.Fatalf("typing into empty file = %#v", got)
	}

	final := newEditableComparison([]string{"last"}, []string{"last"})
	for side := range 2 {
		final.docs[side].FinalNewline = true
	}
	final.compareBaselineSet = false
	final.rebuildCompare()
	final.resetInlineEditing()
	final.moveInlineCursor("ctrl+end")
	lastRow := final.compareRows[len(final.compareRows)-1]
	if lastRow.LeftNo != 2 || !final.cursorForCompareRow(0, len(final.compareRows)-1, lastRow.LeftNo, false).Show {
		t.Fatalf("final-newline cursor row = %#v cursor=%#v", lastRow, final.inlineCursors[0])
	}
	final.insertInlineText("!")
	if got := final.docs[0].Lines; !slices.Equal(got, []string{"last", "!"}) || final.docs[0].FinalNewline {
		t.Fatalf("typing after final newline = lines %#v, final=%t", got, final.docs[0].FinalNewline)
	}
}

func TestEmptySideShowsOnlyOneCursorAndClickMovesItsRow(t *testing.T) {
	a := newEditableComparison(nil, []string{"one", "two", "three"})
	shown := 0
	for index, row := range a.compareRows {
		if a.cursorForCompareRow(0, index, row.LeftNo, row.Metadata).Show {
			shown++
		}
	}
	if shown != 1 {
		t.Fatalf("empty pane showed %d cursors", shown)
	}

	a.clickCompare(tea.MouseEvent{X: 6, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if a.compareAnchorRow != 2 || !a.cursorForCompareRow(0, 2, a.compareRows[2].LeftNo, false).Show {
		t.Fatalf("clicked empty-pane cursor row = %d", a.compareAnchorRow)
	}
}

func TestRuneColumnAtVisualHandlesTabsAndWideRunes(t *testing.T) {
	value := "a\t界z"
	tests := []struct {
		visual int
		want   int
	}{
		{0, 0}, {1, 1}, {3, 1}, {4, 2}, {5, 2}, {6, 3}, {7, 4},
	}
	for _, test := range tests {
		if got := runeColumnAtVisual(value, test.visual); got != test.want {
			t.Errorf("visual column %d = rune %d, want %d", test.visual, got, test.want)
		}
	}
}
