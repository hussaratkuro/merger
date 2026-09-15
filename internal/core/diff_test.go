package core

import (
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func applyEdits(base []string, edits []Edit) []string {
	result := slices.Clone(base)
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		updated := make([]string, 0, len(result)-(edit.BaseEnd-edit.BaseStart)+len(edit.NewLines))
		updated = append(updated, result[:edit.BaseStart]...)
		updated = append(updated, edit.NewLines...)
		updated = append(updated, result[edit.BaseEnd:]...)
		result = updated
	}
	return result
}

func TestDiffEditsRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		base, other []string
	}{
		{"same", []string{"a", "b"}, []string{"a", "b"}},
		{"insert beginning", []string{"a", "b"}, []string{"x", "a", "b"}},
		{"insert middle", []string{"a", "b"}, []string{"a", "x", "b"}},
		{"delete", []string{"a", "x", "b"}, []string{"a", "b"}},
		{"replace", []string{"a", "old", "b"}, []string{"a", "new", "b"}},
		{"repeated lines", []string{"a", "x", "x", "b"}, []string{"a", "x", "y", "b"}},
		{"empty base", nil, []string{"a", "b"}},
		{"empty other", []string{"a", "b"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := applyEdits(tc.base, DiffEdits(tc.base, tc.other))
			if !slices.Equal(got, tc.other) {
				t.Fatalf("applied edits = %#v, want %#v", got, tc.other)
			}
		})
	}
}

func TestDiffEditsLargeInputKeepsSeparatedMiddleChanges(t *testing.T) {
	base := make([]string, 2500)
	for i := range base {
		base[i] = "line-" + strconv.Itoa(i)
	}
	other := slices.Clone(base)
	other[1000] = "first change"
	other[1500] = "second change"
	edits := DiffEdits(base, other)
	if len(edits) != 2 || edits[0].BaseStart != 1000 || edits[0].BaseEnd != 1001 ||
		edits[1].BaseStart != 1500 || edits[1].BaseEnd != 1501 {
		t.Fatalf("large middle edits = %#v", edits)
	}
	if got := applyEdits(base, edits); !slices.Equal(got, other) {
		t.Fatal("large exact middle diff did not reproduce target")
	}
}

func TestDiffEditsRandomRoundTrips(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	words := []string{"a", "b", "c", "d", "repeated"}
	for iteration := range 500 {
		base := make([]string, rng.Intn(30))
		other := make([]string, rng.Intn(30))
		for i := range base {
			base[i] = words[rng.Intn(len(words))]
		}
		for i := range other {
			other[i] = words[rng.Intn(len(words))]
		}
		if got := applyEdits(base, DiffEdits(base, other)); !slices.Equal(got, other) {
			t.Fatalf("iteration %d round trip = %#v, want %#v (base %#v)", iteration, got, other, base)
		}
	}
}

func TestAlignDocumentsProvidesPushRanges(t *testing.T) {
	left := []string{"same", "left", "anchor", "removed", "last"}
	right := []string{"same", "right", "anchor", "last"}
	rows, changes := AlignDocuments(left, right)
	if len(rows) != 5 || len(changes) != 2 {
		t.Fatalf("got %d rows and %d changes", len(rows), len(changes))
	}
	first := changes[0]
	if !slices.Equal(left[first.LeftStart:first.LeftEnd], []string{"left"}) ||
		!slices.Equal(right[first.RightStart:first.RightEnd], []string{"right"}) {
		t.Fatalf("first change ranges do not address source lines: %#v", first)
	}
}

func TestAlignDocumentsByIgnoresWhitespaceButKeepsOriginalText(t *testing.T) {
	left := []string{"same", "  indented value"}
	right := []string{"same", "indented    value"}
	rows, changes := AlignDocumentsBy(left, right, func(line string) string {
		return strings.Join(strings.Fields(line), " ")
	})
	if len(changes) != 0 || len(rows) != 2 {
		t.Fatalf("normalized alignment = %d rows, %d changes", len(rows), len(changes))
	}
	if rows[1].Left != left[1] || rows[1].Right != right[1] {
		t.Fatalf("original text was lost: %#v", rows[1])
	}
}

