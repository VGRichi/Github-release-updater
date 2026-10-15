package core

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/pick"
	"github.com/VGRichi/Reforge/internal/system"
)

// NotInstalledError is Prepare's answer for an app whose release has an installer but that isn't
// installed: it can be installed first (Engine.Install).
type NotInstalledError struct{ Name string }

func (e *NotInstalledError) Error() string { return e.Name + " isn't installed" }

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
		name = path.Base(app.Repository)
	}
	if pick.Installer(release.Assets, app.Pattern, true, e.Arch) != nil {
		programs, err := e.installedPrograms()
		if err != nil {
			return err
		}
		p := installedFrom(programs, app.Name, app.Repository)
		if p == nil {
			return &NotInstalledError{name}
		}
		if app.Name == "" {
			app.Name = BaseName(p.Name)
		}
		return nil
	}
	if pick.Archive(release.Assets, app.Pattern, e.Arch) == nil {
		return errors.New("its latest release has no Windows installer and no .zip, .7z or .tar.gz file")
	}
	app.Type, app.Name = "download", name
	app.Folder, app.InstalledVersion = FindDownloaded(folders, app.Repository)
	if app.Folder == "" && len(folders) > 0 {
		app.Folder = folders[0]
	}
	return nil
}

// installedFrom finds the installed app a repository makes. A name you gave matches loosely, like it
// does for updates ("a part is fine"). Otherwise it's a link to the repository, or the app's first word
// matching the repository's name: in full ("GridCraft" for gridcraft), or starting it with 4 letters or
// more ("Notepad++" for notepad-plus-plus), so git isn't "GitHub Desktop".
func installedFrom(programs []system.Program, givenName, repo string) *system.Program {
	if givenName != "" {
		return FindProgram(programs, givenName)
	}
	for i := range programs {
		if strings.EqualFold(repoFromLinks(programs[i].Links), repo) {
			return &programs[i]
		}
	}
	leaf := simple(path.Base(repo))
	for i := range programs {
		if word := firstWord(programs[i].Name); word == leaf || (len(word) >= 4 && strings.HasPrefix(leaf, word)) {
			return &programs[i]
		}
	}
	return nil
}
