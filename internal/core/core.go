// Package core is what Reforge does: update, install and find apps. It has no OS-specific code;
// the OS comes in through system.System and GitHub through Releases, so tests use fakes for both.
package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/archive"
	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/pick"
	"github.com/VGRichi/Reforge/internal/system"
	"github.com/VGRichi/Reforge/internal/version"
)

// Releases is where releases come from (GitHub).
type Releases interface {
	Latest(repo string, prerelease bool) (*github.Release, error)
	Download(a github.Asset, dir string) (string, error)
}

// Asker asks the user to pick an answer, returning its index. When nobody can answer (a scheduled task,
// no terminal), it returns def; every question here has the safe answer as its default.
type Asker interface {
	Choose(question string, answers []string, def int) int
}

// Status says what happened to an app.
type Status string

const (
	Updated         Status = "Updated"
	UpToDate        Status = "UpToDate"
	UpdateAvailable Status = "UpdateAvailable"
	NotInstalled    Status = "NotInstalled"
	NeedsAdmin      Status = "NeedsAdmin"
	Skipped         Status = "Skipped"
	Failed          Status = "Failed"
	Installed       Status = "Installed"
	Available       Status = "Available"
)

// Result is the outcome for one app.
type Result struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Installed  string `json:"installed"`
	Latest     string `json:"latest"`
	Status     Status `json:"status"`
	Note       string `json:"note"`
}

// Engine runs updates, installs and searches.
type Engine struct {
	GitHub        Releases
	System        system.System
	Ask           Asker
	Arch          pick.Arch
	WorkRoot      string // downloads are extracted in reforge-* folders here
	BackupRoot    string // replaced files go to <BackupRoot>/<app>/<date and time>
	BackupsToKeep int
	DryRun        bool         // check only: download and change nothing
	Progress      func(string) // steps worth showing, like downloads
	Verbose       func(string) // more detail
	Warn          func(string)
	Now           func() time.Time

	programs []system.Program
	loaded   bool
}

func (e *Engine) verbose(format string, a ...any) {
	if e.Verbose != nil {
		e.Verbose(fmt.Sprintf(format, a...))
	}
}

func (e *Engine) warn(format string, a ...any) {
	if e.Warn != nil {
		e.Warn(fmt.Sprintf(format, a...))
	}
}

func (e *Engine) installedPrograms() ([]system.Program, error) {
	if !e.loaded {
		p, err := e.System.Programs()
		if err != nil {
			return nil, err
		}
		e.programs, e.loaded = p, true
	}
	return e.programs, nil
}

// Update checks one app and installs its latest release if it's newer. It may change the app
// (what it learned, like HadDigest, and an archive tool's InstalledVersion); save the list afterwards.
func (e *Engine) Update(app *applist.App) Result {
	r := Result{Name: app.Name, Repository: app.Repository}
	if err := e.update(app, &r); err != nil {
		r.Status, r.Note = Failed, sentence(err)
		e.warn("%s: %s", app.Name, r.Note)
	}
	return r
}

func (e *Engine) update(app *applist.App, r *Result) error {
	var program *system.Program
	if app.IsInstaller() {
		programs, err := e.installedPrograms()
		if err != nil {
			return err
		}
		if program = FindProgram(programs, app.Name); program == nil {
			r.Status, r.Note = NotInstalled, fmt.Sprintf("No installed app named like '%s'", app.Name)
			return nil
		}
	}
	release, err := e.GitHub.Latest(app.Repository, app.Prerelease)
	if err != nil {
		return err
	}
	if release == nil {
		r.Status, r.Note = Skipped, "No releases found"
		return nil
	}
	r.Latest = release.Tag
	switch {
	case app.IsArchive():
		return e.updateArchive(app, release, r)
	case app.IsDownload():
		return e.updateDownload(app, release, r)
	}
	return e.updateInstaller(app, release, program, r)
}

