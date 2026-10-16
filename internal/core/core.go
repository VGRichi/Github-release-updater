// Package core is what Reforge does: add, update and find apps. It has no OS-specific code; the OS
// comes in through system.System and GitHub through Releases, so tests use fakes for both.
//
// The files: core.go (the engine), names.go (matching names), verify.go (checksums and signatures),
// installer.go (installer apps), folders.go (tools copied into folders), download.go (downloads kept
// up to date), add.go, find.go and lookup.go (working out what to track).
package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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
	Search(words string) ([]github.Repo, error)
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

func (r *Result) set(s Status, note string) { r.Status, r.Note = s, note }

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
	checked  map[string]checked // Check's answers, by repository
}

type checked struct {
	release *github.Release
	err     error
}

// checkers is how many apps Check asks GitHub about at once.
const checkers = 6

// Check asks GitHub about the apps' latest releases all at once, so Update, run for each app in turn,
// doesn't wait for each answer one after another. Installer apps that aren't installed are left out,
// since Update doesn't ask about them either.
func (e *Engine) Check(apps []*applist.App) {
	var todo []*applist.App
	seen := map[string]bool{}
	for _, app := range apps {
		k := checkKey(app)
		if seen[k] {
			continue
		}
		if app.IsInstaller() {
			programs, err := e.installedPrograms()
			if err != nil || FindProgram(programs, app.Name) == nil {
				continue
			}
		}
		seen[k] = true
		todo = append(todo, app)
	}
	answers := make([]checked, len(todo))
	var wg sync.WaitGroup
	slots := make(chan struct{}, checkers)
	for i, app := range todo {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			answers[i].release, answers[i].err = e.GitHub.Latest(app.Repository, app.Prerelease)
		})
	}
	wg.Wait()
	e.checked = map[string]checked{}
	for i, app := range todo {
		e.checked[checkKey(app)] = answers[i]
	}
}

func checkKey(app *applist.App) string {
	return fmt.Sprintf("%s %v", strings.ToLower(app.Repository), app.Prerelease)
}

// latest is the app's latest release, from Check if it asked already.
func (e *Engine) latest(app *applist.App) (*github.Release, error) {
	if c, ok := e.checked[checkKey(app)]; ok {
		return c.release, c.err
	}
	return e.GitHub.Latest(app.Repository, app.Prerelease)
}

// sameVersion reports whether a tool's installed version is the release's, however each is written.
func sameVersion(installed, tag string) bool {
	return installed == tag || version.Same(installed, tag)
}

// Update checks one app and installs its latest release if it's newer. It may change the app (what it
// learned, like HadDigest, and a tool's InstalledVersion); save the list afterwards.
func (e *Engine) Update(app *applist.App) Result {
	r := Result{Name: app.Name, Repository: app.Repository}
	if err := e.update(app, &r); err != nil {
		r.set(Failed, sentence(err))
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
			r.set(NotInstalled, fmt.Sprintf("No installed app named like '%s'", app.Name))
			return nil
		}
	}
	release, err := e.latest(app)
	if err != nil || release == nil {
		r.set(Skipped, "No releases found")
		return err
	}
	r.Latest = release.Tag
	switch {
	case app.IsArchive():
		return e.updateFolders(app, release, r)
	case app.IsDownload():
		return e.updateDownload(app, release, r)
	}
	return e.updateInstaller(app, release, program, r)
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

// fetch downloads an asset into a new work folder. done removes the folder.
func (e *Engine) fetch(asset *github.Asset) (file string, done func(), err error) {
	work, err := os.MkdirTemp(e.WorkRoot, "reforge-")
	if err != nil {
		return "", nil, err
	}
	done = func() { os.RemoveAll(work) }
	e.progress("Downloading %s (%s)", asset.Name, size(asset.Size))
	if file, err = e.GitHub.Download(*asset, work); err != nil {
		done()
		return "", nil, err
	}
	return file, done, nil
}

// backupFolder is where this run's backups of an app go, and the app's folder of backups.
func (e *Engine) backupFolder(app *applist.App) (run, all string) {
	all = filepath.Join(e.BackupRoot, unsafeChars.ReplaceAllString(app.Name, "_"))
	return filepath.Join(all, e.now().Format(archive.BackupFolderName)), all
}

var (
	unsafeChars = regexp.MustCompile(`[\\/:*?"<>|]`)
	workFolder  = regexp.MustCompile(`^reforge-\d+$`)
)

// CleanWorkRoot deletes work folders left by a crashed or killed run.
func (e *Engine) CleanWorkRoot() {
	entries, _ := os.ReadDir(e.WorkRoot)
	for _, entry := range entries {
		if !entry.IsDir() || !workFolder.MatchString(entry.Name()) { // before Info: Temp can hold thousands
			continue
		}
		if info, err := entry.Info(); err == nil && info.ModTime().Before(e.now().Add(-24*time.Hour)) {
			os.RemoveAll(filepath.Join(e.WorkRoot, entry.Name()))
		}
	}
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) progress(format string, a ...any) { call(e.Progress, format, a) }
func (e *Engine) verbose(format string, a ...any)  { call(e.Verbose, format, a) }
func (e *Engine) warn(format string, a ...any)     { call(e.Warn, format, a) }

func call(f func(string), format string, a []any) {
	if f != nil {
		f(fmt.Sprintf(format, a...))
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
