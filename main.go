package main

import (
	"flag"
	"fmt"
	"os"

	"goat/editor"
)

const banner = `goat - a nano-inspired terminal text editor`

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: %s [options] [file...]\n", os.Args[0])
		fmt.Fprintln(out, banner)
		fmt.Fprintln(out)
		flag.PrintDefaults()
		fmt.Fprintf(out, "\nConfig file: %s\n", editor.ConfigPath())
	}
	printVersion := flag.Bool("version", false, "print version and exit")
	readOnly := flag.Bool("view", false, "open files read-only")
	flag.Parse()

	if *printVersion {
		fmt.Printf("goat %s\n", version)
		return 0
	}
	return start(flag.Args(), *readOnly)
}

func start(files []string, readOnly bool) int {
	cfg := editor.LoadConfig()
	e, err := editor.NewWithConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "goat: %v\n", err)
		return 1
	}
	// Editor.Run closes the editor itself, including on a panic; Close is
	// idempotent, so this covers the paths that return before Run.
	defer e.Close()

	for _, f := range files {
		if fi, err := os.Stat(f); err == nil && fi.IsDir() {
			e.SetRoot(f)
			e.OpenDir(f)
			continue
		}
		e.OpenPath(f)
	}
	if !e.HasTabs() {
		e.NewTab()
	}
	if readOnly {
		e.SetReadOnly(true)
	}
	if len(files) == 0 {
		e.OpenBrowser()
	}
	if err := e.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "goat: %v\n", err)
		return 2
	}
	return 0
}
