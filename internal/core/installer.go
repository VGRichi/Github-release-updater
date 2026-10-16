package core

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/pick"
	"github.com/VGRichi/Reforge/internal/system"
	"github.com/VGRichi/Reforge/internal/version"
)

// updateInstaller updates an installed app with its release's installer, picking the same kind (.msi
// or .exe) as what's installed.
func (e *Engine) updateInstaller(app *applist.App, release *github.Release, program *system.Program, r *Result) error {
	r.Installed = program.Version
	current, latest := version.Parse(program.Version), version.Parse(release.Tag)
	switch {
	case current == nil || latest == nil:
		r.set(Skipped, "Can't compare versions")
		return nil
	case version.Compare(current, latest) >= 0:
		r.Status = UpToDate
		return nil
	}
	asset := pick.Installer(release.Assets, app.Pattern, program.MSI, e.Arch)
	switch {
	case asset == nil:
		r.set(Skipped, "No matching Windows installer in this release")
	case !e.offer(app, asset, r):
	case !program.PerUser && !e.System.IsAdmin():
		r.set(NeedsAdmin, "Installed for all users; run as administrator")
	case e.approved(app, asset, r):
		return e.runInstaller(app, asset, r, Updated)
	}
	return nil
}

// Install installs an app for the first time: the latest release's installer (the .msi when there's a
// choice), always asking first unless force (the checksum rules still apply). It learns the app's name
// in Installed apps from what the installer added. The second result says whether to track the app:
// not when no new app appeared and app.Name was empty, since a guessed name would never match.
func (e *Engine) Install(app *applist.App, force bool) (Result, bool) {
	r := Result{Repository: app.Repository}
	if err := e.install(app, force, &r); err != nil {
		r.set(Failed, sentence(err))
		e.warn("%s: %s", app.Repository, r.Note)
	}
	track := r.Status == Installed
	r.Name = app.Name
	if app.Name == "" {
		r.Name = path.Base(app.Repository)
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
	if err != nil || release == nil {
		r.set(Skipped, "No releases found")
		return err
	}
	r.Latest = release.Tag
	asset := pick.Installer(release.Assets, app.Pattern, true, e.Arch)
	if asset == nil {
		r.set(Skipped, "No Windows installer in this release (for a zip tool, use reforge add --dest)")
		return nil
	}
	if asset.Digest != "" {
		app.HadDigest = true
	}
	// Installing something new always gets a question; the unverified-download question is that question
	approved := force
	switch {
	case asset.Digest == "" && !app.AllowUnverified:
		approved = e.approve(app, asset)
	case !force:
		approved = e.yes(fmt.Sprintf("Install %s from %s? (%s)", asset.Name, app.Repository, verificationNote(app, asset)))
	}
	if !approved {
		r.set(Skipped, "Not installed")
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
	if err := e.runInstaller(app, asset, r, Installed); err != nil {
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
	leaf := strings.ToLower(path.Base(app.Repository))
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

// runInstaller downloads the installer, checks its signature, runs it silently and records the outcome.
func (e *Engine) runInstaller(app *applist.App, asset *github.Asset, r *Result, done Status) error {
	file, cleanup, err := e.fetch(asset)
	if err != nil {
		return err
	}
	defer cleanup()
	signer, ok, err := e.checkSigner(app, asset, file)
	if err != nil {
		return err
	}
	if !ok {
		r.set(Skipped, "Signature changed since earlier releases; not installed")
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
		r.set(done, verifiedNote(asset))
		if signer != "" {
			r.Note += ", signed by " + signer
		}
		if code != 0 {
			r.Note += ". Restart needed to finish"
		}
	default:
		r.set(Failed, fmt.Sprintf("Installer exit code %d", code))
	}
	return nil
}

// silentArgs recognises common installer types from the first few MB of the file.
func silentArgs(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 4<<20)
	n, _ := io.ReadFull(f, buf) // one Read may return less
	switch {
	case bytes.Contains(buf[:n], []byte("Inno Setup")):
		return "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART"
	case bytes.Contains(buf[:n], []byte("Nullsoft")):
		return "/S"
	}
	return ""
}