func (e *Engine) updateInstaller(app *applist.App, release *github.Release, program *system.Program, r *Result) error {
	r.Installed = program.Version
	current, latest := version.Parse(program.Version), version.Parse(release.Tag)
	if current == nil || latest == nil {
		r.Status, r.Note = Skipped, "Can't compare versions"
		return nil
	}
	if version.Compare(current, latest) >= 0 {
		r.Status = UpToDate
		return nil
	}
	asset := pick.Installer(release.Assets, app.Pattern, program.MSI, e.Arch)
	if asset == nil {
		r.Status, r.Note = Skipped, "No matching Windows installer in this release"
		return nil
	}
	if asset.Digest != "" {
		app.HadDigest = true
	}
	r.Status, r.Note = UpdateAvailable, verificationNote(app, asset)
	if e.DryRun {
		return nil
	}
	if !program.PerUser && !e.System.IsAdmin() {
		r.Status, r.Note = NeedsAdmin, "Installed for all users; run as administrator"
		return nil
	}
	if !e.approve(app, asset) {
		r.Status, r.Note = Skipped, "Unverified download declined"
		return nil
	}
	return e.installAsset(app, asset, r, Updated)
}

// installAsset downloads and runs an installer, and records the outcome.
func (e *Engine) installAsset(app *applist.App, asset *github.Asset, r *Result, done Status) error {
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
	signer, ok, err := e.checkSigner(app, asset, file)
	if err != nil {
		return err
	}
	if !ok {
		r.Status, r.Note = Skipped, "Signature changed since earlier releases; not installed"
		return nil
	}
	e.progress("Installing %s", asset.Name)
	var code int
	if strings.HasSuffix(strings.ToLower(file), ".msi") {
		code, err = e.System.RunInstaller("msiexec.exe", fmt.Sprintf(`/i "%s" /qn /norestart`, file))
	} else {
		args := app.SilentArgs
		if args == "" {
			args = silentArgs(file)
		}
		if args == "" {
			e.warn("Unknown installer type for %s; opening it normally.", asset.Name)
		}
		code, err = e.System.RunInstaller(file, args)
	}
	if err != nil {
		return err
	}
	switch code {
	case 0, 3010, 1641: // 3010 and 1641: done, restart needed
		r.Status, r.Note = done, "Unverified"
		if asset.Digest != "" {
			r.Note = "Checksum verified"
		}
		if signer != "" {
			r.Note += ", signed by " + signer
		}
		if code != 0 {
			r.Note += ". Restart needed to finish"
		}
	default:
		r.Status, r.Note = Failed, fmt.Sprintf("Installer exit code %d", code)
	}
	return nil
}

// checkSigner checks the installer's digital signature and remembers who signed the app. A broken
// signature (a file changed after it was signed) installs nothing. A release signed by someone else
// than earlier ones, or not signed when they were, is unusual (a hacked account can upload a release
// with a valid checksum, but not with the developer's signature), so it asks first. Apps that were
// never signed are installed as before.
func (e *Engine) checkSigner(app *applist.App, asset *github.Asset, file string) (signer string, ok bool, err error) {
	signer, err = e.System.Signer(file)
	if err != nil {
		return "", false, fmt.Errorf("%s: %v. Nothing was installed", asset.Name, err)
	}
	switch {
	case signer == app.Publisher:
	case app.Publisher == "":
		app.Publisher = signer
	default:
		now := "isn't signed"
		if signer != "" {
			now = "is signed by " + signer
		}
		e.warn("%s: earlier releases were signed by %s, but %s %s. That is unusual; check the release page before installing.", app.Name, app.Publisher, asset.Name, now)
		if e.Ask.Choose(fmt.Sprintf("Install %s anyway?", asset.Name), []string{"Yes", "No"}, 1) != 0 {
			return signer, false, nil
		}
		if signer != "" {
			app.Publisher = signer
		}
	}
	return signer, true, nil
}

// silentArgs recognises common installer types from the first few MB of the file.
func silentArgs(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 4<<20)
	n, _ := f.Read(buf)
	switch {
	case bytes.Contains(buf[:n], []byte("Inno Setup")):
		return "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART"
	case bytes.Contains(buf[:n], []byte("Nullsoft")):
		return "/S"
	}
	return ""
}

