package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"merger/internal/ui"
)

func main() {
	config, err := ui.ParseArgs(os.Args[1:])
	if err != nil {
		if ui.IsHelp(err) {
			fmt.Print(ui.Usage)
			return
		}
		fmt.Fprintln(os.Stderr, "merger:", err)
		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, ui.Usage)
		os.Exit(2)
	}

	app := ui.New(config)
	program := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := program.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "merger:", err)
		os.Exit(1)
	}
	finalApp, ok := finalModel.(*ui.App)
	if !ok {
		fmt.Fprintln(os.Stderr, "merger: unexpected final application state")
		os.Exit(1)
	}
	if finalApp.FatalError() != nil {
		fmt.Fprintln(os.Stderr, "merger:", finalApp.FatalError())
		os.Exit(1)
	}
	// In three-way mode, an unsaved exit is a deliberate cancellation. This
	// distinct status lets svntui avoid marking the SVN conflict as resolved.
	if config.Mode == ui.ModeMerge && !finalApp.SuccessfulOutput() {
		os.Exit(2)
	}
}
