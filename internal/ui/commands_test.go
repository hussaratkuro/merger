package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCommandPaletteFiltersScreenActions(t *testing.T) {
	app := New(Config{Mode: ModeCompare})
	app.screen = screenCompare
	app.openCommandPalette()
	app.commands.query = "syn hi"
	items := app.filteredCommands()
	if len(items) != 1 || items[0].hint != "Alt+S" {
		t.Fatalf("syntax command match = %#v", items)
	}
	updated, _ := app.updateCommandPalette(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(*App).commands.open {
		t.Fatal("Esc did not close command palette")
	}
}
