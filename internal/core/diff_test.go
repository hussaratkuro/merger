package core

import (
	"math/rand"
	"slices"
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

func TestDiffEditsLargeInputUsesBoundedFallback(t *testing.T) {
	base := make([]string, 2500)
	other := make([]string, 2500)
	for i := range base {
		base[i], other[i] = "same", "same"
	}
	other[1200] = "changed"
	edits := DiffEdits(base, other)
	if len(edits) != 1 || edits[0].BaseStart != 1200 || edits[0].BaseEnd != 1201 {
		t.Fatalf("large fallback edit = %#v", edits)
	}
	if got := applyEdits(base, edits); !slices.Equal(got, other) {
		t.Fatal("large fallback did not reproduce target")
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
