package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"merger/internal/core"
)

func conflictingMergeApp(t *testing.T) *App {
	t.Helper()
	a := New(Config{Mode: ModeMerge, Output: "result"})
	a.width, a.height, a.screen = 120, 30, screenMerge
	a.mine = core.Document{Lines: []string{"one", "MINE", "three"}, EOL: "\n"}
	a.base = core.Document{Lines: []string{"one", "two", "three"}, EOL: "\n"}
	a.theirs = core.Document{Lines: []string{"one", "THEIRS", "three"}, EOL: "\n"}
	a.initializeMerge()
	if a.mergePlan.UnresolvedCount() != 1 {
		t.Fatalf("setup produced %d unresolved chunks, want 1", a.mergePlan.UnresolvedCount())
	}
	return a
}

func typeMerge(t *testing.T, a *App, text string) *App {
	t.Helper()
	for _, r := range text {
		updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		a = updated.(*App)
	}
	return a
}

func TestMergeResultPaneTypesIntoTheResolvedChunk(t *testing.T) {
	a := conflictingMergeApp(t)
	updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	a = updated.(*App)
	if a.mergeCursor.Line != 1 {
		t.Fatalf("cursor after Alt+Right = %#v, want the chunk's first result line", a.mergeCursor)
	}
	a = typeMerge(t, a, "X")
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"one", "XMINE", "three"}) {
		t.Fatalf("result = %#v", got)
	}
	if a.mergeCursor != (inlineCursor{Line: 1, Col: 1}) {
		t.Fatalf("cursor = %#v, want line 1 column 1", a.mergeCursor)
	}
	if a.mergePlan.UnresolvedCount() != 0 {
		t.Fatal("a typed edit left the chunk unresolved")
	}
	if a.saved {
		t.Fatal("a typed edit did not mark the merge dirty")
	}
}

func TestMergeEditingConflictMarkersStaysUnresolved(t *testing.T) {
	a := conflictingMergeApp(t)
	// Line 2 is the MINE body between the still-present conflict markers.
	a.mergeCursor = inlineCursor{Line: 2}
	a = typeMerge(t, a, "X")
	if got := a.mergePlan.ResultLines()[2]; got != "XMINE" {
		t.Fatalf("edited line = %q", got)
	}
	if a.mergePlan.UnresolvedCount() != 1 {
		t.Fatal("a chunk still holding conflict markers was treated as resolved")
	}
	if cmd := a.saveMergeCmd(); cmd != nil {
		t.Fatal("a chunk still holding conflict markers returned a save command")
	}
}

func TestMergeResultPaneEditsUnchangedContext(t *testing.T) {
	a := conflictingMergeApp(t)
	a.mergeCursor = inlineCursor{Line: 0, Col: 3}
	a = typeMerge(t, a, "!")
	if got := a.mergePlan.ResultLines()[0]; got != "one!" {
		t.Fatalf("context line = %q, want %q", got, "one!")
	}
	if a.mergePlan.UnresolvedCount() != 1 {
		t.Fatal("editing unchanged context resolved the conflict")
	}
	if a.changeCursor != -1 {
		t.Fatalf("changeCursor = %d, want -1 while the cursor sits in context", a.changeCursor)
	}
}

func TestMergeEnterSplitsContextAndKeepsChunkAligned(t *testing.T) {
	a := conflictingMergeApp(t)
	a.mergeCursor = inlineCursor{Line: 0, Col: 1}
	updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyEnter})
	a = updated.(*App)
	result := a.mergePlan.ResultLines()
	if !slices.Equal(result[:2], []string{"o", "ne"}) {
		t.Fatalf("split context = %#v", result[:2])
	}
	if a.mergeCursor != (inlineCursor{Line: 1}) {
		t.Fatalf("cursor = %#v, want the start of the new line", a.mergeCursor)
	}
	if got := result[2]; !strings.HasPrefix(got, "<<<<<<<") {
		t.Fatalf("the conflict chunk moved out of place: line 2 = %q", got)
	}
	if last := result[len(result)-1]; last != "three" {
		t.Fatalf("trailing context = %q", last)
	}
}

func TestMergeBackspaceJoinsAcrossAChunkBoundary(t *testing.T) {
	a := conflictingMergeApp(t)
	updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	a = updated.(*App)
	a.mergeCursor = inlineCursor{Line: 1}
	updated, _ = a.updateMerge(tea.KeyMsg{Type: tea.KeyBackspace})
	a = updated.(*App)
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"oneMINE", "three"}) {
		t.Fatalf("result = %#v", got)
	}
	if a.mergeCursor != (inlineCursor{Line: 0, Col: 3}) {
		t.Fatalf("cursor = %#v, want the join position", a.mergeCursor)
	}
}

func TestMergeUndoRestoresTypedEdits(t *testing.T) {
	a := conflictingMergeApp(t)
	updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	a = updated.(*App)
	a = typeMerge(t, a, "XY")
	updated, _ = a.updateMerge(tea.KeyMsg{Type: tea.KeyCtrlZ})
	a = updated.(*App)
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"one", "XMINE", "three"}) {
		t.Fatalf("after one undo = %#v", got)
	}
	updated, _ = a.updateMerge(tea.KeyMsg{Type: tea.KeyCtrlZ})
	a = updated.(*App)
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"one", "MINE", "three"}) {
		t.Fatalf("after two undos = %#v", got)
	}
	updated, _ = a.updateMerge(tea.KeyMsg{Type: tea.KeyCtrlY})
	a = updated.(*App)
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"one", "XMINE", "three"}) {
		t.Fatalf("after redo = %#v", got)
	}
}

