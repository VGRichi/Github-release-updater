package core

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	if !isDir(app.Folder) {
		r.set(Skipped, "Folder not found: "+app.Folder)
		return nil
	}
	if app.InstalledVersion == release.Tag || version.Same(app.InstalledVersion, release.Tag) {
		r.Status = UpToDate
		return nil
	}
	asset := pick.Archive(release.Assets, app.Pattern, e.Arch)
	if asset == nil {
		r.set(Skipped, "No zip/7z/tar.gz file in this release")
		return nil
	}
	if !e.offer(app, asset, r) || !e.approved(app, asset, r) {
		return nil
	}

	file, cleanup, err := e.fetch(asset)
	if err != nil {
		return err
	}
	defer cleanup()
	note := "Downloaded " + asset.Name
	// Extract first, so a broken archive changes nothing in the folder
	if old := ExtractedFolder(app.Folder, app.Repository, app.InstalledVersion); old != "" {
		name, err := e.replaceExtracted(app, asset, file, old)
		if err != nil {
			return err
		}
		note += ", extracted to " + name
	}
	if err := archive.Move(file, filepath.Join(app.Folder, asset.Name)); err != nil {
		return err
	}
	app.InstalledVersion = release.Tag
	r.Installed = release.Tag
	if asset.Digest != "" {
		note += ". Checksum verified"
	}
	r.set(Updated, note)
	return nil
}

// replaceExtracted extracts the new archive next to the old version's extracted folder, which moves to
// the backups (put back if the new one can't be moved in). It returns the new folder's name.
func (e *Engine) replaceExtracted(app *applist.App, asset *github.Asset, file, old string) (string, error) {
	extracted, err := e.extract(asset, file)
	if err != nil {
		return "", err
	}
	// An archive with one folder inside (dxvk-3.1.2/) keeps its name; otherwise it's named after the archive
	src, name := extracted, archiveBase(asset.Name)
	if entries, _ := os.ReadDir(extracted); len(entries) == 1 && entries[0].IsDir() {
		src, name = filepath.Join(extracted, entries[0].Name()), entries[0].Name()
	}
	newFolder := filepath.Join(app.Folder, name)
	if newFolder != old && isDir(newFolder) {
		return "", fmt.Errorf("%s already exists; move it away and try again", newFolder)
	}
	// The old folder goes to the backups first, which also makes room if both have the same name
	backups, allBackups := e.backupFolder(app)
	backup := filepath.Join(backups, filepath.Base(old))
	if err := os.MkdirAll(backups, 0o755); err != nil {
		return "", err
	}
	if err := archive.Move(old, backup); err != nil {
		return "", fmt.Errorf("couldn't move the old folder %s to the backups (is something using it?): %v", old, err)
	}
	if err := archive.Move(src, newFolder); err != nil {
		archive.Move(backup, old)
		return "", err
	}
	e.verbose("The old folder was moved to %s", backup)
	archive.PruneBackups(allBackups, e.BackupsToKeep)
	return name, nil
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
			best, found = v, version.Text(e.Name())
		}
	}
	return found
}

// FindDownloaded looks through the folders, two levels deep, for a repository's archives or extracted
// folders, and returns the folder with the newest version, and that version. It returns "" with none.
func FindDownloaded(folders []string, repo string) (folder, ver string) {
	var best version.Version
	eachFolder(folders, func(dir string) {
		if found := DetectVersion(dir, repo); found != "" {
			if v := version.Parse(found); best == nil || version.Compare(v, best) > 0 {
				best, folder, ver = v, dir, found
			}
		}
	})
	return folder, ver
}

// eachFolder calls visit for each folder and its subfolders two levels deep, like Downloads\Mods\Game.
func eachFolder(roots []string, visit func(dir string)) {
	for _, root := range roots {
		filepath.WalkDir(root, func(dir string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if rel, _ := filepath.Rel(root, dir); rel != "." && strings.Count(rel, string(filepath.Separator)) >= 2 {
				return filepath.SkipDir
			}
			visit(dir)
			return nil
		})
	}
}

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
