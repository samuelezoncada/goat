package editor

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds user settings. Zero values are not the defaults; use
// DefaultConfig.
type Config struct {
	TabWidth   int    // display width of a tab, and indent step
	ExpandTab  bool   // insert spaces instead of a tab character
	AutoIndent bool   // copy the previous line's indentation on Enter
	Wrap       bool   // soft-wrap long lines instead of scrolling horizontally
	Theme      string // "dark" or "light"
	Clipboard  bool   // sync cut/copy with the terminal clipboard (OSC 52)
	GitBranch  bool   // show the current git branch in the status bar

	// Warnings collected while parsing, surfaced in the status bar so a typo
	// in the config file is not silently ignored.
	Warnings []string
	Path     string // the file the settings were read from, "" if none
}

// DefaultConfig returns the built-in settings.
func DefaultConfig() *Config {
	return &Config{
		TabWidth:   8,
		ExpandTab:  false,
		AutoIndent: true,
		Wrap:       false,
		Theme:      "dark",
		Clipboard:  true,
		GitBranch:  true,
	}
}

// configPath returns the config file location: $GOAT_CONFIG, else
// $XDG_CONFIG_HOME/goat/config, else ~/.config/goat/config.
func configPath() string {
	if p := os.Getenv("GOAT_CONFIG"); p != "" {
		return p
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "goat", "config")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "goat", "config")
}

// LoadConfig reads the config file, falling back to defaults for anything
// missing or malformed. A missing file is not an error.
func LoadConfig() *Config {
	cfg := DefaultConfig()
	path := configPath()
	if path == "" {
		return cfg
	}
	f, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer f.Close()
	cfg.Path = path
	cfg.parse(f)
	return cfg
}

// parse reads "key = value" lines, ignoring blanks and # comments.
func (c *Config) parse(r io.Reader) {
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimSpace(sc.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		key, val, ok := strings.Cut(s, "=")
		if !ok {
			c.warnf("line %d: expected key = value", line)
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "tabwidth":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > 32 {
				c.warnf("line %d: tabwidth must be 1..32", line)
				continue
			}
			c.TabWidth = n
		case "expandtab":
			c.setBool(&c.ExpandTab, key, val, line)
		case "autoindent":
			c.setBool(&c.AutoIndent, key, val, line)
		case "wrap":
			c.setBool(&c.Wrap, key, val, line)
		case "clipboard":
			c.setBool(&c.Clipboard, key, val, line)
		case "gitbranch":
			c.setBool(&c.GitBranch, key, val, line)
		case "theme":
			switch strings.ToLower(val) {
			case "dark", "light":
				c.Theme = strings.ToLower(val)
			default:
				c.warnf("line %d: theme must be dark or light", line)
			}
		default:
			c.warnf("line %d: unknown setting %q", line, key)
		}
	}
}

func (c *Config) setBool(dst *bool, key, val string, line int) {
	switch strings.ToLower(val) {
	case "true", "yes", "on", "1":
		*dst = true
	case "false", "no", "off", "0":
		*dst = false
	default:
		c.warnf("line %d: %s must be true or false", line, key)
	}
}

func (c *Config) warnf(format string, args ...any) {
	if len(c.Warnings) < 10 {
		c.Warnings = append(c.Warnings, sprintf(format, args...))
	}
}

// indentUnit returns the runes inserted for one indent step.
func (c *Config) indentUnit() []rune {
	if c.ExpandTab {
		return []rune(strings.Repeat(" ", c.TabWidth))
	}
	return []rune{'\t'}
}

// ConfigPath returns the location goat reads its settings from, for --help.
func ConfigPath() string { return configPath() }