func TestThreeWayMergeAutoMergesDisjointChanges(t *testing.T) {
	base := []string{"a", "b", "c", "d"}
	mine := []string{"mine", "b", "c", "d"}
	theirs := []string{"a", "b", "c", "theirs"}
	plan := BuildMergePlan(base, mine, theirs)
	if plan.UnresolvedCount() != 0 {
		t.Fatalf("unresolved = %d, want 0", plan.UnresolvedCount())
	}
	want := []string{"mine", "b", "c", "theirs"}
	if got := plan.ResultLines(); !slices.Equal(got, want) {
		t.Fatalf("merged = %#v, want %#v", got, want)
	}
}

func TestThreeWayMergeDetectsAndResolvesOverlap(t *testing.T) {
	base := []string{"before", "base", "after"}
	mine := []string{"before", "mine", "after"}
	theirs := []string{"before", "theirs", "after"}
	plan := BuildMergePlan(base, mine, theirs)
	if plan.UnresolvedCount() != 1 || len(plan.Chunks) != 1 {
		t.Fatalf("chunks = %d, unresolved = %d", len(plan.Chunks), plan.UnresolvedCount())
	}
	if !plan.Resolve(0, ResolutionTheirs, nil) {
		t.Fatal("failed to resolve conflict")
	}
	if got, want := plan.ResultLines(), theirs; !slices.Equal(got, want) {
		t.Fatalf("resolved result = %#v, want %#v", got, want)
	}
	if plan.UnresolvedCount() != 0 {
		t.Fatal("resolved chunk remained unresolved")
	}
}

func TestThreeWayMergeHandlesSamePointInsertions(t *testing.T) {
	base := []string{"a"}
	plan := BuildMergePlan(base, []string{"mine", "a"}, []string{"theirs", "a"})
	if plan.UnresolvedCount() != 1 {
		t.Fatalf("same-position differing insertions unresolved = %d, want 1", plan.UnresolvedCount())
	}
	plan.Resolve(0, ResolutionBoth, nil)
	want := []string{"mine", "theirs", "a"}
	if got := plan.ResultLines(); !slices.Equal(got, want) {
		t.Fatalf("both resolution = %#v, want %#v", got, want)
	}
}

func TestMergeRowsCoverWholeFile(t *testing.T) {
	plan := BuildMergePlan(
		[]string{"top", "base", "bottom"},
		[]string{"top", "mine", "bottom"},
		[]string{"top", "theirs", "bottom"},
	)
	rows, changes := plan.Rows()
	if len(changes) != 1 || rows[0].Mine != "top" || rows[len(rows)-1].Mine != "bottom" {
		t.Fatalf("merge rows do not retain full-file context: %#v", rows)
	}
}

func TestThreeWayMergeIdentityInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(84))
	words := []string{"a", "b", "c", "d"}
	for iteration := range 200 {
		base := make([]string, rng.Intn(18))
		variant := make([]string, rng.Intn(18))
		for i := range base {
			base[i] = words[rng.Intn(len(words))]
		}
		for i := range variant {
			variant[i] = words[rng.Intn(len(words))]
		}
		for _, tc := range []struct {
			mine, theirs []string
		}{
			{variant, base},
			{base, variant},
			{variant, variant},
		} {
			plan := BuildMergePlan(base, tc.mine, tc.theirs)
			if plan.UnresolvedCount() != 0 || !slices.Equal(plan.ResultLines(), variant) {
				t.Fatalf("iteration %d identity merge failed: base=%#v variant=%#v result=%#v unresolved=%d",
					iteration, base, variant, plan.ResultLines(), plan.UnresolvedCount())
			}
		}
	}
}

