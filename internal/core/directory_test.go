package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareDirectoryIsOneLevelAndCollapsed(t *testing.T) {
	left, right := filepath.Join(t.TempDir(), "left"), filepath.Join(t.TempDir(), "right")
	for _, root := range []string{left, right} {
		if err := os.MkdirAll(filepath.Join(root, "nested", "deep"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "nested", "deep", "hidden.txt"), []byte("not indexed"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := CompareDirectory(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "nested" || entries[0].Status != EntryFolder {
		t.Fatalf("one-level entries = %#v", entries)
	}
}

func TestCompareDirectoryQuickStatuses(t *testing.T) {
	left, right := filepath.Join(t.TempDir(), "left"), filepath.Join(t.TempDir(), "right")
	if err := os.Mkdir(left, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(right, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "only-left"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(right, "only-right"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := CompareDirectory(left, right)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]EntryStatus{}
	for _, entry := range entries {
		statuses[entry.Name] = entry.Status
	}
	if statuses["only-left"] != EntryLeftOnly || statuses["only-right"] != EntryRightOnly {
		t.Fatalf("statuses = %#v", statuses)
	}
}

func TestCompareDirectorySeparatesMetadataFromDefiniteContentDifference(t *testing.T) {
	left, right := filepath.Join(t.TempDir(), "left"), filepath.Join(t.TempDir(), "right")
	for _, root := range []string{left, right} {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	leftFile := filepath.Join(left, "same-size.txt")
	rightFile := filepath.Join(right, "same-size.txt")
	if err := os.WriteFile(leftFile, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rightFile, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "different-size.txt"), []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(right, "different-size.txt"), []byte("longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := CompareDirectory(left, right)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]EntryStatus{}
	for _, entry := range entries {
		statuses[entry.Name] = entry.Status
	}
	if statuses["same-size.txt"] != EntryMetadataDifferent {
		t.Fatalf("same-size metadata status = %v", statuses["same-size.txt"])
	}
	if statuses["different-size.txt"] != EntryDifferent {
		t.Fatalf("different-size status = %v", statuses["different-size.txt"])
	}
}

func TestCopyPathCopiesDirectoryOnlyWhenExplicitlyCalled(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "file"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CopyPath(source, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "nested", "file"))
	if err != nil || string(data) != "content" {
		t.Fatalf("copied data = %q, err = %v", data, err)
	}
}

func TestDeletePathRejectsRoot(t *testing.T) {
	if err := DeletePath(string(os.PathSeparator)); err == nil {
		t.Fatal("DeletePath accepted the filesystem root")
	}
}

func TestListDirectoryIsOneLevelAndCanToggleHiddenEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "folder", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("visible"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("hidden"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ListDirectory(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "folder" || entries[0].Kind != EntryDirectory || entries[1].Name != "file.txt" {
		t.Fatalf("visible entries = %#v", entries)
	}
	entries, err = ListDirectory(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries with hidden files = %#v", entries)
	}
}
