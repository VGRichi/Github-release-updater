// Command fakeinstaller stands in for an installer in the Windows tests. It's built as a windowed
// program, like real installers, so it never gets console events.
//
//	fakeinstaller exit <code>        exits with that code
//	fakeinstaller app <pid file>     starts a program that keeps running (like an app opened after
//	                                 setup), writes its process ID to the file, and exits with code 3
package main

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

func main() {
	switch os.Args[1] {
	case "exit":
		code, _ := strconv.Atoi(os.Args[2])
		os.Exit(code)
	case "app":
		app := exec.Command(os.Args[0], "sleep")
		if err := app.Start(); err != nil {
			os.Exit(1)
		}
		os.WriteFile(os.Args[2], []byte(strconv.Itoa(app.Process.Pid)), 0o644)
		os.Exit(3)
	case "sleep":
		time.Sleep(60 * time.Second)
	}
}
