package editor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestSaveRoundTrip is the regression test for the trailing-newline bug: a
// newline-terminated file used to grow by one blank line on every save.
func TestSaveRoundTrip(t *testing.T) {
	for _, content := range []string{
		"a\nb\n",
		"a\nb",
		"",
		"\n",
		"x",
		"a\n\n\n",
		"one\ntwo\nthree\n",
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "f.txt")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		tb, err := OpenTab(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := tb.saveTo(path); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Errorf("round trip %q -> %q", content, string(got))
		}
		// A second open/save cycle must be stable too.
		tb2, err := OpenTab(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := tb2.saveTo(path); err != nil {
			t.Fatal(err)
		}
		got2, _ := os.ReadFile(path)
		if string(got2) != content {
			t.Errorf("second cycle %q -> %q", content, string(got2))
		}
	}
}

func TestNoPhantomTrailingLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(path, []byte("a\nb\n"), 0o644)
	tb, err := OpenTab(path)
	if err != nil {
		t.Fatal(err)
	}
	if tb.lineCount() != 2 {
		t.Fatalf("lines = %d want 2 (no phantom empty line)", tb.lineCount())
	}
	if !tb.trailingNL {
		t.Fatal("trailingNL should be recorded")
	}
}

func TestCRLFNormalizedOnSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(path, []byte("a\r\nb\r\n"), 0o644)
	tb, _ := OpenTab(path)
	if err := tb.saveTo(path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "a\nb\n" {
		t.Fatalf("got %q want %q", got, "a\nb\n")
	}
}

// TestNonUTF8RoundTrip covers the silent corruption of files that are not
// valid UTF-8: the bytes must survive an open/save cycle untouched.
func TestNonUTF8RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latin1.txt")
	// "café" in Latin-1 plus a lone 0xFF, neither of which is valid UTF-8.
	content := []byte{'c', 'a', 'f', 0xE9, '\n', 0xFF, 0xFE, '\n'}
	os.WriteFile(path, content, 0o644)
	tb, err := OpenTab(path)
	if err != nil {
		t.Fatal(err)
	}
	if !tb.rawBytes {
		t.Error("tab should report that it holds non-UTF-8 bytes")
	}
	if err := tb.saveTo(path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(content) {
		t.Fatalf("got % x want % x", got, content)
	}
	// Editing around the raw bytes keeps them intact.
	tb.cur = Pos{0, 0}
	tb.insertRune('X')
	if err := tb.saveTo(path); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	want := append([]byte{'X'}, content...)
	if string(got) != string(want) {
		t.Fatalf("after edit got % x want % x", got, want)
	}
}

func TestBinaryFileRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.bin")
	os.WriteFile(path, []byte{'a', 0, 'b'}, 0o644)
	if _, err := OpenTab(path); err == nil {
		t.Fatal("a file with NUL bytes must not be opened for editing")
	}
}

func TestLargeFileRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse file: no real disk use, but Stat reports the size.
	if err := f.Truncate(maxOpenBytes + 1); err != nil {
		f.Close()
		t.Skipf("cannot create a sparse file here: %v", err)
	}
	f.Close()
	_, err = OpenTab(path)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v, want a size error", err)
	}
}

// TestSaveIsAtomic checks that saving does not truncate the original in place:
// no partially written file can be observed, and the mode is preserved.
func TestSaveAtomicPreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes differ on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), 0o755)
	tb, err := OpenTab(path)
	if err != nil {
		t.Fatal(err)
	}
	tb.cur = Pos{1, 0}
	tb.insertRune('#')
	if err := tb.saveTo(path); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v want 0755 (an executable must stay executable)", fi.Mode().Perm())
	}
	// No temp files left behind.
	des, _ := os.ReadDir(dir)
	for _, de := range des {
		if strings.HasPrefix(de.Name(), ".goat-") {
			t.Fatalf("temp file left behind: %s", de.Name())
		}
	}
}

func TestSaveFollowsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	os.WriteFile(real, []byte("hello\n"), 0o644)
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	tb, err := OpenTab(link)
	if err != nil {
		t.Fatal(err)
	}
	tb.cur = Pos{0, 5}
	tb.insertRune('!')
	if err := tb.saveTo(link); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("saving replaced the symlink with a regular file")
	}
	got, _ := os.ReadFile(real)
	if string(got) != "hello!\n" {
		t.Fatalf("target content %q", got)
	}
}

func TestExternalChangeDetected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(path, []byte("one\n"), 0o644)
	tb, err := OpenTab(path)
	if err != nil {
		t.Fatal(err)
	}
	if tb.externallyChanged() {
		t.Fatal("freshly opened file reported as changed")
	}
	// Someone else writes the file.
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(path, []byte("someone else\n"), 0o644)
	if !tb.externallyChanged() {
		t.Fatal("external modification not detected")
	}
	// Our own save clears the flag again.
	if err := tb.saveTo(path); err != nil {
		t.Fatal(err)
	}
	if tb.externallyChanged() {
		t.Fatal("our own save reported as an external change")
	}
}

func TestReadOnlyDetection(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("read-only permissions are not enforced for this user")
	}
	path := filepath.Join(t.TempDir(), "ro.txt")
	os.WriteFile(path, []byte("locked\n"), 0o444)
	tb, err := OpenTab(path)
	if err != nil {
		t.Fatal(err)
	}
	if !tb.readOnly {
		t.Fatal("a non-writable file should open read-only")
	}
	e := &Editor{tabs: []*Tab{tb}, cfg: DefaultConfig()}
	e.editAction(func() { tb.insertRune('x') })
	if tb.dirty {
		t.Fatal("a read-only buffer must not be modified")
	}
	if !strings.Contains(e.msg, "read-only") {
		t.Fatalf("expected a read-only message, got %q", e.msg)
	}
}

func TestEmergencySaveWritesDirtyBuffers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("one\n"), 0o644)
	tb, _ := OpenTab(path)
	tb.cur = Pos{0, 3}
	tb.insertRune('!')

	clean, _ := OpenTab(path)
	clean.dirty = false

	e := &Editor{tabs: []*Tab{tb, clean}, cfg: DefaultConfig()}
	saved := e.emergencySave()
	if len(saved) != 1 {
		t.Fatalf("saved %v, want just the dirty buffer", saved)
	}
	got, err := os.ReadFile(path + ".save")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "one!\n" {
		t.Fatalf("emergency save content %q", got)
	}
}

// statFile is a tiny indirection so tests can wait for a file to appear.
func statFile(path string) (os.FileInfo, error) { return os.Stat(path) }
