//go:build !windows

package main

import "os"

// terminalColors reports whether f is a terminal that shows colours.
func terminalColors(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// startedByDoubleClick: other systems don't open a terminal for a double-clicked program like Windows does.
func startedByDoubleClick() bool { return false }

func runShell() bool { return false }
