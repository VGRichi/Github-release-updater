package system

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// These use the real Windows APIs.

func TestProgramsReadsInstalledApps(t *testing.T) {
	programs, err := New().Programs()
	if err != nil || len(programs) == 0 {
		t.Fatalf("got %d programs, %v", len(programs), err)
	}
	for _, p := range programs {
		if p.Name == "" || !strings.Contains(p.Key, `\Uninstall\`) {
			t.Fatalf("bad entry: %+v", p)
		}
	}
}

func TestDownloadOriginReadsTheMarkOfTheWeb(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tool.zip")
	os.WriteFile(file, []byte("x"), 0o644)
	if got := New().DownloadOrigin(file); got != "" {
		t.Fatalf("a file without a note: %q", got)
	}
	note := "[ZoneTransfer]\r\nZoneId=3\r\nReferrerUrl=https://github.com/o/r/releases/tag/v1\r\n"
	if err := os.WriteFile(file+":Zone.Identifier", []byte(note), 0o644); err != nil {
		t.Skip("this drive has no alternate data streams:", err)
	}
	if got := New().DownloadOrigin(file); got != note {
		t.Fatalf("got %q", got)
	}
}

func TestRunInstallerReturnsTheExitCode(t *testing.T) {
	code, err := New().RunInstaller(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), "/c exit 7")
	if err != nil || code != 7 {
		t.Fatalf("got %d, %v", code, err)
	}
}

func TestRunInstallerDoesNotWaitForProgramsItStarts(t *testing.T) {
	// The "installer" starts a long-running program (like an app opened after setup) and exits
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := `$c = Start-Process powershell.exe -ArgumentList '-NoProfile','-Command','Start-Sleep 60' -WindowStyle Hidden -PassThru; Set-Content -LiteralPath '` + pidFile + `' $c.Id; exit 3`
	start := time.Now()
	code, err := New().RunInstaller("powershell.exe", `-NoProfile -WindowStyle Hidden -Command "`+script+`"`)
	if data, readErr := os.ReadFile(pidFile); readErr == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if p, findErr := os.FindProcess(pid); findErr == nil {
			p.Kill()
		}
	}
	if err != nil || code != 3 {
		t.Fatalf("got %d, %v", code, err)
	}
	if time.Since(start) > 45*time.Second {
		t.Fatal("waited for the program the installer started")
	}
}

func TestDownloadsFolder(t *testing.T) {
	if dir := DownloadsFolder(); dir == "" || !filepath.IsAbs(dir) {
		t.Fatalf("got %q", dir)
	}
}
