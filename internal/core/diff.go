package core

import (
	"slices"
	"sort"
	"strings"
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

// DiffEdits returns the replacements that turn base into other. It uses Myers'
// linear-space O(ND) algorithm, so files of any size are diffed exactly: the
// cost grows with the amount of change, not with the square of the file length.
func DiffEdits(base, other []string) []Edit {
	if slices.Equal(base, other) {
		return nil
	}
	ids := make(map[string]int32, len(base)+len(other))
	intern := func(lines []string) []int32 {
		out := make([]int32, len(lines))
		for i, line := range lines {
			id, ok := ids[line]
			if !ok {
				id = int32(len(ids))
				ids[line] = id
			}
			out[i] = id
		}
		return out
	}
	a, b := intern(base), intern(other)
	size := 2*(len(a)+len(b)) + 4
	d := &myersDiff{a: a, b: b, vf: make([]int, size), vb: make([]int, size)}
	d.diff(0, len(a), 0, len(b))

	var edits []Edit
	i, j := 0, 0
	flush := func(baseEnd, otherEnd int) {
		if baseEnd > i || otherEnd > j {
			edits = append(edits, Edit{BaseStart: i, BaseEnd: baseEnd, NewLines: slices.Clone(other[j:otherEnd])})
		}
	}
	for _, m := range d.matches {
		flush(m[0], m[1])
		i, j = m[0]+1, m[1]+1
	}
	flush(len(base), len(other))
	return edits
}

type myersDiff struct {
	a, b    []int32
	vf, vb  []int
	matches [][2]int // (base index, other index) pairs of equal lines, ascending
}

// diff records the matched line pairs of a[a0:a1] and b[b0:b1] in order by
// splitting the problem at its middle snake.
func (d *myersDiff) diff(a0, a1, b0, b1 int) {
	for a0 < a1 && b0 < b1 && d.a[a0] == d.b[b0] {
		d.matches = append(d.matches, [2]int{a0, b0})
		a0++
		b0++
	}
	suffix := 0
	for a0 < a1-suffix && b0 < b1-suffix && d.a[a1-1-suffix] == d.b[b1-1-suffix] {
		suffix++
	}
	a1, b1 = a1-suffix, b1-suffix
	if a0 < a1 && b0 < b1 {
		x, y, u, v, ok := d.middleSnake(a0, a1, b0, b1)
		if ok {
			d.diff(a0, x, b0, y)
			for x < u {
				d.matches = append(d.matches, [2]int{x, y})
				x++
				y++
			}
			d.diff(u, a1, v, b1)
		}
	}
	for k := 0; k < suffix; k++ {
		d.matches = append(d.matches, [2]int{a1 + k, b1 + k})
	}
}

// maxSnakeSteps bounds the edit distance the middle-snake search explores. A
// region needing more edits than this is a wholesale rewrite, which is shown
// as one replacement instead of spending quadratic time on it.
const maxSnakeSteps = 4000

// middleSnake runs Myers' forward and reverse searches until they meet and
// returns the snake (x,y)-(u,v) on a shortest edit path through a[a0:a1] and
// b[b0:b1]. Both inputs must be non-empty and share no common prefix or suffix.
func (d *myersDiff) middleSnake(a0, a1, b0, b1 int) (x, y, u, v int, ok bool) {
	n, m := a1-a0, b1-b0
	delta := n - m
	odd := delta&1 != 0
	limit := min((n+m+1)/2+1, maxSnakeSteps)
	offset := limit + 1
	vf, vb := d.vf[:2*offset+1], d.vb[:2*offset+1]
	vf[offset+1], vb[offset+1] = 0, 0
	for dist := 0; dist <= limit; dist++ {
		for k := -dist; k <= dist; k += 2 {
			var px int
			if k == -dist || (k != dist && vf[offset+k-1] < vf[offset+k+1]) {
				px = vf[offset+k+1]
			} else {
				px = vf[offset+k-1] + 1
			}
			py := px - k
			fx, fy := px, py
			for fx < n && fy < m && d.a[a0+fx] == d.b[b0+fy] {
				fx++
				fy++
			}
			vf[offset+k] = fx
			if rk := delta - k; odd && rk >= -(dist-1) && rk <= dist-1 && fx+vb[offset+rk] >= n {
				return a0 + px, b0 + py, a0 + fx, b0 + fy, true
			}
		}
		for k := -dist; k <= dist; k += 2 {
			var px int
			if k == -dist || (k != dist && vb[offset+k-1] < vb[offset+k+1]) {
				px = vb[offset+k+1]
			} else {
				px = vb[offset+k-1] + 1
			}
			py := px - k
			rx, ry := px, py
			for rx < n && ry < m && d.a[a1-1-rx] == d.b[b1-1-ry] {
				rx++
				ry++
			}
			vb[offset+k] = rx
			if fk := delta - k; !odd && fk >= -dist && fk <= dist && rx+vf[offset+fk] >= n {
				return a0 + n - rx, b0 + m - ry, a0 + n - px, b0 + m - py, true
			}
		}
	}
	return 0, 0, 0, 0, false
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
	return AlignDocumentsBy(left, right, func(line string) string { return line })
}

// AlignDocumentsBy compares normalized line keys while retaining the original
// text in the returned rows. It is used for options such as ignoring whitespace
// without making copied or displayed content lossy.
func AlignDocumentsBy(left, right []string, normalize func(string) string) ([]CompareRow, []CompareChange) {
	leftKeys, rightKeys := make([]string, len(left)), make([]string, len(right))
	for index, line := range left {
		leftKeys[index] = normalize(line)
	}
	for index, line := range right {
		rightKeys[index] = normalize(line)
	}
	edits := DiffEdits(leftKeys, rightKeys)
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
		added := right[rightPos : rightPos+len(edit.NewLines)]
		for i := range max(len(removed), len(edit.NewLines)) {
			row := CompareRow{Change: changeIndex}
			switch {
			case i < len(removed) && i < len(added):
				row.Left, row.Right = removed[i], added[i]
				row.LeftNo, row.RightNo = edit.BaseStart+i+1, rightPos+i+1
				row.Kind = ChangeModified
			case i < len(removed):
				row.Left, row.LeftNo, row.Kind = removed[i], edit.BaseStart+i+1, ChangeDeleted
			default:
				row.Right, row.RightNo, row.Kind = added[i], rightPos+i+1, ChangeAdded
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

// Clone returns a deep copy so the UI can snapshot a plan for undo/redo.
func (p MergePlan) Clone() MergePlan {
	clone := MergePlan{Base: slices.Clone(p.Base), Chunks: slices.Clone(p.Chunks)}
	for i := range clone.Chunks {
		clone.Chunks[i].Base = slices.Clone(clone.Chunks[i].Base)
		clone.Chunks[i].Mine = slices.Clone(clone.Chunks[i].Mine)
		clone.Chunks[i].Theirs = slices.Clone(clone.Chunks[i].Theirs)
		clone.Chunks[i].Result = slices.Clone(clone.Chunks[i].Result)
	}
	return clone
}

// ResultOwner records where one merged-result line is stored so a free cursor
// over the result pane can write back into the owning chunk or base context.
type ResultOwner struct {
	Chunk int // -1 for an unchanged base context line
	Index int // position inside Chunks[Chunk].Result, or inside Base
}

// ResultOwners lists one owner per line of ResultLines, in the same order.
func (p MergePlan) ResultOwners() []ResultOwner {
	owners := make([]ResultOwner, 0, len(p.Base))
	position := 0
	for index, chunk := range p.Chunks {
		for ; position < chunk.BaseStart; position++ {
			owners = append(owners, ResultOwner{Chunk: -1, Index: position})
		}
		for i := range chunk.Result {
			owners = append(owners, ResultOwner{Chunk: index, Index: i})
		}
		position = chunk.BaseEnd
	}
	for ; position < len(p.Base); position++ {
		owners = append(owners, ResultOwner{Chunk: -1, Index: position})
	}
	return owners
}

// ChunkForResultLine reports the chunk owning a merged-result line, or -1 when
// the line is unchanged base context.
func (p MergePlan) ChunkForResultLine(line int) int {
	owners := p.ResultOwners()
	if line < 0 || line >= len(owners) {
		return -1
	}
	return owners[line].Chunk
}

// ChunkResultStart returns the merged-result line where a chunk begins, which
// stays meaningful even when the chunk resolved to no lines at all.
func (p MergePlan) ChunkResultStart(index int) int {
	line, position := 0, 0
	for i, chunk := range p.Chunks {
		line += chunk.BaseStart - position
		if i == index {
			return line
		}
		line += len(chunk.Result)
		position = chunk.BaseEnd
	}
	return line
}

// SetResultLine replaces one merged-result line in place.
func (p *MergePlan) SetResultLine(line int, value string) bool {
	owners := p.ResultOwners()
	if line < 0 || line >= len(owners) {
		return false
	}
	owner := owners[line]
	if owner.Chunk < 0 {
		p.Base[owner.Index] = value
		return true
	}
	p.Chunks[owner.Chunk].Result[owner.Index] = value
	p.markEdited(owner.Chunk)
	return true
}

// InsertResultLineAfter inserts a line directly below a merged-result line,
// keeping it in the same chunk or base context. Pass -1 to insert at the top.
func (p *MergePlan) InsertResultLineAfter(line int, value string) bool {
	owners := p.ResultOwners()
	if line < -1 || line >= len(owners) {
		return false
	}
	if len(owners) == 0 {
		if len(p.Chunks) > 0 {
			last := len(p.Chunks) - 1
			p.Chunks[last].Result = append(p.Chunks[last].Result, value)
			p.markEdited(last)
			return true
		}
		p.Base = append(p.Base, value)
		return true
	}
	if line < 0 {
		return p.insertOwned(owners[0], 0, value)
	}
	return p.insertOwned(owners[line], 1, value)
}

func (p *MergePlan) insertOwned(owner ResultOwner, shift int, value string) bool {
	index := owner.Index + shift
	if owner.Chunk < 0 {
		p.Base = slices.Insert(p.Base, index, value)
		for i := range p.Chunks {
			if p.Chunks[i].BaseStart >= index {
				p.Chunks[i].BaseStart++
				p.Chunks[i].BaseEnd++
			}
		}
		return true
	}
	chunk := &p.Chunks[owner.Chunk]
	chunk.Result = slices.Insert(chunk.Result, index, value)
	p.markEdited(owner.Chunk)
	return true
}

// DeleteResultLine removes one merged-result line from its owner.
func (p *MergePlan) DeleteResultLine(line int) bool {
	owners := p.ResultOwners()
	if line < 0 || line >= len(owners) {
		return false
	}
	owner := owners[line]
	if owner.Chunk < 0 {
		p.Base = slices.Delete(p.Base, owner.Index, owner.Index+1)
		for i := range p.Chunks {
			if p.Chunks[i].BaseStart > owner.Index {
				p.Chunks[i].BaseStart--
				p.Chunks[i].BaseEnd--
			}
		}
		return true
	}
	chunk := &p.Chunks[owner.Chunk]
	chunk.Result = slices.Delete(chunk.Result, owner.Index, owner.Index+1)
	p.markEdited(owner.Chunk)
	return true
}

// markEdited records a typed edit as a manual resolution, but a chunk whose
// text still carries conflict markers stays unresolved so it cannot be saved.
func (p *MergePlan) markEdited(index int) {
	chunk := &p.Chunks[index]
	if HasConflictMarkers(chunk.Result) {
		chunk.Resolution = ResolutionUnresolved
		return
	}
	chunk.Resolution = ResolutionManual
}

// HasConflictMarkers reports whether any line is still a conflict marker.
func HasConflictMarkers(lines []string) bool {
	for _, line := range lines {
		for _, marker := range [...]string{"<<<<<<<", "|||||||", "=======", ">>>>>>>"} {
			if strings.HasPrefix(line, marker) {
				return true
			}
		}
	}
	return false
}
