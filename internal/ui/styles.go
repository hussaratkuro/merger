package ui

import (
	"github.com/charmbracelet/lipgloss"

	"merger/internal/theme"
)

// Catppuccin Mocha fallback, shared visually with eod and svntui.
var (
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
	styleLogo     lipgloss.Style
	styleTitle    lipgloss.Style
	styleText     lipgloss.Style
	styleMuted    lipgloss.Style
	styleSubtle   lipgloss.Style
	styleAdded    lipgloss.Style
	styleDeleted  lipgloss.Style
	styleModified lipgloss.Style
	styleConflict lipgloss.Style
	styleSuccess  lipgloss.Style
	styleWarning  lipgloss.Style
	styleError    lipgloss.Style
	styleKey      lipgloss.Style
	styleFocus    lipgloss.Style
	styleCursor   lipgloss.Style
	styleSelected lipgloss.Style
	// The scrollbar sits in its own column beside the change map so the file
	// position stays legible even where the map is solid with changes.
	styleScrollTrack lipgloss.Style
	styleScrollThumb lipgloss.Style
	styleBorder      lipgloss.Style
)

func init() { applyTheme(theme.Current()) }

func applyTheme(p theme.Palette) {
	diff, appearance := resolveDiffPalette(p)
	moBase, moSurface0, moSurface1 = string(p.Base), string(p.Surface0), string(p.Surface1)
	moOverlay0, moOverlay1 = string(p.Overlay0), string(p.Overlay1)
	moSubtext0, moText = string(p.Subtext0), string(p.Text)
	moLavender, moBlue, moSapphire = string(p.Lavender), string(p.Blue), string(p.Sapphire)
	moTeal, moGreen, moYellow = string(p.Teal), string(diff.added), string(diff.modified)
	moPeach, moRed, moMauve = string(p.Peach), string(diff.deleted), string(p.Mauve)

	styleLogo = lipgloss.NewStyle().Bold(true).Foreground(p.Mauve)
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(p.Lavender)
	styleText = lipgloss.NewStyle().Foreground(p.Text)
	styleMuted = lipgloss.NewStyle().Foreground(p.Overlay0)
	styleSubtle = lipgloss.NewStyle().Foreground(p.Overlay1)
	styleAdded = lipgloss.NewStyle().Foreground(diff.added)
	styleDeleted = lipgloss.NewStyle().Foreground(diff.deleted)
	styleModified = lipgloss.NewStyle().Foreground(diff.modified)
	styleConflict = lipgloss.NewStyle().Foreground(diff.conflict).Bold(true)
	if appearance == diffAppearanceMono {
		styleAdded = styleAdded.Bold(true)
		styleDeleted = styleDeleted.Strikethrough(true)
		styleModified = styleModified.Underline(true)
		styleConflict = styleConflict.Underline(true)
	}
	styleSuccess = lipgloss.NewStyle().Foreground(p.Green).Bold(true)
	styleWarning = lipgloss.NewStyle().Foreground(p.Peach).Bold(true)
	styleError = lipgloss.NewStyle().Foreground(p.Red).Bold(true)
	styleKey = lipgloss.NewStyle().Foreground(p.Mauve).Bold(true)
	styleFocus = lipgloss.NewStyle().Background(p.Surface1).Foreground(p.Lavender).Bold(true)
	styleCursor = lipgloss.NewStyle().Background(p.Mauve).Foreground(p.OnAccent).Bold(true)
	styleSelected = lipgloss.NewStyle().Background(p.Surface0)
	styleScrollTrack = lipgloss.NewStyle().Foreground(p.Surface1)
	styleScrollThumb = lipgloss.NewStyle().Foreground(p.Lavender).Bold(true)
	styleBorder = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(p.Surface1)
}

func hint(key, description string) string {
	return styleKey.Render(key) + styleMuted.Render(":"+description)
}
