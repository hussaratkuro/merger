package ui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type commandItem struct {
	label string
	key   tea.KeyMsg
	hint  string
}

type commandPalette struct {
	open   bool
	query  string
	cursor int
	items  []commandItem
}

func (a *App) openCommandPalette() {
	items := []commandItem{
		command("Open help", "F1", tea.KeyMsg{Type: tea.KeyF1}),
		command("Return or quit", "Esc", tea.KeyMsg{Type: tea.KeyEsc}),
	}
	switch a.screen {
	case screenPicker:
		items = append([]commandItem{
			command("Open or select highlighted path", "Enter", tea.KeyMsg{Type: tea.KeyEnter}),
			command("Select highlighted path", "Space", runeCommand(' ')),
			command("Open parent directory", "Backspace", tea.KeyMsg{Type: tea.KeyBackspace}),
			command("Toggle hidden files", ".", runeCommand('.')),
		}, items...)
	case screenDirectory:
		items = append([]commandItem{
			command("Open highlighted difference", "Enter", tea.KeyMsg{Type: tea.KeyEnter}),
			command("Previous difference", "Up", tea.KeyMsg{Type: tea.KeyUp}),
			command("Next difference", "Down", tea.KeyMsg{Type: tea.KeyDown}),
			command("Switch focused side", "Tab", tea.KeyMsg{Type: tea.KeyTab}),
			command("Copy highlighted entry left", "Alt+Left", altCommand(tea.KeyLeft)),
			command("Copy highlighted entry right", "Alt+Right", altCommand(tea.KeyRight)),
			command("Toggle matching entries", "Ctrl+S", tea.KeyMsg{Type: tea.KeyCtrlS}),
			command("Refresh directory comparison", "F5", tea.KeyMsg{Type: tea.KeyF5}),
		}, items...)
	case screenCompare:
		items = append([]commandItem{
			command("Find text", "Ctrl+F", tea.KeyMsg{Type: tea.KeyCtrlF}),
			command("Previous change", "Alt+Up", altCommand(tea.KeyUp)),
			command("Next change", "Alt+Down", altCommand(tea.KeyDown)),
			command("Copy change left", "Alt+Left", altCommand(tea.KeyLeft)),
			command("Copy change right", "Alt+Right", altCommand(tea.KeyRight)),
			command("Switch focused file", "Tab", tea.KeyMsg{Type: tea.KeyTab}),
			command("Toggle ignored whitespace", "Alt+W", altRuneCommand('w')),
			command("Toggle ignored line endings", "Alt+E", altRuneCommand('e')),
			command("Toggle syntax highlighting", "Alt+S", altRuneCommand('s')),
			command("Reload both files", "F5", tea.KeyMsg{Type: tea.KeyF5}),
		}, items...)
		if !a.config.ReadOnly {
			items = append(items, command("Save changed files", "Ctrl+S", tea.KeyMsg{Type: tea.KeyCtrlS}))
		}
	case screenMerge:
		items = append([]commandItem{
			command("Previous conflict", "Alt+Up", altCommand(tea.KeyUp)),
			command("Next conflict", "Alt+Down", altCommand(tea.KeyDown)),
			command("Use mine", "Alt+Right", altCommand(tea.KeyRight)),
			command("Use theirs", "Alt+Left", altCommand(tea.KeyLeft)),
			command("Use base", "Alt+B", altRuneCommand('b')),
			command("Use both", "Alt+A", altRuneCommand('a')),
			command("Restore conflict markers", "Alt+U", altRuneCommand('u')),
			command("Save merge result", "Ctrl+S", tea.KeyMsg{Type: tea.KeyCtrlS}),
		}, items...)
	}
	a.commands = commandPalette{open: true, items: items}
}

func command(label, hint string, key tea.KeyMsg) commandItem {
	return commandItem{label: label, hint: hint, key: key}
}

func runeCommand(character rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}}
}

func altRuneCommand(character rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}, Alt: true}
}

func altCommand(key tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: key, Alt: true} }

func (a *App) filteredCommands() []commandItem {
	var result []commandItem
	for _, item := range a.commands.items {
		if fuzzyCommandMatch(item.label+" "+item.hint, a.commands.query) {
			result = append(result, item)
		}
	}
	return result
}

func fuzzyCommandMatch(label, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	label = strings.ToLower(label)
	position := 0
	for _, character := range query {
		found := strings.IndexRune(label[position:], character)
		if found < 0 {
			return false
		}
		position += found + 1
	}
	return true
}

func (a *App) updateCommandPalette(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := a.filteredCommands()
	switch key.String() {
	case "esc", "ctrl+p", "ctrl+shift+p":
		a.commands = commandPalette{}
	case "up", "ctrl+k":
		a.commands.cursor = max(0, a.commands.cursor-1)
	case "down", "ctrl+j", "tab":
		a.commands.cursor = min(max(0, len(items)-1), a.commands.cursor+1)
	case "backspace":
		query := []rune(a.commands.query)
		if len(query) > 0 {
			a.commands.query = string(query[:len(query)-1])
			a.commands.cursor = 0
		}
	case "enter":
		if len(items) == 0 {
			return a, nil
		}
		selected := items[min(a.commands.cursor, len(items)-1)]
		a.commands = commandPalette{}
		switch a.screen {
		case screenPicker:
			return a.updatePicker(selected.key)
		case screenDirectory:
			return a.updateDirectory(selected.key)
		case screenCompare:
			return a.updateCompare(selected.key)
		case screenMerge:
			return a.updateMerge(selected.key)
		case screenHelp:
			a.screen = a.returnScreen
		}
	default:
		if key.Type == tea.KeyRunes && !key.Alt {
			for _, character := range key.Runes {
				if unicode.IsPrint(character) {
					a.commands.query += string(character)
				}
			}
			a.commands.cursor = 0
		}
	}
	return a, nil
}

func (a *App) viewCommandPalette() string {
	width := min(max(62, a.width*3/4), max(20, a.width-4))
	items := a.filteredCommands()
	limit := min(len(items), max(3, a.height-10))
	start := max(0, min(a.commands.cursor-limit/2, len(items)-limit))
	rows := []string{styleTitle.Render("Command palette"), styleMuted.Render("Fuzzy-search actions on the current screen"), "", "> " + a.commands.query + "█", ""}
	if len(items) == 0 {
		rows = append(rows, styleMuted.Render("No matching commands"))
	}
	for index := start; index < start+limit; index++ {
		item := items[index]
		labelWidth := max(8, width-24)
		label := truncateVisual(item.label, labelWidth)
		line := label + strings.Repeat(" ", max(1, labelWidth-lipgloss.Width(label))) + item.hint
		if index == a.commands.cursor {
			line = styleFocus.Width(width - 6).Render(line)
		}
		rows = append(rows, line)
	}
	rows = append(rows, "", styleMuted.Render("Enter runs · ↑/↓ selects · Esc closes"))
	box := styleBorder.Padding(1, 2).Width(width).Render(strings.Join(rows, "\n"))
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, box)
}
