package main

import (
	"os"
	"os/exec"
	"path/filepath"
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

// runShell turns the window a double-click opened into a PowerShell prompt (PowerShell 7 if installed),
// with Reforge's folder on its PATH so 'reforge' commands work there, and waits until it's closed.
// It reports false if PowerShell couldn't start.
func runShell() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	shell, err := exec.LookPath("pwsh.exe")
	if err != nil {
		shell = "powershell.exe"
	}
	cmd := exec.Command(shell, "-NoLogo")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(exe)+";"+os.Getenv("PATH"))
	cmd.Dir, _ = os.UserHomeDir()
	if cmd.Start() != nil {
		return false
	}
	cmd.Wait()
	return true
}

var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// startedByDoubleClick reports whether Reforge has a console window to itself, which is what happens
// when it's opened from Explorer. From a terminal, the shell shares the console.
func startedByDoubleClick() bool {
	var pids [2]uint32
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}
