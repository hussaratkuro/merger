package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"merger/internal/core"
)

func (a *App) visibleDirectoryEntries() []core.DirectoryEntry {
	if a.showSame {
		return a.dirEntries
	}
	result := make([]core.DirectoryEntry, 0, len(a.dirEntries))
	for _, entry := range a.dirEntries {
		if entry.Status != core.EntrySame {
			result = append(result, entry)
		}
	}
	return result
}

func (a *App) directoryVisibleRows() int {
	return max(3, a.height-10)
}

func (a *App) updateDirectory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if a.dirBusy && key != "ctrl+c" {
		return a, nil
	}
	entries := a.visibleDirectoryEntries()

	if key != "alt+left" && key != "alt+right" && key != "alt+delete" {
		a.dirPending = pendingDirectoryAction{}
	}

	switch key {
	case "esc":
		if a.dirQuery != "" {
			a.clearDirectoryQuery()
			return a, nil
		}
		if a.directoryFromPicker {
			a.screen = screenPicker
			return a, nil
		}
		a.cancelled = false
		return a, tea.Quit
	case "?":
		a.clearDirectoryQuery()
		a.returnScreen, a.screen = screenDirectory, screenHelp
		return a, nil
	case "tab":
		a.clearDirectoryQuery()
		a.dirFocus = 1 - a.dirFocus
	case "up":
		a.clearDirectoryQuery()
		a.dirCursor = max(0, a.dirCursor-1)
	case "down":
		a.clearDirectoryQuery()
		a.dirCursor = min(max(0, len(entries)-1), a.dirCursor+1)
	case "pgup":
		a.clearDirectoryQuery()
		a.dirCursor = max(0, a.dirCursor-a.directoryVisibleRows())
	case "pgdown":
		a.clearDirectoryQuery()
		a.dirCursor = min(max(0, len(entries)-1), a.dirCursor+a.directoryVisibleRows())
	case "home":
		a.clearDirectoryQuery()
		a.dirCursor = 0
	case "end":
		a.clearDirectoryQuery()
		a.dirCursor = max(0, len(entries)-1)
	case "ctrl+s":
		a.clearDirectoryQuery()
		a.showSame = !a.showSame
		a.dirCursor, a.dirOffset = 0, 0
		if a.showSame {
			a.setStatus("Showing entries with matching metadata.", false)
		} else {
			a.setStatus("Entries with matching metadata hidden.", false)
		}
	case "f5":
		a.clearDirectoryQuery()
		a.dirBusy = true
		a.setStatus("Refreshing this directory level...", false)
		return a, loadDirectoryCmd(a.dirLeft, a.dirRight)
	case "backspace", "ctrl+h":
		if a.dirQuery != "" {
			runes := []rune(a.dirQuery)
			a.dirQuery = string(runes[:len(runes)-1])
			a.dirQueryID++
			if a.dirQuery != "" {
				a.jumpToDirectoryMatch(entries)
				return a, a.expireDirectoryQueryCmd()
			}
			return a, nil
		}
		return a.openParentDirectory()
	case "enter":
		if len(entries) == 0 {
			break
		}
		a.clearDirectoryQuery()
		return a.openDirectoryEntry(entries[a.dirCursor])
	case "alt+right":
		a.clearDirectoryQuery()
		if len(entries) > 0 {
			return a.confirmDirectoryCopy(entries[a.dirCursor], 1)
		}
	case "alt+left":
		a.clearDirectoryQuery()
		if len(entries) > 0 {
			return a.confirmDirectoryCopy(entries[a.dirCursor], -1)
		}
	case "alt+delete":
		a.clearDirectoryQuery()
		if len(entries) > 0 {
			return a.confirmDirectoryDelete(entries[a.dirCursor])
		}
	}
	if isBrowserQueryKey(msg) {
		a.dirQuery += string(msg.Runes)
		a.dirQueryID++
		a.jumpToDirectoryMatch(entries)
		return a, a.expireDirectoryQueryCmd()
	}

	a.dirCursor = clamp(a.dirCursor, 0, max(0, len(entries)-1))
	a.dirOffset = adjustOffset(a.dirOffset, a.dirCursor, a.directoryVisibleRows())
	return a, nil
}

func (a *App) jumpToDirectoryMatch(entries []core.DirectoryEntry) {
	query := strings.ToLower(a.dirQuery)
	for index, entry := range entries {
		if strings.HasPrefix(strings.ToLower(entry.Name), query) {
			a.dirCursor = index
			a.dirOffset = adjustOffset(a.dirOffset, a.dirCursor, a.directoryVisibleRows())
			return
		}
	}
}

func (a *App) clearDirectoryQuery() {
	a.dirQuery = ""
	a.dirQueryID++
}

func (a *App) expireDirectoryQueryCmd() tea.Cmd {
	id := a.dirQueryID
	return tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg {
		return directoryQueryExpiredMsg{id: id}
	})
}

