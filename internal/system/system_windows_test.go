package system

import (
	"os"
	"os/exec"
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

// fakeInstaller builds testdata/fakeinstaller: a windowed program like a real installer. Console
// programs (cmd, PowerShell) made these tests flaky: Windows can send their window a close event.
func fakeInstaller(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "fakeinstaller.exe")
	if out, err := exec.Command("go", "build", "-ldflags=-H=windowsgui", "-o", exe, "./testdata/fakeinstaller").CombinedOutput(); err != nil {
		t.Fatalf("building the fake installer: %v\n%s", err, out)
	}
	return exe
}

func TestRunInstallerReturnsTheExitCode(t *testing.T) {
	code, err := New().RunInstaller(fakeInstaller(t), "exit 7")
	if err != nil || code != 7 {
		t.Fatalf("got %d, %v", code, err)
	}
}

func TestRunInstallerDoesNotWaitForProgramsItStarts(t *testing.T) {
	// The "installer" starts a long-running program (like an app opened after setup) and exits
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	installer := fakeInstaller(t)
	start := time.Now()
	code, err := New().RunInstaller(installer, `app "`+pidFile+`"`)
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

func TestSigner(t *testing.T) {
	// PowerShell 7 is installed on GitHub's Windows machines, and signed by Microsoft
	signed, err := exec.LookPath("pwsh.exe")
	if err != nil {
		t.Skip("pwsh.exe isn't installed")
	}
	if name, err := New().Signer(signed); err != nil || name != "Microsoft Corporation" {
		t.Errorf("signed file: %q, %v", name, err)
	}

	unsigned := filepath.Join(t.TempDir(), "unsigned.exe")
	os.WriteFile(unsigned, []byte("MZ not signed"), 0o644)
	if name, err := New().Signer(unsigned); err != nil || name != "" {
		t.Errorf("unsigned file: %q, %v", name, err)
	}

	// Changed after signing, like a tampered download
	data, _ := os.ReadFile(signed)
	data[len(data)/2]++
	tampered := filepath.Join(t.TempDir(), "tampered.exe")
	os.WriteFile(tampered, data, 0o644)
	if name, err := New().Signer(tampered); err == nil || !strings.Contains(err.Error(), "isn't valid") {
		t.Errorf("tampered file: %q, %v", name, err)
	}
}

func TestDownloadsFolder(t *testing.T) {
	if dir := DownloadsFolder(); dir == "" || !filepath.IsAbs(dir) {
		t.Fatalf("got %q", dir)
	}
}
