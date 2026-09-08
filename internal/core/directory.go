package core

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type EntryKind uint8

const (
	EntryMissing EntryKind = iota
	EntryFile
	EntryDirectory
	EntrySymlink
	EntryOther
)

type EntrySide struct {
	Exists  bool
	Kind    EntryKind
	Size    int64
	Mode    os.FileMode
	ModNano int64
	Link    string
}

type EntryStatus uint8

const (
	EntrySame EntryStatus = iota
	EntryDifferent
	EntryLeftOnly
	EntryRightOnly
	EntryTypeMismatch
	EntryFolder
	EntryMetadataDifferent
)

func (s EntryStatus) String() string {
	switch s {
	case EntrySame:
		return "same metadata"
	case EntryDifferent:
		return "different"
	case EntryLeftOnly:
		return "left only"
	case EntryRightOnly:
		return "right only"
	case EntryTypeMismatch:
		return "type mismatch"
	case EntryFolder:
		return "folder"
	case EntryMetadataDifferent:
		return "metadata"
	default:
		return "unknown"
	}
}

type DirectoryEntry struct {
	Name   string
	Left   EntrySide
	Right  EntrySide
	Status EntryStatus
}

type BrowserEntry struct {
	Name string
	Path string
	Kind EntryKind
}

// ListDirectory reads one directory level for the interactive path picker.
// Like CompareDirectory, it deliberately never descends into child folders.
func ListDirectory(path string, showHidden bool) ([]BrowserEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("browse directory: %w", err)
	}
	result := make([]BrowserEntry, 0, len(entries))
	for _, entry := range entries {
		if !showHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		fullPath := filepath.Join(path, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}
		result = append(result, BrowserEntry{Name: entry.Name(), Path: fullPath, Kind: kindOf(info.Mode())})
	}
	sort.Slice(result, func(i, j int) bool {
		iDir, jDir := result[i].Kind == EntryDirectory, result[j].Kind == EntryDirectory
		if iDir != jDir {
			return iDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func (e DirectoryEntry) IsDirectory() bool {
	return e.Left.Kind == EntryDirectory || e.Right.Kind == EntryDirectory
}

// CompareDirectory reads exactly one level. It never stats or indexes children
// of a subdirectory, so opening a directory with huge nested trees stays fast.
func CompareDirectory(leftPath, rightPath string) ([]DirectoryEntry, error) {
	left, err := readOneDirectory(leftPath)
	if err != nil {
		return nil, fmt.Errorf("left directory: %w", err)
	}
	right, err := readOneDirectory(rightPath)
	if err != nil {
		return nil, fmt.Errorf("right directory: %w", err)
	}
	names := make(map[string]struct{}, len(left)+len(right))
	for name := range left {
		names[name] = struct{}{}
	}
	for name := range right {
		names[name] = struct{}{}
	}
	entries := make([]DirectoryEntry, 0, len(names))
	for name := range names {
		l, lok := left[name]
		r, rok := right[name]
		entries = append(entries, DirectoryEntry{
			Name: name, Left: l, Right: r,
			Status: compareEntrySides(l, lok, r, rok),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		iDir, jDir := entries[i].IsDirectory(), entries[j].IsDirectory()
		if iDir != jDir {
			return iDir
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func readOneDirectory(path string) (map[string]EntrySide, error) {
	result := map[string]EntrySide{}
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		fullPath := filepath.Join(path, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}
		side := EntrySide{
			Exists: true, Kind: kindOf(info.Mode()), Size: info.Size(),
			Mode: info.Mode(), ModNano: info.ModTime().UnixNano(),
		}
		if side.Kind == EntrySymlink {
			side.Link, _ = os.Readlink(fullPath)
		}
		result[entry.Name()] = side
	}
	return result, nil
}

func kindOf(mode os.FileMode) EntryKind {
	switch {
	case mode.IsRegular():
		return EntryFile
	case mode.IsDir():
		return EntryDirectory
	case mode&os.ModeSymlink != 0:
		return EntrySymlink
	default:
		return EntryOther
	}
}

func compareEntrySides(left EntrySide, leftOK bool, right EntrySide, rightOK bool) EntryStatus {
	switch {
	case !leftOK:
		return EntryRightOnly
	case !rightOK:
		return EntryLeftOnly
	case left.Kind != right.Kind:
		return EntryTypeMismatch
	case left.Kind == EntryDirectory:
		return EntryFolder
	case left.Kind == EntrySymlink:
		if left.Link == right.Link {
			return EntrySame
		}
		return EntryDifferent
	case left.Kind == EntryFile:
		// This is intentionally a metadata comparison. Exact contents are read
		// only when the user opens this file, keeping directory opening O(entries)
		// with no recursive or bulk file reads.
		if left.Size != right.Size {
			return EntryDifferent
		}
		if left.ModNano != right.ModNano || left.Mode.Perm() != right.Mode.Perm() {
			return EntryMetadataDifferent
		}
		return EntrySame
	default:
		if left.Mode == right.Mode && left.Size == right.Size {
			return EntrySame
		}
		return EntryDifferent
	}
}

func CopyPath(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect source: %w", err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return copySymlink(source, destination)
	case info.IsDir():
		return copyDirectory(source, destination, info.Mode())
	case info.Mode().IsRegular():
		return copyFile(source, destination, info.Mode())
	default:
		return fmt.Errorf("unsupported source type: %s", source)
	}
}

func copyDirectory(source, destination string, mode os.FileMode) error {
	if target, err := os.Lstat(destination); err == nil && !target.IsDir() {
		return fmt.Errorf("destination is not a directory: %s", destination)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(destination, mode.Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := CopyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	if target, err := os.Lstat(destination); err == nil && target.IsDir() {
		return fmt.Errorf("destination is a directory: %s", destination)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".merger-copy-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode.Perm()); err != nil {
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return err
	}
	keep = true
	return nil
}

func copySymlink(source, destination string) error {
	target, err := os.Readlink(source)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(destination); err == nil {
		if info.IsDir() {
			return fmt.Errorf("destination is a directory: %s", destination)
		}
		if err := os.Remove(destination); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, destination)
}

func DeletePath(path string) error {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	root := string(os.PathSeparator)
	if volume != "" {
		root = volume + string(os.PathSeparator)
	}
	if clean == "" || clean == "." || clean == root {
		return fmt.Errorf("refusing to delete broad path %q", path)
	}
	return os.RemoveAll(clean)
}
