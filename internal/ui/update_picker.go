package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"merger/internal/core"
)

func (a *App) browserVisibleRows() int {
	return max(3, a.height-11)
}

func (a *App) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		if a.browserQuery != "" {
			a.clearBrowserQuery()
			return a, nil
		}
		if a.browserFirst != "" {
			a.browserFirst = ""
			a.setStatus("First selection cleared.", false)
			return a, nil
		}
		a.cancelled = false
		return a, tea.Quit
	case "?":
		a.clearBrowserQuery()
		a.returnScreen, a.screen = screenPicker, screenHelp
		return a, nil
	case "up":
		a.clearBrowserQuery()
		a.browserCursor = max(0, a.browserCursor-1)
	case "down":
		a.clearBrowserQuery()
		a.browserCursor = min(max(0, len(a.browserEntries)-1), a.browserCursor+1)
	case "pgup":
		a.clearBrowserQuery()
		a.browserCursor = max(0, a.browserCursor-a.browserVisibleRows())
	case "pgdown":
		a.clearBrowserQuery()
		a.browserCursor = min(max(0, len(a.browserEntries)-1), a.browserCursor+a.browserVisibleRows())
	case "home":
		a.clearBrowserQuery()
		a.browserCursor = 0
	case "end":
		a.clearBrowserQuery()
		a.browserCursor = max(0, len(a.browserEntries)-1)
	case ".":
		a.clearBrowserQuery()
		a.browserShowHidden = !a.browserShowHidden
		a.browserCursor, a.browserOffset = 0, 0
		a.screen, a.loadingLabel = screenLoading, "Refreshing file explorer..."
		return a, loadBrowserCmd(a.browserPath, a.browserShowHidden)
	case "backspace", "ctrl+h":
		if a.browserQuery != "" {
			runes := []rune(a.browserQuery)
			a.browserQuery = string(runes[:len(runes)-1])
			a.browserQueryID++
			if a.browserQuery != "" {
				a.jumpToBrowserMatch()
				return a, a.expireBrowserQueryCmd()
			}
			return a, nil
		}
		parent := filepath.Dir(a.browserPath)
		if parent == a.browserPath {
			a.setStatus("Already at the filesystem root.", false)
			return a, nil
		}
		a.browserCursor, a.browserOffset = 0, 0
		a.screen, a.loadingLabel = screenLoading, "Opening parent directory..."
		return a, loadBrowserCmd(parent, a.browserShowHidden)
	case " ":
		if len(a.browserEntries) > 0 {
			return a.chooseBrowserPath(a.browserEntries[a.browserCursor].Path)
		}
	case "tab", "ctrl+enter":
		if len(a.browserEntries) > 0 {
			return a.chooseBrowserPath(a.browserEntries[a.browserCursor].Path)
		}
	case "enter":
		if len(a.browserEntries) == 0 {
			break
		}
		entry := a.browserEntries[a.browserCursor]
		if entry.Kind == core.EntryDirectory {
			a.clearBrowserQuery()
			a.browserCursor, a.browserOffset = 0, 0
			a.screen, a.loadingLabel = screenLoading, "Opening directory..."
			return a, loadBrowserCmd(entry.Path, a.browserShowHidden)
		}
		if entry.Kind != core.EntryFile {
			a.setStatus("Select a regular file or directory.", true)
			return a, nil
		}
		return a.chooseBrowserPath(entry.Path)
	}
	if isBrowserQueryKey(msg) {
		a.browserQuery += string(msg.Runes)
		a.browserQueryID++
		a.jumpToBrowserMatch()
		return a, a.expireBrowserQueryCmd()
	}
	a.browserCursor = clamp(a.browserCursor, 0, max(0, len(a.browserEntries)-1))
	a.browserOffset = adjustOffset(a.browserOffset, a.browserCursor, a.browserVisibleRows())
	return a, nil
}

func isBrowserQueryKey(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || msg.Alt || len(msg.Runes) == 0 {
		return false
	}
	for _, r := range msg.Runes {
		if !unicode.IsPrint(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func (a *App) jumpToBrowserMatch() {
	query := strings.ToLower(a.browserQuery)
	for index, entry := range a.browserEntries {
		if strings.HasPrefix(strings.ToLower(entry.Name), query) {
			a.browserCursor = index
			a.browserOffset = adjustOffset(a.browserOffset, a.browserCursor, a.browserVisibleRows())
			return
		}
	}
}

func (a *App) clearBrowserQuery() {
	a.browserQuery = ""
	a.browserQueryID++
}

func (a *App) expireBrowserQueryCmd() tea.Cmd {
	id := a.browserQueryID
	return tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg {
		return browserQueryExpiredMsg{id: id}
	})
}

func (a *App) chooseBrowserPath(path string) (tea.Model, tea.Cmd) {
	a.clearBrowserQuery()
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		a.setStatus(fmt.Sprintf("Cannot select %s: %v", path, err), true)
		return a, nil
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		a.setStatus("Select a regular file or directory.", true)
		return a, nil
	}
	if a.browserFirst == "" {
		a.browserFirst = path
		a.setStatus("First selection set. Choose the second path.", false)
		return a, nil
	}

	firstInfo, err := os.Stat(a.browserFirst)
	if err != nil {
		a.browserFirst = ""
		a.setStatus("The first selection is no longer available; select it again.", true)
		return a, nil
	}
	if firstInfo.IsDir() != info.IsDir() {
		a.setStatus("Choose two files or two directories; their types cannot be mixed.", true)
		return a, nil
	}

	left, right := a.browserFirst, path
	a.status, a.statusError = "", false
	a.browserFirst = ""
	if info.IsDir() {
		a.config.Mode = ModeDirectory
		a.config.Left, a.config.Right = left, right
		a.dirLeft, a.dirRight = left, right
		a.directoryFromPicker = true
		a.compareFromPicker = false
		a.dirCursor, a.dirOffset = 0, 0
		a.screen, a.loadingLabel = screenLoading, "Reading this directory level..."
		return a, loadDirectoryCmd(left, right)
	}
	a.config.Mode = ModeCompare
	a.config.Left, a.config.Right = left, right
	a.compareFromPicker = true
	a.directoryFromPicker = false
	a.screen, a.loadingLabel = screenLoading, "Loading file comparison..."
	return a, loadCompareCmd(left, right, true, true, false)
}
