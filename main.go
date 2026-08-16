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
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] [file...]\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), banner)
		flag.PrintDefaults()
	}
	printVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *printVersion {
		fmt.Printf("goat %s\n", version)
		return
	}

	if flag.NArg() == 0 {
		// Open the current directory's file list in the browser.
		run(nil)
		return
	}
	run(flag.Args())
}

func run(files []string) {
	e, err := editor.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "goat: %v\n", err)
		os.Exit(1)
	}
	defer e.Close()

	for _, f := range files {
		if fi, err := os.Stat(f); err == nil && fi.IsDir() {
			e.SetRoot(f)
			e.OpenDir(f)
		} else {
			e.OpenPath(f)
		}
	}
	if !e.HasTabs() {
		e.NewTab()
	}
	e.Run()
}
