package editor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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

// buildTimeout bounds a full index build so a huge or pathological tree cannot
// leave the editor showing "Building symbol index..." forever.
const buildTimeout = 60 * time.Second

// usageScanWorkers is how many files are scanned in parallel for usages.
var usageScanWorkers = runtime.NumCPU()

// maxScanBytes skips files too large to be source code.
const maxScanBytes = 8 << 20

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
		return fmt.Errorf("find-definition needs universal-ctags; install it (apt/brew install universal-ctags, choco install universal-ctags, or the prebuilt zip from github.com/universal-ctags/ctags-win32)")
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
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	// Keep stderr separate: a warning line mixed into the tag stream could be
	// parsed as a bogus tag.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return fmt.Errorf("ctags timed out after %s over %s", buildTimeout, root)
	}
	idx, lower := parseCtags(out, root)
	if err != nil && len(idx) == 0 {
		detail := strings.TrimSpace(stderr.String())
		if len(detail) > 300 {
			detail = detail[:300] + "..."
		}
		if detail != "" {
			return fmt.Errorf("ctags failed (%v); universal-ctags required. output: %s", err, detail)
		}
		return fmt.Errorf("ctags failed (%v); universal-ctags required", err)
	}
	c.mu.Lock()
	c.idx = idx
	c.lower = lower
	c.ready = true
	c.mu.Unlock()
	return nil
}

// UpdateFile re-runs ctags for a single file and swaps its tags into the
// index, so saving costs one file's worth of work instead of a full re-index.
func (c *ctagsIndex) UpdateFile(path string) error {
	exe, err := exec.LookPath("ctags")
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(c.root)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(root, abs); err != nil || strings.HasPrefix(rel, "..") {
		return nil // outside the project: nothing indexed for it
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "--fields=+n", "--sort=no", "-o", "-", abs)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return fmt.Errorf("ctags failed for %s: %v", filepath.Base(abs), err)
	}
	fresh, freshLower := parseCtags(out, root)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idx == nil {
		c.idx = map[string][]Loc{}
		c.lower = map[string][]Loc{}
	}
	dropFile(c.idx, abs)
	dropFile(c.lower, abs)
	for name, locs := range fresh {
		c.idx[name] = append(c.idx[name], locs...)
	}
	for name, locs := range freshLower {
		c.lower[name] = append(c.lower[name], locs...)
	}
	c.ready = true
	return nil
}

// dropFile removes every tag that points into path.
func dropFile(m map[string][]Loc, path string) {
	for name, locs := range m {
		kept := locs[:0]
		for _, l := range locs {
			if !samePath(l.File, path) {
				kept = append(kept, l)
			}
		}
		if len(kept) == 0 {
			delete(m, name)
			continue
		}
		m[name] = kept
	}
}

// Definitions returns tag matches for sym: exact first, case-insensitive as a
// fallback.
func (c *ctagsIndex) Definitions(sym string) []Loc {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if l, ok := c.idx[sym]; ok && len(l) > 0 {
		return append([]Loc(nil), l...)
	}
	return append([]Loc(nil), c.lower[strings.ToLower(sym)]...)
}

// Usages scans the project for whole-word occurrences of sym, skipping heavy
// directories and binary files. Files are scanned in parallel and the scan
// stops early when cancel is closed, so a superseded lookup stops burning CPU.
// The result is sorted so the list is stable between runs.
func (c *ctagsIndex) Usages(sym string, cancel <-chan struct{}) []Loc {
	paths := make(chan string, 256)
	go func() {
		defer close(paths)
		filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
			select {
			case <-cancel:
				return filepath.SkipAll
			default:
			}
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != c.root && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if fi, err := d.Info(); err == nil && fi.Size() > maxScanBytes {
				return nil
			}
			select {
			case paths <- path:
			case <-cancel:
				return filepath.SkipAll
			}
			return nil
		})
	}()

	workers := usageScanWorkers
	if workers < 1 {
		workers = 1
	}
	var mu sync.Mutex
	var locs []Loc
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range paths {
				select {
				case <-cancel:
					return
				default:
				}
				found := scanFileForWord(path, sym)
				if len(found) == 0 {
					continue
				}
				mu.Lock()
				locs = append(locs, found...)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	sort.Slice(locs, func(i, j int) bool {
		if locs[i].File != locs[j].File {
			return locs[i].File < locs[j].File
		}
		return locs[i].Line < locs[j].Line
	})
	return locs
}

// scanFileForWord returns the whole-word occurrences of sym in one file.
func scanFileForWord(path, sym string) []Loc {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil // skip binary files
	}
	if !bytes.Contains(data, []byte(sym)) {
		return nil // cheap reject before splitting into lines
	}
	var out []Loc
	line := 0
	for len(data) > 0 {
		nl := bytes.IndexByte(data, '\n')
		var chunk []byte
		if nl < 0 {
			chunk, data = data, nil
		} else {
			chunk, data = data[:nl], data[nl+1:]
		}
		if bytes.Contains(chunk, []byte(sym)) {
			if col := findWordCol(sym, decodeRunes(chunk)); col >= 0 {
				out = append(out, Loc{File: path, Line: line, Col: col})
			}
		}
		line++
	}
	return out
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