func (a *App) openDirectoryEntry(entry core.DirectoryEntry) (tea.Model, tea.Cmd) {
	leftPath := filepath.Join(a.dirLeft, entry.Name)
	rightPath := filepath.Join(a.dirRight, entry.Name)
	if entry.IsDirectory() {
		if entry.Left.Exists && entry.Right.Exists && entry.Left.Kind != entry.Right.Kind {
			a.setStatus("Cannot enter a directory/file type mismatch.", true)
			return a, nil
		}
		a.dirHistory = append(a.dirHistory, directoryLocation{
			left: a.dirLeft, right: a.dirRight, cursor: a.dirCursor, offset: a.dirOffset,
		})
		a.dirLeft, a.dirRight = leftPath, rightPath
		a.dirCursor, a.dirOffset = 0, 0
		a.screen = screenLoading
		a.loadingLabel = "Reading this directory level..."
		return a, loadDirectoryCmd(leftPath, rightPath)
	}
	if entry.Left.Kind != core.EntryFile && entry.Right.Kind != core.EntryFile {
		a.setStatus("Only regular files can be opened in the text comparator.", true)
		return a, nil
	}
	if entry.Left.Exists && entry.Right.Exists && entry.Left.Kind != entry.Right.Kind {
		a.setStatus("Cannot text-compare different entry types.", true)
		return a, nil
	}
	a.screen = screenLoading
	a.loadingLabel = "Loading file comparison..."
	return a, loadCompareCmd(leftPath, rightPath, entry.Left.Exists, entry.Right.Exists, true)
}

func (a *App) openParentDirectory() (tea.Model, tea.Cmd) {
	if len(a.dirHistory) > 0 {
		last := a.dirHistory[len(a.dirHistory)-1]
		a.dirHistory = a.dirHistory[:len(a.dirHistory)-1]
		a.dirLeft, a.dirRight = last.left, last.right
		a.dirCursor, a.dirOffset = last.cursor, last.offset
	} else {
		leftParent, rightParent := filepath.Dir(a.dirLeft), filepath.Dir(a.dirRight)
		if leftParent == a.dirLeft && rightParent == a.dirRight {
			a.setStatus("Already at the filesystem root.", false)
			return a, nil
		}
		a.dirLeft, a.dirRight = leftParent, rightParent
		a.dirCursor, a.dirOffset = 0, 0
	}
	a.screen = screenLoading
	a.loadingLabel = "Reading parent directory..."
	return a, loadDirectoryCmd(a.dirLeft, a.dirRight)
}

func (a *App) confirmDirectoryCopy(entry core.DirectoryEntry, direction int) (tea.Model, tea.Cmd) {
	action := pendingDirectoryAction{kind: "copy", name: entry.Name, direction: direction}
	if a.dirPending != action {
		a.dirPending = action
		arrow := "left → right"
		if direction < 0 {
			arrow = "right → left"
		}
		a.setStatus(fmt.Sprintf("Press the same Alt+arrow again to copy %q %s.", entry.Name, arrow), false)
		return a, nil
	}
	a.dirPending = pendingDirectoryAction{}
	var source, destination string
	if direction > 0 {
		if !entry.Left.Exists {
			a.setStatus("The selected entry does not exist on the left.", true)
			return a, nil
		}
		source = filepath.Join(a.dirLeft, entry.Name)
		destination = filepath.Join(a.dirRight, entry.Name)
	} else {
		if !entry.Right.Exists {
			a.setStatus("The selected entry does not exist on the right.", true)
			return a, nil
		}
		source = filepath.Join(a.dirRight, entry.Name)
		destination = filepath.Join(a.dirLeft, entry.Name)
	}
	a.dirBusy = true
	a.setStatus("Copying "+entry.Name+"...", false)
	return a, func() tea.Msg {
		err := core.CopyPath(source, destination)
		return directoryOperationMsg{description: "Copied " + entry.Name + ".", err: err}
	}
}

func (a *App) confirmDirectoryDelete(entry core.DirectoryEntry) (tea.Model, tea.Cmd) {
	action := pendingDirectoryAction{kind: "delete", name: entry.Name, direction: a.dirFocus}
	if a.dirPending != action {
		a.dirPending = action
		side := "left"
		if a.dirFocus == 1 {
			side = "right"
		}
		a.setStatus(fmt.Sprintf("Press Alt+Delete again to permanently delete %q from the %s.", entry.Name, side), true)
		return a, nil
	}
	a.dirPending = pendingDirectoryAction{}
	base := a.dirLeft
	exists := entry.Left.Exists
	if a.dirFocus == 1 {
		base, exists = a.dirRight, entry.Right.Exists
	}
	if !exists {
		a.setStatus("The selected entry does not exist on the focused side.", true)
		return a, nil
	}
	target := filepath.Join(base, entry.Name)
	a.dirBusy = true
	a.setStatus("Deleting "+entry.Name+"...", true)
	return a, func() tea.Msg {
		err := core.DeletePath(target)
		return directoryOperationMsg{description: "Deleted " + entry.Name + ".", err: err}
	}
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
