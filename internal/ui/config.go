package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Mode uint8

const (
	ModeCompare Mode = iota
	ModeDirectory
	ModeMerge
	ModePicker
)

type Config struct {
	Mode               Mode
	Left, Right        string
	LeftLabel          string
	RightLabel         string
	Mine, Base, Theirs string
	Output             string
	BrowseRoot         string
	ReadOnly           bool
	IgnoreWhitespace   bool
	IgnoreEOL          bool
	Syntax             bool
}

const Usage = `merger - Catppuccin terminal diff and merge tool

Usage:
  merger
      Open an interactive file and directory picker.

  merger LEFT RIGHT
      Compare two files or two directories.

  merger --read-only --label-left BASE --label-right WORKING LEFT RIGHT
      Open a labelled comparison with all filesystem writes disabled.

  merger MINE BASE THEIRS --output RESULT
      Three-way merge compatible with Meld's SVN argument order.

  merger --mine MINE --base BASE --theirs THEIRS --output RESULT
      Explicit three-way merge form.

Keys:
  Alt+Up/Down       previous/next change
  Alt+Left/Right    push the selected change in the arrow direction
  Enter             open a file/directory
  Backspace         parent directory
  Space / Tab       select the highlighted path
  Type a name       jump to a matching picker entry
  Type / click      edit directly in a two-file comparison and in the
                    MERGED RESULT pane of a three-way merge
  Alt+B/A/U         three-way merge: base / both / restore conflict markers
  Ctrl+Z            undo
  Ctrl+Shift+Z/Y    redo
  Ctrl+S            save
  F1 / ?            help (F1 in the editable comparison and merge views)
  Ctrl+F / /         search comparison text (/ is read-only mode only)
  Alt+W              toggle ignoring whitespace-only differences
  Alt+E              toggle line-ending differences
  Alt+S              toggle lightweight syntax highlighting
  Ctrl+Shift+P       fuzzy command palette (Ctrl+P fallback)
`

func ParseArgs(args []string) (Config, error) {
	var cfg Config
	if len(args) == 0 {
		cfg.Mode = ModePicker
		cfg.BrowseRoot = pickerStartDirectory()
		return cfg, nil
	}
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := func(name string) (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a path", name)
			}
			i++
			return args[i], nil
		}
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return Config{}, errHelp
		case arg == "--read-only":
			cfg.ReadOnly = true
		case arg == "--ignore-whitespace":
			cfg.IgnoreWhitespace = true
		case arg == "--ignore-eol":
			cfg.IgnoreEOL = true
		case arg == "--syntax":
			cfg.Syntax = true
		case arg == "--label-left":
			v, err := value(arg)
			if err != nil {
				return Config{}, err
			}
			cfg.LeftLabel = v
		case strings.HasPrefix(arg, "--label-left="):
			cfg.LeftLabel = strings.TrimPrefix(arg, "--label-left=")
		case arg == "--label-right":
			v, err := value(arg)
			if err != nil {
				return Config{}, err
			}
			cfg.RightLabel = v
		case strings.HasPrefix(arg, "--label-right="):
			cfg.RightLabel = strings.TrimPrefix(arg, "--label-right=")
		case arg == "--mine":
			v, err := value(arg)
			if err != nil {
				return Config{}, err
			}
			cfg.Mine = v
		case strings.HasPrefix(arg, "--mine="):
			cfg.Mine = strings.TrimPrefix(arg, "--mine=")
		case arg == "--base":
			v, err := value(arg)
			if err != nil {
				return Config{}, err
			}
			cfg.Base = v
		case strings.HasPrefix(arg, "--base="):
			cfg.Base = strings.TrimPrefix(arg, "--base=")
		case arg == "--theirs":
			v, err := value(arg)
			if err != nil {
				return Config{}, err
			}
			cfg.Theirs = v
		case strings.HasPrefix(arg, "--theirs="):
			cfg.Theirs = strings.TrimPrefix(arg, "--theirs=")
		case arg == "--output":
			v, err := value(arg)
			if err != nil {
				return Config{}, err
			}
			cfg.Output = v
		case strings.HasPrefix(arg, "--output="):
			cfg.Output = strings.TrimPrefix(arg, "--output=")
		case strings.HasPrefix(arg, "-"):
			return Config{}, fmt.Errorf("unknown option: %s", arg)
		default:
			positional = append(positional, arg)
		}
	}

	explicitMerge := cfg.Mine != "" || cfg.Base != "" || cfg.Theirs != ""
	if explicitMerge {
		if len(positional) != 0 {
			return Config{}, fmt.Errorf("do not mix named merge paths with positional paths")
		}
		if cfg.Mine == "" || cfg.Base == "" || cfg.Theirs == "" {
			return Config{}, fmt.Errorf("--mine, --base and --theirs are all required")
		}
		cfg.Mode = ModeMerge
	} else {
		switch len(positional) {
		case 2:
			cfg.Left, cfg.Right = positional[0], positional[1]
		case 3:
			cfg.Mode = ModeMerge
			cfg.Mine, cfg.Base, cfg.Theirs = positional[0], positional[1], positional[2]
		default:
			return Config{}, fmt.Errorf("expected two paths, or three paths for a merge")
		}
	}

	if cfg.Mode == ModeMerge {
		if cfg.ReadOnly || cfg.LeftLabel != "" || cfg.RightLabel != "" || cfg.IgnoreWhitespace || cfg.IgnoreEOL || cfg.Syntax {
			return Config{}, fmt.Errorf("read-only comparison options are not valid for a three-way merge")
		}
		if cfg.Output == "" {
			cfg.Output = cfg.Mine
		}
		var err error
		cfg.Mine, err = absolutePath(cfg.Mine)
		if err != nil {
			return Config{}, err
		}
		cfg.Base, _ = absolutePath(cfg.Base)
		cfg.Theirs, _ = absolutePath(cfg.Theirs)
		cfg.Output, _ = absolutePath(cfg.Output)
		return cfg, nil
	}
	if cfg.Output != "" {
		return Config{}, fmt.Errorf("--output is only valid for a three-way merge")
	}

	var err error
	cfg.Left, err = absolutePath(cfg.Left)
	if err != nil {
		return Config{}, err
	}
	cfg.Right, err = absolutePath(cfg.Right)
	if err != nil {
		return Config{}, err
	}
	leftInfo, leftErr := os.Stat(cfg.Left)
	rightInfo, rightErr := os.Stat(cfg.Right)
	if leftErr != nil || rightErr != nil {
		return Config{}, fmt.Errorf("both comparison paths must exist")
	}
	if leftInfo.IsDir() != rightInfo.IsDir() {
		return Config{}, fmt.Errorf("cannot compare a file with a directory")
	}
	if leftInfo.IsDir() {
		cfg.Mode = ModeDirectory
	}
	if cfg.LeftLabel == "" {
		cfg.LeftLabel = "LEFT"
	}
	if cfg.RightLabel == "" {
		cfg.RightLabel = "RIGHT"
	}
	return cfg, nil
}

func pickerStartDirectory() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Clean(home)
	}
	if current, err := os.Getwd(); err == nil && current != "" {
		return filepath.Clean(current)
	}
	return string(os.PathSeparator)
}

var errHelp = fmt.Errorf("help requested")

func IsHelp(err error) bool { return err == errHelp }

func absolutePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	return filepath.Clean(abs), nil
}
