package editor

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// ctagsIndex is a SymbolProvider backed by a universal-ctags tag index.
//
// Definitions come from the tag index, which is built lazily (one ctags -R
// pass over the project root) and rebuilt whenever a file is saved. Usages
// are answered by scanning the project tree for whole-word occurrences, which
// works across all languages; per-language ctags reference tags could replace
// that scan later.
type ctagsIndex struct {
	root  string
	mu    sync.RWMutex
	idx   map[string][]Loc
	lower map[string][]Loc
	ready bool
}

func newCtagsIndex(root string) *ctagsIndex {
	return &ctagsIndex{root: root}
}

// Ready reports whether the tag index has been built.
func (c *ctagsIndex) Ready() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// Build runs universal-ctags over the root and swaps in a fresh tag index.
func (c *ctagsIndex) Build() error {
	exe, err := exec.LookPath("ctags")
	if err != nil {
		return fmt.Errorf("find-definition needs universal-ctags; install it (e.g. apt install universal-ctags or brew install universal-ctags)")
	}
	root, err := filepath.Abs(c.root)
	if err != nil {
		return err
	}
	args := []string{"-R", "--fields=+n", "--sort=no", "-o", "-"}
	for d := range skipDirs {
		args = append(args, "--exclude="+d)
	}
	args = append(args, root)
	cmd := exec.Command(exe, args...)
	out, err := cmd.CombinedOutput()
	idx, lower := parseCtags(out, root)
	if err != nil && len(idx) == 0 {
		return fmt.Errorf("ctags failed: %v", err)
	}
	c.mu.Lock()
	c.idx = idx
	c.lower = lower
	c.ready = true
	c.mu.Unlock()
	return nil
}

// Definitions returns tag matches for sym: exact first, case-insensitive as a
// fallback.
func (c *ctagsIndex) Definitions(sym string) []Loc {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if l, ok := c.idx[sym]; ok && len(l) > 0 {
		return l
	}
	return c.lower[strings.ToLower(sym)]
}

// Usages scans the project for whole-word occurrences of sym, skipping heavy
// directories. The caller runs this in a background goroutine.
func (c *ctagsIndex) Usages(sym string) []Loc {
	var locs []Loc
	filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != c.root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return nil // skip binary files
		}
		for i, ln := range strings.Split(string(data), "\n") {
			if col := findWordCol(sym, []rune(ln)); col >= 0 {
				locs = append(locs, Loc{File: path, Line: i, Col: col})
			}
		}
		return nil
	})
	return locs
}

// parseCtags parses a ctags tag stream into exact and lowercased indices.
// Tolerates warning lines (which lack tab-separated fields). Relative file
// paths from the stream are resolved against root (ctags emits the paths it
// was given, which we pass absolute).
func parseCtags(out []byte, root string) (map[string][]Loc, map[string][]Loc) {
	idx := map[string][]Loc{}
	lower := map[string][]Loc{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '!' || !strings.Contains(line, "\t") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		name := fields[0]
		file := fields[1]
		lineNo := 0
		for _, f := range fields[3:] {
			if rest, ok := strings.CutPrefix(f, "line:"); ok {
				if n, err := strconv.Atoi(rest); err == nil {
					lineNo = n
				}
				break
			}
		}
		if lineNo < 1 {
			continue
		}
		loc := Loc{File: file, Line: lineNo - 1}
		if !filepath.IsAbs(loc.File) {
			loc.File = filepath.Join(root, loc.File)
		}
		if abs, err := filepath.Abs(loc.File); err == nil {
			loc.File = abs
		}
		idx[name] = append(idx[name], loc)
		lk := strings.ToLower(name)
		lower[lk] = append(lower[lk], loc)
	}
	return idx, lower
}
