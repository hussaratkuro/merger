package ui

import "testing"

func TestInlineChangeRangesExcludeCommonText(t *testing.T) {
	left, right := inlineChangeRanges("hello old world", "hello new world", true)
	if left != (visualRange{start: 6, end: 9}) || right != (visualRange{start: 6, end: 9}) {
		t.Fatalf("ranges = %#v / %#v", left, right)
	}
}

func TestSyntaxDecorationsRecognizeKeywordsStringsAndComments(t *testing.T) {
	line := `if value == "ok" { // note`
	classes := syntaxDecorations(line, "main.go", true)
	if len(classes) != len([]rune(line)) || classes[0] != syntaxKeyword || classes[12] != syntaxString || classes[len(classes)-1] != syntaxComment {
		t.Fatalf("unexpected syntax classes: %#v", classes)
	}
	if decorations := syntaxDecorations(line, "README.md", true); decorations != nil {
		t.Fatalf("unsupported file was highlighted: %#v", decorations)
	}
}
