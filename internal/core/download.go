package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/archive"
	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/pick"
	"github.com/VGRichi/Reforge/internal/version"
)

// updateDownload keeps a downloaded archive (like OptiScaler's .7z in Downloads\Mods) up to date: the
// new version's archive is downloaded into the same folder, next to the old ones. If the old version
// was extracted there (a folder like dxvk-3.1.1), the new one is extracted too, and the old folder is
// moved to the backups, since you may have changed files in it.
func (e *Engine) updateDownload(app *applist.App, release *github.Release, r *Result) error {
	r.Installed = app.InstalledVersion
	if info, err := os.Stat(app.Folder); err != nil || !info.IsDir() {
		r.Status, r.Note = Skipped, "Folder not found: "+app.Folder
		return nil
	}
	if app.InstalledVersion == release.Tag || version.Same(app.InstalledVersion, release.Tag) {
		r.Status = UpToDate
		return nil
	}
	asset := pick.Archive(release.Assets, app.Pattern, e.Arch)
	if asset == nil {
		r.Status, r.Note = Skipped, "No zip/7z/tar.gz file in this release"
		return nil
	}
	if asset.Digest != "" {
		app.HadDigest = true
	}
	r.Status, r.Note = UpdateAvailable, verificationNote(app, asset)
	if e.DryRun {
		return nil
	}
	if !e.approve(app, asset) {
		r.Status, r.Note = Skipped, "Unverified download declined"
		return nil
	}

	work, err := os.MkdirTemp(e.WorkRoot, "reforge-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	e.progress("Downloading %s (%s)", asset.Name, size(asset.Size))
	file, err := e.GitHub.Download(*asset, work)
	if err != nil {
		return err
	}

	// Extract first, so a broken archive changes nothing in the folder
	oldFolder := ExtractedFolder(app.Folder, app.Repository, app.InstalledVersion)
	var newFolder string
	if oldFolder != "" {
		e.progress("Extracting %s", asset.Name)
		extracted := filepath.Join(work, "extracted")
		if err := archive.Extract(file, extracted); err != nil {
			return err
		}
		// An archive with one folder inside (dxvk-3.1.2/) keeps its name; otherwise it's named after the archive
		src, name := extracted, archiveBase(asset.Name)
		if entries, _ := os.ReadDir(extracted); len(entries) == 1 && entries[0].IsDir() {
			src, name = filepath.Join(extracted, entries[0].Name()), entries[0].Name()
		}
		newFolder = filepath.Join(app.Folder, name)
		if _, err := os.Stat(newFolder); err == nil && newFolder != oldFolder {
			return fmt.Errorf("%s already exists; move it away and try again", newFolder)
		}
		// The old folder goes to the backups first, which also makes room if both have the same name
		appBackups := filepath.Join(e.BackupRoot, safeName(app.Name))
		backup := filepath.Join(appBackups, e.now().Format(archive.BackupFolderName), filepath.Base(oldFolder))
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := archive.Move(oldFolder, backup); err != nil {
			return fmt.Errorf("couldn't move the old folder %s to the backups (is something using it?): %v", oldFolder, err)
		}
		if err := archive.Move(src, newFolder); err != nil {
			archive.Move(backup, oldFolder) // put the old version back
			return err
		}
		e.verbose("The old folder was moved to %s", backup)
		archive.PruneBackups(appBackups, e.BackupsToKeep)
	}

	target := filepath.Join(app.Folder, asset.Name)
	if err := archive.Move(file, target); err != nil {
		return err
	}
	app.InstalledVersion = release.Tag
	r.Installed, r.Status = release.Tag, Updated
	r.Note = "Downloaded " + asset.Name
	if newFolder != "" {
		r.Note += ", extracted to " + filepath.Base(newFolder)
	}
	if asset.Digest != "" {
		r.Note += ". Checksum verified"
	}
	return nil
}

// ExtractedFolder finds the folder in dir that an archive of this version was extracted into: its
// name has the repository's name and the version in it, like dxvk-3.1.1 for doitsujin/dxvk 3.1.1.
func ExtractedFolder(dir, repo, ver string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() && namedLike(e.Name(), repo) && version.Same(e.Name(), ver) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// DetectVersion finds the newest version of a repository's archives or extracted folders in dir, from
// their names (Optiscaler_0.9.4.7z, dxvk-3.1.1). It returns "" when there are none.
func DetectVersion(dir, repo string) string {
	entries, _ := os.ReadDir(dir)
	var best version.Version
	found := ""
	for _, e := range entries {
		if !namedLike(e.Name(), repo) || (!e.IsDir() && !pick.IsArchive(e.Name())) {
			continue
		}
		if v := version.Parse(e.Name()); v != nil && (best == nil || version.Compare(v, best) > 0) {
			best, found = v, versionText(e.Name())
		}
	}
	return found
}

// namedLike reports whether a file or folder name contains the repository's name, ignoring case and
// punctuation ("Optiscaler_0.9.4" for optiscaler/OptiScaler).
func namedLike(name, repo string) bool {
	clean := func(s string) string { return strings.ToLower(notAlphanumeric.ReplaceAllString(s, "")) }
	leaf := clean(repoLeaf(repo))
	return leaf != "" && strings.Contains(clean(name), leaf)
}

var versionNumber = regexp.MustCompile(`\d+(?:\.\d+){0,3}`)

// versionText is the version number in a name, as written ("3.1.1" in dxvk-3.1.1).
func versionText(name string) string { return versionNumber.FindString(name) }

// archiveBase is an archive's name without its extension: tool-1.2 for tool-1.2.tar.gz.
func archiveBase(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tgz", ".zip", ".7z"} {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}
