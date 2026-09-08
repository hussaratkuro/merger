package core

import (
	"slices"
	"sort"
)

type ChangeKind uint8

const (
	ChangeSame ChangeKind = iota
	ChangeAdded
	ChangeDeleted
	ChangeModified
	ChangeConflict
)

type Edit struct {
	BaseStart int
	BaseEnd   int
	NewLines  []string
}

const maxLCSCells = 4_000_000

// DiffEdits returns the replacements that turn base into other. Ordinary files
// use an exact LCS. Inputs large enough to make a quadratic allocation unsafe
// are represented by one middle replacement after trimming their common ends.
func DiffEdits(base, other []string) []Edit {
	if slices.Equal(base, other) {
		return nil
	}
	n, m := len(base), len(other)
	if n > 0 && m > 0 && n+1 > maxLCSCells/(m+1) {
		return middleReplacement(base, other)
	}

	cols := m + 1
	dp := make([]int32, (n+1)*cols)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			idx := i*cols + j
			if base[i] == other[j] {
				dp[idx] = dp[(i+1)*cols+j+1] + 1
			} else {
				down, right := dp[(i+1)*cols+j], dp[i*cols+j+1]
				if down >= right {
					dp[idx] = down
				} else {
					dp[idx] = right
				}
			}
		}
	}

	var edits []Edit
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && base[i] == other[j] {
			i++
			j++
			continue
		}
		start := i
		var added []string
		for i < n || j < m {
			if i < n && j < m && base[i] == other[j] {
				break
			}
			if j < m && (i == n || dp[i*cols+j+1] > dp[(i+1)*cols+j]) {
				added = append(added, other[j])
				j++
			} else if i < n {
				i++
			} else {
				added = append(added, other[j])
				j++
			}
		}
		edits = append(edits, Edit{BaseStart: start, BaseEnd: i, NewLines: slices.Clone(added)})
	}
	return edits
}

