package ui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha, shared visually with eod and svntui.
const (
	moBase     = "#1e1e2e"
	moSurface0 = "#313244"
	moSurface1 = "#45475a"
	moOverlay0 = "#6c7086"
	moOverlay1 = "#7f849c"
	moSubtext0 = "#a6adc8"
	moText     = "#cdd6f4"
	moLavender = "#b4befe"
	moBlue     = "#89b4fa"
	moSapphire = "#74c7ec"
	moTeal     = "#94e2d5"
	moGreen    = "#a6e3a1"
	moYellow   = "#f9e2af"
	moPeach    = "#fab387"
	moRed      = "#f38ba8"
	moMauve    = "#cba6f7"
)

var (
	styleLogo     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(moMauve))
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(moLavender))
	styleText     = lipgloss.NewStyle().Foreground(lipgloss.Color(moText))
	styleMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color(moOverlay0))
	styleSubtle   = lipgloss.NewStyle().Foreground(lipgloss.Color(moOverlay1))
	styleAdded    = lipgloss.NewStyle().Foreground(lipgloss.Color(moGreen))
	styleDeleted  = lipgloss.NewStyle().Foreground(lipgloss.Color(moRed))
	styleModified = lipgloss.NewStyle().Foreground(lipgloss.Color(moYellow))
	styleConflict = lipgloss.NewStyle().Foreground(lipgloss.Color(moRed)).Bold(true)
	styleSuccess  = lipgloss.NewStyle().Foreground(lipgloss.Color(moGreen)).Bold(true)
	styleWarning  = lipgloss.NewStyle().Foreground(lipgloss.Color(moPeach)).Bold(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color(moRed)).Bold(true)
	styleKey      = lipgloss.NewStyle().Foreground(lipgloss.Color(moMauve)).Bold(true)
	styleFocus    = lipgloss.NewStyle().Background(lipgloss.Color(moSurface1)).Foreground(lipgloss.Color(moLavender)).Bold(true)
	styleCursor   = lipgloss.NewStyle().Background(lipgloss.Color(moLavender)).Foreground(lipgloss.Color(moBase)).Bold(true)
	styleSelected = lipgloss.NewStyle().Background(lipgloss.Color(moSurface0))
	styleBorder   = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(moSurface1))
)

func hint(key, description string) string {
	return styleKey.Render(key) + styleMuted.Render(":"+description)
}
