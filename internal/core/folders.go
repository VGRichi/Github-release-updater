package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/archive"
	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/pick"
)

// updateFolders updates a tool whose files are copied into folders (--dest, like a game's): the
// release's archive is extracted and copied into each one, with replaced files backed up. It's all or
// nothing: if any file can't be copied (often one in use by a running game), everything is put back.
func (e *Engine) updateFolders(app *applist.App, release *github.Release, r *Result) error {
	r.Installed = app.InstalledVersion
	var dests, missing []string
	for _, d := range app.Destination {
		if d == "" {
			continue
		}
		dests = append(dests, d)
		if !isDir(d) {
			missing = append(missing, d)
		}
	}
	switch {
	case len(dests) == 0:
		r.set(Skipped, "No destination folder set")
		return nil
	case len(missing) > 0:
		r.set(Skipped, "Folder not found: "+strings.Join(missing, ", "))
		return nil
	case sameVersion(app.InstalledVersion, release.Tag):
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
	extracted, err := e.extract(asset, file)
	if err != nil {
		return err
	}
	src, err := archive.ContentRoot(extracted, app.Subfolder)
	if err != nil {
		return err
	}

	backups, allBackups := e.backupFolder(app)
	opt := archive.Options{Include: app.Include, Keep: app.Keep, Rename: app.Rename}
	var journal archive.Journal
	for i, dest := range dests {
		n, err := archive.Copy(src, dest, filepath.Join(backups, strconv.Itoa(i+1)), opt, &journal)
		if err != nil {
			for _, undoErr := range journal.Undo() {
				e.warn("%s", undoErr)
			}
			return fmt.Errorf("couldn't update %s (is the app running?): %s Files already replaced were put back", dest, sentence(err))
		}
		e.verbose("Copied %d file(s) to %s", n, dest)
	}
	if isDir(backups) {
		e.verbose("Replaced files were backed up to %s", backups)
	}
	archive.PruneBackups(allBackups, e.BackupsToKeep)

	app.InstalledVersion = release.Tag
	r.Installed = release.Tag
	r.set(Updated, verifiedNote(asset))
	return nil
}

// extract unpacks a downloaded archive next to it, into a folder named extracted.
func (e *Engine) extract(asset *github.Asset, file string) (string, error) {
	e.progress("Extracting %s", asset.Name)
	extracted := filepath.Join(filepath.Dir(file), "extracted")
	return extracted, archive.Extract(file, extracted)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
