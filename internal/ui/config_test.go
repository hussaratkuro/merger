package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseArgsWithoutPathsStartsPicker(t *testing.T) {
	cfg, err := ParseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModePicker || !filepath.IsAbs(cfg.BrowseRoot) {
		t.Fatalf("picker config = %#v", cfg)
	}
}

func TestParseArgsDetectsDirectoryMode(t *testing.T) {
	left, right := filepath.Join(t.TempDir(), "left"), filepath.Join(t.TempDir(), "right")
	if err := os.Mkdir(left, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(right, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseArgs([]string{left, right})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeDirectory {
		t.Fatalf("mode = %v, want directory", cfg.Mode)
	}
}

func TestParseArgsSupportsMeldCompatibleThreeWayForm(t *testing.T) {
	cfg, err := ParseArgs([]string{"mine", "base", "theirs", "--output=result"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeMerge || filepath.Base(cfg.Mine) != "mine" || filepath.Base(cfg.Output) != "result" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestParseArgsRejectsMixedFileAndDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseArgs([]string{dir, file}); err == nil {
		t.Fatal("mixed file/directory comparison was accepted")
	}
}

func TestParseArgsRejectsOutputForTwoWayComparison(t *testing.T) {
	left := filepath.Join(t.TempDir(), "left")
	right := filepath.Join(t.TempDir(), "right")
	for _, path := range []string{left, right} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ParseArgs([]string{left, right, "--output=result"}); err == nil {
		t.Fatal("two-way comparison silently accepted --output")
	}
}