func TestMergePlanEditsRouteIntoTheOwningChunk(t *testing.T) {
	plan := BuildMergePlan(
		[]string{"one", "two", "three"},
		[]string{"one", "MINE", "three"},
		[]string{"one", "THEIRS", "three"},
	)
	if plan.UnresolvedCount() != 1 {
		t.Fatalf("expected one conflict, got %d", plan.UnresolvedCount())
	}
	plan.Resolve(0, ResolutionMine, nil)
	if got := plan.ChunkResultStart(0); got != 1 {
		t.Fatalf("chunk result start = %d, want 1", got)
	}
	if got := plan.ChunkForResultLine(1); got != 0 {
		t.Fatalf("owner of result line 1 = %d, want chunk 0", got)
	}
	if got := plan.ChunkForResultLine(0); got != -1 {
		t.Fatalf("owner of context line 0 = %d, want -1", got)
	}

	plan.SetResultLine(1, "EDITED")
	if got := plan.ResultLines(); !slices.Equal(got, []string{"one", "EDITED", "three"}) {
		t.Fatalf("result = %#v", got)
	}
	if plan.Chunks[0].Resolution != ResolutionManual {
		t.Fatalf("resolution = %v, want manual", plan.Chunks[0].Resolution)
	}
	if !slices.Equal(plan.Chunks[0].Mine, []string{"MINE"}) {
		t.Fatalf("editing the result damaged the MINE side: %#v", plan.Chunks[0].Mine)
	}
}

func TestMergePlanContextEditsKeepChunksAligned(t *testing.T) {
	plan := BuildMergePlan(
		[]string{"one", "two", "three", "four"},
		[]string{"one", "MINE", "three", "four"},
		[]string{"one", "THEIRS", "three", "four"},
	)
	plan.Resolve(0, ResolutionMine, nil)
	// Split the leading context line, then delete the trailing one.
	plan.SetResultLine(0, "o")
	plan.InsertResultLineAfter(0, "ne")
	if got := plan.ResultLines(); !slices.Equal(got, []string{"o", "ne", "MINE", "three", "four"}) {
		t.Fatalf("after the split = %#v", got)
	}
	if got := plan.ChunkForResultLine(2); got != 0 {
		t.Fatalf("the chunk lost its place after a context insert: owner = %d", got)
	}
	plan.DeleteResultLine(4)
	if got := plan.ResultLines(); !slices.Equal(got, []string{"o", "ne", "MINE", "three"}) {
		t.Fatalf("after the delete = %#v", got)
	}
	if got := plan.ChunkForResultLine(2); got != 0 {
		t.Fatalf("the chunk lost its place after a context delete: owner = %d", got)
	}
}

func TestMergePlanKeepsChunksWithConflictMarkersUnresolved(t *testing.T) {
	plan := BuildMergePlan(
		[]string{"one", "two", "three"},
		[]string{"one", "MINE", "three"},
		[]string{"one", "THEIRS", "three"},
	)
	plan.SetResultLine(2, "edited inside the markers")
	if plan.UnresolvedCount() != 1 {
		t.Fatal("an edit that left conflict markers in place was accepted as resolved")
	}
	for plan.ChunkForResultLine(1) == 0 && HasConflictMarkers(plan.Chunks[0].Result) {
		plan.DeleteResultLine(1)
	}
	if plan.UnresolvedCount() != 0 {
		t.Fatalf("removing every marker left %d unresolved", plan.UnresolvedCount())
	}
}

func TestMergePlanCloneIsIndependent(t *testing.T) {
	plan := BuildMergePlan([]string{"a"}, []string{"b"}, []string{"c"})
	snapshot := plan.Clone()
	plan.Resolve(0, ResolutionMine, nil)
	if got := snapshot.ResultLines(); slices.Equal(got, plan.ResultLines()) {
		t.Fatalf("the clone followed the original: %#v", got)
	}
	if snapshot.UnresolvedCount() != 1 {
		t.Fatal("the clone lost the original unresolved state")
	}
}
