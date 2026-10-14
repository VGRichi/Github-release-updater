// Package system is everything that depends on the operating system: installed apps, running
// installers, admin rights, download notes and winget. The rest of Reforge only sees the System interface.
package system

import (
	"bufio"
	"strings"
)

// Program is an app in the OS's list of installed apps (on Windows: Settings > Apps > Installed apps).
type Program struct {
	Name    string
	Version string
	Key     string   // identifies the entry, so a before/after comparison can spot new ones
	PerUser bool     // installed for the current user only, so updating needs no admin rights
	MSI     bool     // installed with an .msi
	Links   []string // website, update and help links from its uninstall information
}

// Package is an app winget manages.
type Package struct{ Name, Version string }

// System is what Reforge needs from the operating system.
type System interface {
	Programs() ([]Program, error)
	// RunInstaller runs an installer with silent arguments and waits for it (not for programs it starts).
	RunInstaller(file, args string) (exitCode int, err error)
	// Signer checks a file's digital signature: who signed it, "" when it isn't signed, and an error
	// when the signature is broken or untrusted (like a file changed after it was signed).
	Signer(file string) (string, error)
	IsAdmin() bool
	// DownloadOrigin is the note browsers add to downloads saying where they came from, or "".
	DownloadOrigin(file string) string
	// WingetPackages are the apps winget can update; nil without winget.
	WingetPackages() []Package
}

// parseWingetList reads `winget list` output: a header row, a row of dashes, then one app per row.
// Columns start where the header's words start; the headers are translated, so only positions are used.
// Long names are cut off with "…".
func parseWingetList(out string) []Package {
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.LastIndex(line, "\r"); i >= 0 { // progress spinners overwrite the line
			line = line[i+1:]
		}
		lines = append(lines, line)
	}
	for i := 1; i < len(lines); i++ {
		dashes := strings.TrimSpace(lines[i])
		if len(dashes) < 10 || strings.Trim(dashes, "-") != "" {
			continue
		}
		starts := columnStarts([]rune(lines[i-1]))
		if len(starts) < 3 {
			return nil
		}
		var pkgs []Package
		for _, line := range lines[i+1:] {
			row := []rune(line)
			name := strings.TrimSuffix(strings.TrimSpace(cut(row, 0, starts[1])), "…")
			ver := strings.TrimSpace(cut(row, starts[2], end(starts, 3, len(row))))
			if name != "" {
				pkgs = append(pkgs, Package{Name: name, Version: ver})
			}
		}
		return pkgs
	}
	return nil
}

func columnStarts(header []rune) []int {
	var starts []int
	for i, r := range header {
		if r != ' ' && (i == 0 || header[i-1] == ' ') {
			starts = append(starts, i)
		}
	}
	return starts
}

func end(starts []int, i, fallback int) int {
	if i < len(starts) {
		return starts[i]
	}
	return fallback
}

func cut(row []rune, from, to int) string {
	if from >= len(row) {
		return ""
	}
	return string(row[from:min(to, len(row))])
}
