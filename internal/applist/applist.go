// Package applist reads and writes the list of tracked apps (apps.json).
package applist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// App is one tracked app or tool. The JSON field names are the ones the PowerShell module wrote,
// so an existing list keeps working.
type App struct {
	Name             string            `json:"Name"`
	Repository       string            `json:"Repository"`
	Type             string            `json:"Type,omitempty"` // "archive", "download", or empty for an installer app
	Destination      Strings           `json:"Destination,omitempty"`
	Folder           string            `json:"Folder,omitempty"` // downloads: where the release's archives are kept
	Subfolder        string            `json:"Subfolder,omitempty"`
	Pattern          string            `json:"Pattern,omitempty"`
	Include          Strings           `json:"Include,omitempty"`
	Keep             Strings           `json:"Keep,omitempty"`
	Rename           map[string]string `json:"Rename,omitempty"`
	SilentArgs       string            `json:"SilentArgs,omitempty"`
	Prerelease       bool              `json:"Prerelease,omitempty"`
	AllowUnverified  bool              `json:"AllowUnverified,omitempty"`
	HadDigest        bool              `json:"HadDigest,omitempty"`
	Publisher        string            `json:"Publisher,omitempty"` // who signed its installers, once one was signed
	InstalledVersion string            `json:"InstalledVersion,omitempty"`
}

// IsArchive reports whether the app is an archive tool whose files are copied into folders (--dest).
func (a *App) IsArchive() bool { return a.Type == "archive" }

// IsDownload reports whether the app is a downloaded archive kept up to date in its folder: each new
// version's archive is downloaded next to the old ones, and extracted if the old one was.
func (a *App) IsDownload() bool { return a.Type == "download" }

// IsInstaller reports whether the app is installed with an installer (.msi / .exe).
func (a *App) IsInstaller() bool { return !a.IsArchive() && !a.IsDownload() }

// Strings is a list that also reads a single string, which hand-edited lists often have.
type Strings []string

func (s *Strings) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*s = Strings{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

// DefaultPath is %APPDATA%\Reforge\apps.json on Windows, ~/Library/Application Support/Reforge/apps.json
// on macOS and ~/.config/Reforge/apps.json on Linux.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Reforge", "apps.json"), nil
}

// Load reads the list. A missing or empty file is an empty list.
func Load(path string) ([]*App, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))) // Windows PowerShell writes a BOM
	if len(data) == 0 {
		return nil, nil
	}
	var apps []*App
	if data[0] == '{' { // a list with one app, saved as a plain object
		var app App
		err = json.Unmarshal(data, &app)
		apps = []*App{&app}
	} else {
		err = json.Unmarshal(data, &apps)
	}
	if err != nil {
		return nil, fmt.Errorf("can't read the app list %s: %w", path, err)
	}
	return apps, nil
}

// Save writes the list through a temp file, so a crash or a full disk never leaves it half-written.
func Save(path string, apps []*App) error {
	if apps == nil {
		apps = []*App{}
	}
	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// Find returns the app with this exact name (ignoring case), or nil.
func Find(apps []*App, name string) *App {
	for _, a := range apps {
		if strings.EqualFold(a.Name, name) {
			return a
		}
	}
	return nil
}
