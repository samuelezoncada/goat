package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestPromptCancelCallbackRuns(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	cancelled := false
	e.beginPromptCancel("Save? ", "", nil, func(string) {}, func() { cancelled = true })
	e.promptKey(keyEvent(tcellKeyEsc))
	if !cancelled {
		t.Fatal("dismissing a prompt must notify the caller")
	}
	if e.prompt != nil || e.mode != ModeNormal {
		t.Fatal("prompt should be closed")
	}
}

func TestPromptHistory(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.beginPrompt("Search: ", "", nil, func(string) {})
	e.prompt.input = []rune("first")
	e.promptKey(keyEvent(tcellKeyEnter))
	e.beginPrompt("Search: ", "", nil, func(string) {})
	e.prompt.input = []rune("second")
	e.promptKey(keyEvent(tcellKeyEnter))

	e.beginPrompt("Search: ", "", nil, func(string) {})
	e.promptKey(keyEvent(tcell.KeyUp))
	if got := string(e.prompt.input); got != "second" {
		t.Fatalf("first Up = %q want second", got)
	}
	e.promptKey(keyEvent(tcell.KeyUp))
	if got := string(e.prompt.input); got != "first" {
		t.Fatalf("second Up = %q want first", got)
	}
	e.promptKey(keyEvent(tcell.KeyDown))
	e.promptKey(keyEvent(tcell.KeyDown))
	if got := string(e.prompt.input); got != "" {
		t.Fatalf("Down back to the live input = %q", got)
	}
}

func TestPromptHistoryDedupes(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	for i := 0; i < 3; i++ {
		e.beginPrompt("Search: ", "", nil, func(string) {})
		e.prompt.input = []rune("same")
		e.promptKey(keyEvent(tcellKeyEnter))
	}
	if got := len(e.history["search"]); got != 1 {
		t.Fatalf("history has %d entries want 1", got)
	}
}

func TestPromptFilenameCompletion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "alpha.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "alphabet.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "zulu.txt"), nil, 0o644)

	e := &Editor{cfg: DefaultConfig()}
	e.beginPrompt("File to read: ", "", nil, func(string) {})
	prefix := filepath.Join(dir, "z")
	e.setPromptText(e.prompt, prefix)
	e.promptKey(keyEvent(tcell.KeyTab))
	if got := string(e.prompt.input); got != filepath.Join(dir, "zulu.txt") {
		t.Fatalf("unique completion = %q", got)
	}

	// A shared prefix completes as far as it is unambiguous, then cycles.
	e.setPromptText(e.prompt, filepath.Join(dir, "al"))
	e.promptKey(keyEvent(tcell.KeyTab))
	if got := string(e.prompt.input); got != filepath.Join(dir, "alpha") {
		t.Fatalf("common prefix = %q", got)
	}
	e.promptKey(keyEvent(tcell.KeyTab))
	first := string(e.prompt.input)
	e.promptKey(keyEvent(tcell.KeyTab))
	second := string(e.prompt.input)
	if first == second {
		t.Fatalf("Tab should cycle candidates, stuck at %q", first)
	}
}

func TestPromptStackPreservesOuterPrompt(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.beginPrompt("Replace Where Is: ", "", nil, func(string) {})
	e.beginPrompt("Replace with: ", "", nil, func(string) {})
	if len(e.promptStack) != 1 {
		t.Fatalf("stack = %d, the outer prompt should be remembered", len(e.promptStack))
	}
	if e.prompt.label != "Replace with: " {
		t.Fatalf("active prompt %q", e.prompt.label)
	}
}

func TestPromptCtrlUClears(t *testing.T) {
	e := &Editor{cfg: DefaultConfig()}
	e.beginPrompt("Search: ", "hello", nil, func(string) {})
	e.promptKey(keyEvent(tcell.KeyCtrlU))
	if len(e.prompt.input) != 0 || e.prompt.pos != 0 {
		t.Fatalf("input %q pos %d", string(e.prompt.input), e.prompt.pos)
	}
}
