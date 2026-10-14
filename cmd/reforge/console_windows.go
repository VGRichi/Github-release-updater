package main

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// terminalColors reports whether f is a console that shows colours, turning them on if needed.
func terminalColors(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	h := windows.Handle(f.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return false
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}

var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// startedByDoubleClick reports whether Reforge has a console window to itself, which is what happens
// when it's opened from Explorer. From a terminal, the shell shares the console.
func startedByDoubleClick() bool {
	var pids [2]uint32
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}
