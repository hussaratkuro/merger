package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentPreservesCRLFWhenEdited(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("one\r\ntwo\r\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	doc, err := ReadDocument(path, "LEFT")
	if err != nil {
		t.Fatal(err)
	}
	doc.SetEditorText("one\nchanged\n")
	if got, want := doc.Text(), "one\r\nchanged\r\n"; got != want {
		t.Fatalf("edited text = %q, want %q", got, want)
	}
}

func TestAtomicWriteReplacesContentAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.txt")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("new"), 0o640); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("result = %q, err = %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, err = %v", info.Mode().Perm(), err)
	}
}