func TestMergeUndoRestoresASideChoice(t *testing.T) {
	a := conflictingMergeApp(t)
	updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	a = updated.(*App)
	updated, _ = a.updateMerge(tea.KeyMsg{Type: tea.KeyCtrlZ})
	a = updated.(*App)
	if a.mergePlan.UnresolvedCount() != 1 {
		t.Fatal("undo did not restore the unresolved conflict")
	}
}

func TestMergeRendersTheCursorOnExactlyOneResultRow(t *testing.T) {
	a := conflictingMergeApp(t)
	a.mergeCursor = inlineCursor{Line: 2, Col: 1}
	shown := 0
	for rowIndex, row := range a.mergeRows {
		cursor := a.cursorForMergeRow(rowIndex, row.ResultNo)
		if !cursor.Show {
			continue
		}
		shown++
		if row.ResultNo != 3 {
			t.Fatalf("cursor shown on result line %d, want 3", row.ResultNo)
		}
		if cursor.RuneColumn != 1 {
			t.Fatalf("cursor column = %d, want 1", cursor.RuneColumn)
		}
	}
	if shown != 1 {
		t.Fatalf("cursor shown on %d rows, want exactly 1", shown)
	}
}

func TestMergeClickPlacesTheResultCursor(t *testing.T) {
	a := conflictingMergeApp(t)
	layout := a.mergeLayout()
	a.clickMerge(tea.MouseEvent{X: layout.resultStart + 2, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if a.mergeCursor != (inlineCursor{Line: 0, Col: 2}) {
		t.Fatalf("cursor after a click on the first result row = %#v", a.mergeCursor)
	}
	a = typeMerge(t, a, "Z")
	if got := a.mergePlan.ResultLines()[0]; got != "onZe" {
		t.Fatalf("typed after a click = %q", got)
	}
}

func TestMergeClickOnAnOuterPaneSelectsTheChange(t *testing.T) {
	a := conflictingMergeApp(t)
	a.changeCursor = -1
	layout := a.mergeLayout()
	// Row 1 of the merge view is the first line of the conflict chunk.
	a.clickMerge(tea.MouseEvent{X: layout.mineStart + 1, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if a.changeCursor != 0 {
		t.Fatalf("changeCursor = %d, want the clicked chunk", a.changeCursor)
	}
	if a.mergeCursor.Line != a.mergePlan.ChunkResultStart(0) {
		t.Fatalf("cursor = %#v, want the chunk start", a.mergeCursor)
	}
}

func TestMergeAltLettersResolveWithoutConsumingTypedText(t *testing.T) {
	a := conflictingMergeApp(t)
	updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}, Alt: true})
	a = updated.(*App)
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"one", "two", "three"}) {
		t.Fatalf("Alt+B = %#v, want the base text", got)
	}
	a = typeMerge(t, a, "b")
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"one", "btwo", "three"}) {
		t.Fatalf("plain b = %#v, want it typed into the result", got)
	}
}

func TestMergeCursorMovesWithArrowKeys(t *testing.T) {
	a := conflictingMergeApp(t)
	updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	a = updated.(*App)
	a.mergeCursor = inlineCursor{}
	for _, key := range []tea.KeyType{tea.KeyDown, tea.KeyRight, tea.KeyRight} {
		updated, _ = a.updateMerge(tea.KeyMsg{Type: key})
		a = updated.(*App)
	}
	if a.mergeCursor != (inlineCursor{Line: 1, Col: 2}) {
		t.Fatalf("cursor = %#v, want line 1 column 2", a.mergeCursor)
	}
	updated, _ = a.updateMerge(tea.KeyMsg{Type: tea.KeyEnd})
	a = updated.(*App)
	if a.mergeCursor.Col != 4 {
		t.Fatalf("End column = %d, want the end of %q", a.mergeCursor.Col, "MINE")
	}
}

func TestResolvedMergeAcceptsTypedResolutionOnSave(t *testing.T) {
	a := conflictingMergeApp(t)
	deleteLine := func(line int) {
		t.Helper()
		a.mergeCursor = inlineCursor{Line: line}
		for range a.mergeLineLength(line) + 1 {
			updated, _ := a.updateMerge(tea.KeyMsg{Type: tea.KeyDelete})
			a = updated.(*App)
		}
	}
	// Remove "<<<<<<< MINE", then the five marker lines that follow "MINE".
	deleteLine(1)
	for range 5 {
		deleteLine(2)
	}
	if got := a.mergePlan.ResultLines(); !slices.Equal(got, []string{"one", "MINE", "three"}) {
		t.Fatalf("hand-resolved result = %#v", got)
	}
	if a.mergePlan.UnresolvedCount() != 0 {
		t.Fatal("removing every conflict marker did not resolve the chunk")
	}
	if cmd := a.saveMergeCmd(); cmd == nil {
		t.Fatal("a hand-resolved merge refused to save")
	}
}
