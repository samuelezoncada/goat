package editor

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestHintBarMatchesBindings keeps the bottom hint bar from advertising a key
// that is not in the binding table (and therefore not in the help page).
func TestHintBarMatchesBindings(t *testing.T) {
	keys := bindingKeys()
	for _, h := range hintBar {
		if !keys[h.keys] {
			t.Errorf("hint %q is not documented in the binding table", h.keys)
		}
		if h.label == "" {
			t.Errorf("hint %q has no label", h.keys)
		}
	}
}

func TestHelpSectionsComeFromBindings(t *testing.T) {
	secs := bindingSections()
	if len(secs) < 5 {
		t.Fatalf("only %d help sections", len(secs))
	}
	total := 0
	for _, s := range secs {
		if s.title == "" {
			t.Error("a help section has no title")
		}
		for _, r := range s.rows {
			if r.keys == "" || r.desc == "" {
				t.Errorf("incomplete help row in %q: %+v", s.title, r)
			}
			total++
		}
	}
	if total != len(bindings) {
		t.Fatalf("help shows %d rows for %d bindings", total, len(bindings))
	}
}

// TestREADMEDocumentsEveryBinding is the drift guard: every key in the table
// must appear in the README's keybinding documentation.
func TestREADMEDocumentsEveryBinding(t *testing.T) {
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Skipf("README not readable: %v", err)
	}
	readme := string(data)
	// Compare on the individual keys, since the README groups them freely.
	splitter := regexp.MustCompile(`\s*/\s*`)
	for _, b := range bindings {
		for _, key := range splitter.Split(b.keys, -1) {
			key = strings.TrimSpace(key)
			if key == "" || key == "a-z" {
				continue
			}
			if !strings.Contains(readme, key) {
				t.Errorf("README does not mention %q (from the %q section)", key, b.section)
			}
		}
	}
}

func TestNoDuplicateBindingRows(t *testing.T) {
	seen := map[string]string{}
	for _, b := range bindings {
		k := b.section + "|" + b.keys
		if prev, ok := seen[k]; ok {
			t.Errorf("duplicate binding row %q (%s and %s)", k, prev, b.desc)
		}
		seen[k] = b.desc
	}
}
