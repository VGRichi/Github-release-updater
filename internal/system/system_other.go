//go:build !windows

package system

import (
	"errors"
	"os"
	"path/filepath"
)

// macOS and Linux: archive tools work; installed apps, installers and download notes come later.
type otherSystem struct{}

// New returns the System for this OS.
func New() System { return otherSystem{} }

func (otherSystem) Programs() ([]Program, error) { return nil, nil }

func (otherSystem) RunInstaller(file, args string) (int, error) {
	return 0, errors.New("installer apps aren't supported on this OS yet; archive tools are")
}

func (otherSystem) IsAdmin() bool { return os.Geteuid() == 0 }

// DownloadsFolder is ~/Downloads.
func DownloadsFolder() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

func (otherSystem) DownloadOrigin(file string) string { return "" }

func (otherSystem) WingetPackages() []Package { return nil }
