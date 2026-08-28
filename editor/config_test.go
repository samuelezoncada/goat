package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	c := DefaultConfig()
	if c.TabWidth != 8 || c.ExpandTab || !c.AutoIndent || c.Wrap || c.Theme != "dark" || !c.Clipboard {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestConfigParse(t *testing.T) {
	c := DefaultConfig()
	c.parse(strings.NewReader(`
# goat settings
tabwidth = 4
expandtab = true
autoindent = no
wrap = on
theme = light
clipboard = false
`))
	if c.TabWidth != 4 {
		t.Errorf("tabwidth = %d", c.TabWidth)
	}
	if !c.ExpandTab || c.AutoIndent || !c.Wrap || c.Clipboard {
		t.Errorf("bools wrong: %+v", c)
	}
	if c.Theme != "light" {
		t.Errorf("theme = %q", c.Theme)
	}
	if len(c.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", c.Warnings)
	}
}

func TestConfigWarnsOnGarbage(t *testing.T) {
	c := DefaultConfig()
	c.parse(strings.NewReader("tabwidth = zero\nnope = 1\nbroken line\ntheme = purple\n"))
	if len(c.Warnings) != 4 {
		t.Fatalf("warnings = %v, want one per bad line", c.Warnings)
	}
	// Bad values leave the defaults in place.
	if c.TabWidth != 8 || c.Theme != "dark" {
		t.Fatalf("defaults not preserved: %+v", c)
	}
}

func TestConfigLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	os.WriteFile(path, []byte("tabwidth = 2\ntheme = light\n"), 0o644)
	t.Setenv("GOAT_CONFIG", path)
	c := LoadConfig()
	if c.TabWidth != 2 || c.Theme != "light" {
		t.Fatalf("loaded %+v", c)
	}
	if c.Path != path {
		t.Fatalf("path = %q", c.Path)
	}
}

func TestConfigMissingFileIsFine(t *testing.T) {
	t.Setenv("GOAT_CONFIG", filepath.Join(t.TempDir(), "nope"))
	c := LoadConfig()
	if c.TabWidth != 8 {
		t.Fatalf("missing config should leave defaults: %+v", c)
	}
}

func TestIndentUnit(t *testing.T) {
	if got := string((&Config{TabWidth: 3, ExpandTab: true}).indentUnit()); got != "   " {
		t.Fatalf("expandtab unit %q", got)
	}
	if got := string((&Config{TabWidth: 3}).indentUnit()); got != "\t" {
		t.Fatalf("tab unit %q", got)
	}
}