func (e *Engine) updateArchive(app *applist.App, release *github.Release, r *Result) error {
	r.Installed = app.InstalledVersion
	var dests, missing []string
	for _, d := range app.Destination {
		if d == "" {
			continue
		}
		dests = append(dests, d)
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			missing = append(missing, d)
		}
	}
	switch {
	case len(dests) == 0:
		r.Status, r.Note = Skipped, "No destination folder set"
		return nil
	case len(missing) > 0:
		r.Status, r.Note = Skipped, "Folder not found: "+strings.Join(missing, ", ")
		return nil
	case app.InstalledVersion == release.Tag:
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
	extracted := filepath.Join(work, "extracted")
	e.progress("Extracting %s", asset.Name)
	if err := archive.Extract(file, extracted); err != nil {
		return err
	}
	src, err := archive.ContentRoot(extracted, app.Subfolder)
	if err != nil {
		return err
	}

	appBackups := filepath.Join(e.BackupRoot, safeName(app.Name))
	backupRoot := filepath.Join(appBackups, e.now().Format(archive.BackupFolderName))
	opt := archive.Options{Include: app.Include, Keep: app.Keep, Rename: app.Rename}
	var journal archive.Journal
	for i, dest := range dests {
		n, err := archive.Copy(src, dest, filepath.Join(backupRoot, strconv.Itoa(i+1)), opt, &journal)
		if err != nil {
			// All or nothing: a half-updated folder (often a file in use by a running game) is worse than the old version
			for _, undoErr := range journal.Undo() {
				e.warn("%s", undoErr)
			}
			return fmt.Errorf("couldn't update %s (is the app running?): %s Files already replaced were put back", dest, sentence(err))
		}
		e.verbose("Copied %d file(s) to %s", n, dest)
	}
	if _, err := os.Stat(backupRoot); err == nil {
		e.verbose("Replaced files were backed up to %s", backupRoot)
	}
	archive.PruneBackups(appBackups, e.BackupsToKeep)

	app.InstalledVersion = release.Tag
	r.Installed, r.Status = release.Tag, Updated
	r.Note = "Unverified"
	if asset.Digest != "" {
		r.Note = "Checksum verified"
	}
	return nil
}

// Verification policy. With a checksum: verified, installed without asking. Without one: asks, unless
// the app is trusted (AllowUnverified). Without one when earlier releases had one: always asks.

func verificationNote(app *applist.App, asset *github.Asset) string {
	switch {
	case asset.Digest != "":
		return "Checksum available"
	case app.HadDigest:
		return "No checksum (unusual: earlier releases had one)"
	case app.AllowUnverified:
		return "No checksum (trusted app)"
	}
	return "No checksum (will ask before installing)"
}

func (e *Engine) approve(app *applist.App, asset *github.Asset) bool {
	if asset.Digest != "" {
		return true
	}
	if app.HadDigest {
		e.warn("%s: earlier releases had a checksum, but %s has none. That is unusual; check the release page before installing.", app.Name, asset.Name)
		return e.Ask.Choose(fmt.Sprintf("Install %s anyway?", asset.Name), []string{"Yes", "No"}, 1) == 0
	}
	if app.AllowUnverified {
		e.verbose("No checksum for %s; installing because %s is trusted.", asset.Name, app.Name)
		return true
	}
	switch e.Ask.Choose(fmt.Sprintf("%s has no checksum, so it can't be verified. Install anyway? (Always = trust this app from now on)", asset.Name),
		[]string{"Yes", "Always", "No"}, 2) {
	case 0:
		return true
	case 1:
		app.AllowUnverified = true
		return true
	}
	return false
}

