package editor

import (
	"strings"
	"testing"
)

func TestConfigWarningsReportedAfterOpening(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Path = "/home/u/.config/goat/config"
	cfg.Warnings = []string{"line 4: unknown setting \"nonsense\"", "line 5: tabwidth must be 1..32"}

	// A broken config outranks the informational message left by opening files.
	e := &Editor{cfg: cfg}
	e.statusf("Read 3 lines")
	e.reportConfigWarnings()
	if !strings.Contains(e.msg, "unknown setting") || !strings.Contains(e.msg, "+1 more") {
		t.Fatalf("warning not surfaced: %q", e.msg)
	}

	// But it must never bury a failure.
	e2 := &Editor{cfg: cfg}
	e2.errorf("Error reading a.bin: binary file")
	e2.reportConfigWarnings()
	if !strings.Contains(e2.msg, "binary file") {
		t.Fatalf("a real error was replaced by a config warning: %q", e2.msg)
	}

	// A single warning is reported without the counter.
	cfg2 := DefaultConfig()
	cfg2.Path = "/c/config"
	cfg2.Warnings = []string{"line 1: bad"}
	e3 := &Editor{cfg: cfg2}
	e3.reportConfigWarnings()
	if strings.Contains(e3.msg, "more") {
		t.Fatalf("single warning message %q", e3.msg)
	}
}
