package core

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/pick"
	"github.com/VGRichi/Reforge/internal/system"
	"github.com/VGRichi/Reforge/internal/version"
)

// Prepare works out how a new app is kept up to date, from what's installed and what its latest
// release has:
//   - an installed app whose release has an installer is updated with that installer, and named the
//     way Installed apps shows it;
//   - an app with an installer that isn't installed yet returns a NotInstalledError, so it can be
//     installed first;
//   - a release of archives only (like DXVK or OptiScaler) is a download, kept up to date in the folder
//     where you have it: the folders are searched two levels deep for its archives or extracted
//     folders, which also tell the version you have. With none, it goes in the first folder.
func (e *Engine) Prepare(app *applist.App, folders []string) error {
	release, err := e.GitHub.Latest(app.Repository, app.Prerelease)
	if err != nil {
		return err
	}
	if release == nil {
		return fmt.Errorf("%s has no releases on GitHub", app.Repository)
	}
	name := app.Name
	if name == "" {
		name = repoLeaf(app.Repository)
	}
	installer := pick.Installer(release.Assets, app.Pattern, true, e.Arch)
	if installer != nil {
		programs, err := e.installedPrograms()
		if err != nil {
			return err
		}
		if p := installedFrom(programs, app.Name, app.Repository); p != nil {
			if app.Name == "" {
				app.Name = BaseName(p.Name)
			}
			return nil
		}
		return &NotInstalledError{name}
	}
	if pick.Archive(release.Assets, app.Pattern, e.Arch) != nil {
		app.Type, app.Name = "download", name
		app.Folder, app.InstalledVersion = FindDownloaded(folders, app.Repository)
		if app.Folder == "" && len(folders) > 0 {
			app.Folder = folders[0]
		}
		return nil
	}
	return errors.New("its latest release has no Windows installer and no .zip, .7z or .tar.gz file")
}

// installedFrom finds the installed app a repository makes. A name you gave matches loosely, like it
// does for updates ("a part is fine"). Otherwise it's a link to the repository, or the app's first word
// matching the repository's name: in full ("GridCraft" for gridcraft), or starting it with 4 letters or
// more ("Notepad++" for notepad-plus-plus), so git isn't "GitHub Desktop".
func installedFrom(programs []system.Program, givenName, repo string) *system.Program {
	if givenName != "" {
		return FindProgram(programs, givenName)
	}
	clean := func(s string) string { return strings.ToLower(notAlphanumeric.ReplaceAllString(s, "")) }
	leaf := clean(repoLeaf(repo))
	link := "github.com/" + strings.ToLower(repo)
	for i := range programs {
		for _, l := range programs[i].Links {
			if l = strings.ToLower(strings.TrimSuffix(l, ".git")); strings.HasSuffix(strings.TrimRight(l, "/"), link) || strings.Contains(l, link+"/") {
				return &programs[i]
			}
		}
	}
	for i := range programs {
		fields := strings.Fields(programs[i].Name)
		if len(fields) == 0 {
			continue
		}
		if word := clean(fields[0]); word == leaf || (len(word) >= 4 && strings.HasPrefix(leaf, word)) {
			return &programs[i]
		}
	}
	return nil
}

// FindDownloaded looks through the folders, two levels deep, for a repository's archives or extracted
// folders (Optiscaler_0.9.4.7z, dxvk-3.1.1), and returns the folder with the newest version, and that
// version. It returns "" when there are none.
func FindDownloaded(folders []string, repo string) (folder, ver string) {
	var best version.Version
	for _, root := range folders {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= 2 {
				return filepath.SkipDir
			}
			if found := DetectVersion(path, repo); found != "" {
				if v := version.Parse(found); best == nil || version.Compare(v, best) > 0 {
					best, folder, ver = v, path, found
				}
			}
			return nil
		})
	}
	return folder, ver
}

// NotInstalledError is Prepare's answer for an app whose release has an installer but that isn't
// installed: it can be installed first (Engine.Install).
type NotInstalledError struct{ Name string }

func (e *NotInstalledError) Error() string { return e.Name + " isn't installed" }