// Install installs an app for the first time: the latest release's installer (the .msi when there's a
// choice), always asking first unless force (the checksum rules still apply). It learns the app's name
// in Installed apps from what the installer added. The second result says whether to track the app:
// not when no new app appeared and app.Name was empty, since a guessed name would never match.
func (e *Engine) Install(app *applist.App, force bool) (Result, bool) {
	r := Result{Name: app.Name, Repository: app.Repository}
	if err := e.install(app, force, &r); err != nil {
		r.Status, r.Note = Failed, sentence(err)
		e.warn("%s: %s", app.Repository, r.Note)
	}
	track := r.Status == Installed
	r.Name = app.Name
	if app.Name == "" {
		r.Name = repoLeaf(app.Repository)
		if track {
			track = false
			r.Note += ". Not tracked"
			e.warn("Installed, but it didn't show up as a new app in Installed apps (was it already installed?), so it isn't tracked. Track it with the name Windows shows: reforge add %s --name \"<name>\"", app.Repository)
		}
	}
	return r, track
}

func (e *Engine) install(app *applist.App, force bool, r *Result) error {
	release, err := e.GitHub.Latest(app.Repository, app.Prerelease)
	if err != nil {
		return err
	}
	if release == nil {
		r.Status, r.Note = Skipped, "No releases found"
		return nil
	}
	r.Latest = release.Tag
	asset := pick.Installer(release.Assets, app.Pattern, true, e.Arch)
	if asset == nil {
		r.Status, r.Note = Skipped, "No Windows installer in this release (for a zip tool, use reforge add --dest)"
		return nil
	}
	if asset.Digest != "" {
		app.HadDigest = true
	}
	r.Status, r.Note = Available, verificationNote(app, asset)
	if e.DryRun {
		return nil
	}
	// Installing something new always gets a question; the unverified-download question is that question
	var approved bool
	switch {
	case asset.Digest == "" && !app.AllowUnverified:
		approved = e.approve(app, asset)
	case force:
		approved = true
	default:
		approved = e.Ask.Choose(fmt.Sprintf("Install %s from %s? (%s)", asset.Name, app.Repository, r.Note), []string{"Yes", "No"}, 1) == 0
	}
	if !approved {
		r.Status, r.Note = Skipped, "Not installed"
		return nil
	}

	// What the installer added (or changed, if the app was already there) tells us its name
	before, err := e.System.Programs()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, p := range before {
		seen[p.Key+"|"+p.Version] = true
	}
	if err := e.installAsset(app, asset, r, Installed); err != nil {
		return err
	}
	if r.Status != Installed {
		if !e.System.IsAdmin() {
			r.Note += ". It may need to run as administrator"
		}
		return nil
	}
	after, err := e.System.Programs()
	if err != nil {
		return err
	}
	var found *system.Program
	leaf := strings.ToLower(repoLeaf(app.Repository))
	for i := range after {
		p := &after[i]
		if seen[p.Key+"|"+p.Version] {
			continue
		}
		if found == nil || (!strings.Contains(strings.ToLower(found.Name), leaf) && strings.Contains(strings.ToLower(p.Name), leaf)) {
			found = p
		}
	}
	if found != nil {
		if app.Name == "" {
			app.Name = BaseName(found.Name)
		}
		r.Installed = found.Version
	}
	return nil
}

// CleanWorkRoot deletes work folders left by a crashed or killed run.
func (e *Engine) CleanWorkRoot() {
	entries, _ := os.ReadDir(e.WorkRoot)
	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil && entry.IsDir() && workFolder.MatchString(entry.Name()) && info.ModTime().Before(e.now().Add(-24*time.Hour)) {
			os.RemoveAll(filepath.Join(e.WorkRoot, entry.Name()))
		}
	}
}

var workFolder = regexp.MustCompile(`^reforge-\d+$`)

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

var unsafeChars = regexp.MustCompile(`[\\/:*?"<>|]`)

func safeName(name string) string { return unsafeChars.ReplaceAllString(name, "_") }

func repoLeaf(repo string) string { return repo[strings.LastIndex(repo, "/")+1:] }

func (e *Engine) progress(format string, a ...any) {
	if e.Progress != nil {
		e.Progress(fmt.Sprintf(format, a...))
	}
}

func size(bytes int64) string {
	if bytes <= 0 {
		return "size unknown"
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
}

// sentence makes an error readable as a note: a capital first letter and one full stop.
func sentence(err error) string {
	s := strings.TrimSpace(err.Error())
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
