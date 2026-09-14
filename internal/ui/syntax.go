package ui

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

func inlineChangeRanges(left, right string, enabled bool) (visualRange, visualRange) {
	if !enabled {
		return visualRange{}, visualRange{}
	}
	leftRunes, rightRunes := []rune(expandTabs(left)), []rune(expandTabs(right))
	prefix := 0
	for prefix < len(leftRunes) && prefix < len(rightRunes) && leftRunes[prefix] == rightRunes[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(leftRunes)-prefix && suffix < len(rightRunes)-prefix &&
		leftRunes[len(leftRunes)-1-suffix] == rightRunes[len(rightRunes)-1-suffix] {
		suffix++
	}
	toVisual := func(value []rune, index int) int { return lipgloss.Width(string(value[:index])) }
	return visualRange{start: toVisual(leftRunes, prefix), end: toVisual(leftRunes, len(leftRunes)-suffix)},
		visualRange{start: toVisual(rightRunes, prefix), end: toVisual(rightRunes, len(rightRunes)-suffix)}
}

func syntaxDecorations(line, filename string, enabled bool) []syntaxClass {
	if !enabled || !syntaxFile(filename) {
		return nil
	}
	runes := []rune(expandTabs(line))
	classes := make([]syntaxClass, len(runes))
	extension := strings.ToLower(filepath.Ext(filename))
	hashComment := extension == ".py" || extension == ".sh" || extension == ".bash" || extension == ".zsh" ||
		extension == ".yaml" || extension == ".yml" || extension == ".toml" || extension == ".conf"
	for index := 0; index < len(runes); {
		if (hashComment && runes[index] == '#') || (index+1 < len(runes) && runes[index] == '/' && runes[index+1] == '/') {
			for mark := index; mark < len(runes); mark++ {
				classes[mark] = syntaxComment
			}
			break
		}
		if runes[index] == '\'' || runes[index] == '"' || runes[index] == '`' {
			quote := runes[index]
			start := index
			index++
			for index < len(runes) {
				if runes[index] == '\\' {
					index += min(2, len(runes)-index)
					continue
				}
				index++
				if runes[index-1] == quote {
					break
				}
			}
			for mark := start; mark < index; mark++ {
				classes[mark] = syntaxString
			}
			continue
		}
		if unicode.IsDigit(runes[index]) {
			start := index
			for index < len(runes) && (unicode.IsDigit(runes[index]) || strings.ContainsRune("._xXaAbBcCdDeEfF", runes[index])) {
				index++
			}
			for mark := start; mark < index; mark++ {
				classes[mark] = syntaxNumber
			}
			continue
		}
		if unicode.IsLetter(runes[index]) || runes[index] == '_' {
			start := index
			for index < len(runes) && (unicode.IsLetter(runes[index]) || unicode.IsDigit(runes[index]) || runes[index] == '_') {
				index++
			}
			if syntaxKeywords[strings.ToLower(string(runes[start:index]))] {
				for mark := start; mark < index; mark++ {
					classes[mark] = syntaxKeyword
				}
			}
			continue
		}
		index++
	}
	return classes
}

func syntaxFile(filename string) bool {
	extension := strings.ToLower(filepath.Ext(filename))
	switch extension {
	case ".go", ".rs", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".c", ".h", ".cpp", ".hpp",
		".cs", ".php", ".rb", ".sh", ".bash", ".zsh", ".json", ".yaml", ".yml", ".toml", ".xml", ".html", ".css", ".sql", ".conf":
		return true
	}
	return false
}

var syntaxKeywords = map[string]bool{
	"break": true, "case": true, "class": true, "const": true, "continue": true, "default": true,
	"defer": true, "do": true, "else": true, "enum": true, "false": true, "for": true, "func": true,
	"function": true, "if": true, "import": true, "interface": true, "let": true, "map": true, "new": true,
	"nil": true, "null": true, "package": true, "private": true, "public": true, "range": true, "return": true,
	"select": true, "static": true, "struct": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "type": true, "var": true, "while": true,
}