func middleReplacement(base, other []string) []Edit {
	prefix := 0
	for prefix < len(base) && prefix < len(other) && base[prefix] == other[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(base)-prefix && suffix < len(other)-prefix &&
		base[len(base)-1-suffix] == other[len(other)-1-suffix] {
		suffix++
	}
	return []Edit{{
		BaseStart: prefix,
		BaseEnd:   len(base) - suffix,
		NewLines:  slices.Clone(other[prefix : len(other)-suffix]),
	}}
}

type CompareRow struct {
	Left, Right     string
	LeftNo, RightNo int
	Kind            ChangeKind
	Change          int
	Metadata        bool
}

type CompareChange struct {
	RowStart, RowEnd     int
	LeftStart, LeftEnd   int
	RightStart, RightEnd int
	Metadata             bool
}

// AlignDocuments creates line-numbered side-by-side rows and source ranges for
// Meld-compatible push-left/push-right operations.
func AlignDocuments(left, right []string) ([]CompareRow, []CompareChange) {
	edits := DiffEdits(left, right)
	rows := make([]CompareRow, 0, max(len(left), len(right)))
	changes := make([]CompareChange, 0, len(edits))
	leftPos, rightPos := 0, 0

	appendSameThrough := func(end int) {
		for leftPos < end {
			rows = append(rows, CompareRow{
				Left: left[leftPos], Right: right[rightPos],
				LeftNo: leftPos + 1, RightNo: rightPos + 1,
				Kind: ChangeSame, Change: -1,
			})
			leftPos++
			rightPos++
		}
	}

	for _, edit := range edits {
		appendSameThrough(edit.BaseStart)
		changeIndex := len(changes)
		change := CompareChange{
			RowStart: len(rows), LeftStart: edit.BaseStart, LeftEnd: edit.BaseEnd,
			RightStart: rightPos, RightEnd: rightPos + len(edit.NewLines),
		}
		removed := left[edit.BaseStart:edit.BaseEnd]
		for i := range max(len(removed), len(edit.NewLines)) {
			row := CompareRow{Change: changeIndex}
			switch {
			case i < len(removed) && i < len(edit.NewLines):
				row.Left, row.Right = removed[i], edit.NewLines[i]
				row.LeftNo, row.RightNo = edit.BaseStart+i+1, rightPos+i+1
				row.Kind = ChangeModified
			case i < len(removed):
				row.Left, row.LeftNo, row.Kind = removed[i], edit.BaseStart+i+1, ChangeDeleted
			default:
				row.Right, row.RightNo, row.Kind = edit.NewLines[i], rightPos+i+1, ChangeAdded
			}
			rows = append(rows, row)
		}
		leftPos = edit.BaseEnd
		rightPos += len(edit.NewLines)
		change.RowEnd = len(rows)
		changes = append(changes, change)
	}
	appendSameThrough(len(left))
	return rows, changes
}

type Resolution uint8

const (
	ResolutionAuto Resolution = iota
	ResolutionUnresolved
	ResolutionMine
	ResolutionTheirs
	ResolutionBase
	ResolutionBoth
	ResolutionManual
)

func (r Resolution) String() string {
	switch r {
	case ResolutionAuto:
		return "auto"
	case ResolutionUnresolved:
		return "unresolved"
	case ResolutionMine:
		return "mine"
	case ResolutionTheirs:
		return "theirs"
	case ResolutionBase:
		return "base"
	case ResolutionBoth:
		return "both"
	case ResolutionManual:
		return "manual"
	default:
		return "unknown"
	}
}

type MergeChunk struct {
	BaseStart, BaseEnd int
	Base, Mine, Theirs []string
	Result             []string
	Conflict           bool
	Resolution         Resolution
}

type MergePlan struct {
	Base   []string
	Chunks []MergeChunk
}

type taggedEdit struct {
	Edit
	side byte
}

// BuildMergePlan performs a line-based three-way merge. Disjoint changes and
// identical edits resolve automatically; overlapping differing edits remain
// explicit conflict chunks until the user chooses or edits their result.
func BuildMergePlan(base, mine, theirs []string) MergePlan {
	var tagged []taggedEdit
	for _, edit := range DiffEdits(base, mine) {
		tagged = append(tagged, taggedEdit{Edit: edit, side: 'm'})
	}
	for _, edit := range DiffEdits(base, theirs) {
		tagged = append(tagged, taggedEdit{Edit: edit, side: 't'})
	}
	sort.SliceStable(tagged, func(i, j int) bool {
		if tagged[i].BaseStart != tagged[j].BaseStart {
			return tagged[i].BaseStart < tagged[j].BaseStart
		}
		if tagged[i].BaseEnd != tagged[j].BaseEnd {
			return tagged[i].BaseEnd < tagged[j].BaseEnd
		}
		return tagged[i].side < tagged[j].side
	})

	plan := MergePlan{Base: slices.Clone(base)}
	for i := 0; i < len(tagged); {
		start, end := tagged[i].BaseStart, tagged[i].BaseEnd
		j := i + 1
		for j < len(tagged) && overlapsRange(tagged[j].Edit, start, end) {
			start = min(start, tagged[j].BaseStart)
			end = max(end, tagged[j].BaseEnd)
			j++
		}
		group := tagged[i:j]
		baseLines := slices.Clone(base[start:end])
		mineLines := applyTaggedEdits(base, start, end, group, 'm')
		theirLines := applyTaggedEdits(base, start, end, group, 't')
		chunk := MergeChunk{
			BaseStart: start, BaseEnd: end, Base: baseLines,
			Mine: mineLines, Theirs: theirLines, Resolution: ResolutionAuto,
		}
		switch {
		case slices.Equal(mineLines, theirLines):
			chunk.Result = slices.Clone(mineLines)
		case slices.Equal(mineLines, baseLines):
			chunk.Result = slices.Clone(theirLines)
		case slices.Equal(theirLines, baseLines):
			chunk.Result = slices.Clone(mineLines)
		default:
			chunk.Conflict = true
			chunk.Resolution = ResolutionUnresolved
			chunk.Result = conflictMarkerLines(mineLines, baseLines, theirLines)
		}
		plan.Chunks = append(plan.Chunks, chunk)
		i = j
	}
	return plan
}

func overlapsRange(edit Edit, start, end int) bool {
	if start == end && edit.BaseStart == edit.BaseEnd {
		return edit.BaseStart == start
	}
	return edit.BaseStart < end && start < edit.BaseEnd
}

func applyTaggedEdits(base []string, start, end int, edits []taggedEdit, side byte) []string {
	var sideEdits []Edit
	for _, tagged := range edits {
		if tagged.side == side {
			sideEdits = append(sideEdits, tagged.Edit)
		}
	}
	if len(sideEdits) == 0 {
		return slices.Clone(base[start:end])
	}
	var result []string
	position := start
	for _, edit := range sideEdits {
		if edit.BaseStart > position {
			result = append(result, base[position:edit.BaseStart]...)
		}
		result = append(result, edit.NewLines...)
		position = max(position, edit.BaseEnd)
	}
	if position < end {
		result = append(result, base[position:end]...)
	}
	return result
}

func conflictMarkerLines(mine, base, theirs []string) []string {
	result := []string{"<<<<<<< MINE"}
	result = append(result, mine...)
	result = append(result, "||||||| BASE")
	result = append(result, base...)
	result = append(result, "=======")
	result = append(result, theirs...)
	result = append(result, ">>>>>>> THEIRS")
	return result
}

func (p *MergePlan) Resolve(index int, resolution Resolution, manual []string) bool {
	if index < 0 || index >= len(p.Chunks) {
		return false
	}
	chunk := &p.Chunks[index]
	switch resolution {
	case ResolutionMine:
		chunk.Result = slices.Clone(chunk.Mine)
	case ResolutionTheirs:
		chunk.Result = slices.Clone(chunk.Theirs)
	case ResolutionBase:
		chunk.Result = slices.Clone(chunk.Base)
	case ResolutionBoth:
		chunk.Result = append(slices.Clone(chunk.Mine), chunk.Theirs...)
	case ResolutionManual:
		chunk.Result = slices.Clone(manual)
	case ResolutionUnresolved:
		chunk.Result = conflictMarkerLines(chunk.Mine, chunk.Base, chunk.Theirs)
	default:
		return false
	}
	chunk.Resolution = resolution
	return true
}

func (p MergePlan) ResultLines() []string {
	var result []string
	position := 0
	for _, chunk := range p.Chunks {
		result = append(result, p.Base[position:chunk.BaseStart]...)
		result = append(result, chunk.Result...)
		position = chunk.BaseEnd
	}
	result = append(result, p.Base[position:]...)
	return result
}

func (p MergePlan) UnresolvedCount() int {
	count := 0
	for _, chunk := range p.Chunks {
		if chunk.Resolution == ResolutionUnresolved {
			count++
		}
	}
	return count
}

type MergeRow struct {
	Mine, Result, Theirs      string
	MineNo, ResultNo, TheirNo int
	Kind                      ChangeKind
	Chunk                     int
}

func (p MergePlan) Rows() ([]MergeRow, []CompareChange) {
	var rows []MergeRow
	changes := make([]CompareChange, 0, len(p.Chunks))
	basePos, mineNo, resultNo, theirNo := 0, 1, 1, 1
	for chunkIndex, chunk := range p.Chunks {
		for basePos < chunk.BaseStart {
			line := p.Base[basePos]
			rows = append(rows, MergeRow{Mine: line, Result: line, Theirs: line,
				MineNo: mineNo, ResultNo: resultNo, TheirNo: theirNo, Kind: ChangeSame, Chunk: -1})
			basePos++
			mineNo++
			resultNo++
			theirNo++
		}
		change := CompareChange{RowStart: len(rows)}
		kind := ChangeModified
		if chunk.Resolution == ResolutionUnresolved {
			kind = ChangeConflict
		}
		for i := range max(len(chunk.Mine), len(chunk.Result), len(chunk.Theirs)) {
			row := MergeRow{Kind: kind, Chunk: chunkIndex}
			if i < len(chunk.Mine) {
				row.Mine, row.MineNo = chunk.Mine[i], mineNo
				mineNo++
			}
			if i < len(chunk.Result) {
				row.Result, row.ResultNo = chunk.Result[i], resultNo
				resultNo++
			}
			if i < len(chunk.Theirs) {
				row.Theirs, row.TheirNo = chunk.Theirs[i], theirNo
				theirNo++
			}
			rows = append(rows, row)
		}
		change.RowEnd = len(rows)
		changes = append(changes, change)
		basePos = chunk.BaseEnd
	}
	for basePos < len(p.Base) {
		line := p.Base[basePos]
		rows = append(rows, MergeRow{Mine: line, Result: line, Theirs: line,
			MineNo: mineNo, ResultNo: resultNo, TheirNo: theirNo, Kind: ChangeSame, Chunk: -1})
		basePos++
		mineNo++
		resultNo++
		theirNo++
	}
	return rows, changes
}
