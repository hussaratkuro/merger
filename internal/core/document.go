package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Document struct {
	Path         string
	Label        string
	Lines        []string
	EOL          string
	FinalNewline bool
	Mode         os.FileMode
	Binary       bool
	Data         []byte
	Dirty        bool
}

func ReadDocument(path, label string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Document{}, fmt.Errorf("stat %s: %w", path, err)
	}
	doc := Document{Path: path, Label: label, Mode: info.Mode(), Data: data, Binary: bytes.IndexByte(data, 0) >= 0}
	doc.setText(string(data))
	doc.Dirty = false
	return doc, nil
}

func (d *Document) setText(text string) {
	d.EOL = detectedEOL(text)
	if d.EOL == "" {
		d.EOL = "\n"
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	d.FinalNewline = strings.HasSuffix(normalized, "\n")
	if d.FinalNewline {
		normalized = strings.TrimSuffix(normalized, "\n")
	}
	if normalized == "" {
		d.Lines = nil
	} else {
		d.Lines = strings.Split(normalized, "\n")
	}
	d.Data = []byte(text)
}

func (d *Document) SetEditorText(text string) {
	previousEOL := d.EOL
	d.setText(text)
	if previousEOL != "" {
		d.EOL = previousEOL
	}
	d.Binary = false
	d.Dirty = true
}

func (d Document) Text() string {
	if d.Binary {
		return string(d.Data)
	}
	text := strings.Join(d.Lines, d.EOL)
	if d.FinalNewline {
		text += d.EOL
	}
	return text
}

func (d Document) LFText() string {
	text := strings.Join(d.Lines, "\n")
	if d.FinalNewline {
		text += "\n"
	}
	return text
}

func detectedEOL(text string) string {
	crlf := strings.Count(text, "\r\n")
	lf := strings.Count(strings.ReplaceAll(text, "\r\n", ""), "\n")
	if crlf > lf {
		return "\r\n"
	}
	if crlf > 0 || lf > 0 {
		return "\n"
	}
	return ""
}

func SplitEditorLines(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".merger-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if mode.Perm() == 0 {
		mode = 0o644
	}
	if err := tmp.Chmod(mode.Perm()); err != nil {
		return fmt.Errorf("set output permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync output: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	keep = true
	return nil
}
