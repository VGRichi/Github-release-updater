package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/core"
	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/pick"
	"github.com/VGRichi/Reforge/internal/system"
	"github.com/VGRichi/Reforge/internal/wildcard"
)

// downloadsFolder is where add and find look for downloads; tests replace it.
var downloadsFolder = system.DownloadsFolder

// newEngine connects the engine to the real GitHub and OS; tests replace it.
var newEngine = func(c *cli) *core.Engine {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return &core.Engine{
		GitHub: github.New(token), System: system.New(), Ask: c, Arch: pick.Arch(runtime.GOARCH),
		WorkRoot: os.TempDir(), BackupsToKeep: 3,
		Progress: c.progress, Verbose: c.detail, Warn: c.warn,
	}
}

// engine returns an engine whose backups go next to the list file.
func (c *cli) engine(listPath string) *core.Engine {
	e := newEngine(c)
	e.BackupRoot = filepath.Join(filepath.Dir(listPath), "backups")
	return e
}

func (c *cli) load() ([]*applist.App, string, error) {
	path := c.configPath
	if path == "" {
		var err error
		if path, err = applist.DefaultPath(); err != nil {
			return nil, "", err
		}
	}
	apps, err := applist.Load(path)
	return apps, path, err
}

func runAdd(c *cli, args []string) error {
	fs := c.flags("add")
	name := fs.String("name", "", "the app's name as shown in Installed apps (a part is fine); found for you when it's installed")
	var dest, include, keep multi
	rename := renames{}
	fs.Var(&dest, "dest", "copy the release's files into this folder on every update, like a game's folder (more than once for several)")
	subfolder := fs.String("subfolder", "", "with --dest: copy one folder from inside the archive, like x64")
	fs.Var(&include, "include", `with --dest: only copy matching files, like '*.dll' or 'bin\*'`)
	fs.Var(&keep, "keep", "with --dest: files that are added if missing but never overwritten, like '*.ini'")
	fs.Var(rename, "rename", "with --dest: rename a file while copying, like OptiScaler.dll=dxgi.dll")
	pattern := fs.String("pattern", "", "pick the release file yourself, like '*win-x64.msi'")
	silentArgs := fs.String("silent-args", "", "silent-install arguments for an installer that isn't recognised")
	prerelease := fs.Bool("prerelease", false, "follow beta releases")
	trusted := fs.Bool("allow-unverified", false, "trust this app: install releases without a checksum without asking")
	force := fs.Bool("force", false, "replace an app already in the list")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 || len(pos) > 2 || (len(pos) == 2 && len(dest) > 0) {
		fs.Usage()
		return errUsage
	}
	repo, ok := github.ParseRepo(pos[0])
	if !ok {
		return fmt.Errorf("'%s' isn't a GitHub repository. Use owner/repo, for example storytold/photocraft", pos[0])
	}
	app := &applist.App{
		Name: *name, Repository: repo, Subfolder: *subfolder, Pattern: *pattern, SilentArgs: *silentArgs,
		Include: applist.Strings(include), Keep: applist.Strings(keep), Prerelease: *prerelease, AllowUnverified: *trusted,
	}
	if len(rename) > 0 {
		app.Rename = rename
	}
	apps, path, err := c.load()
	if err != nil {
		return err
	}

	switch {
	case len(dest) > 0: // copied into folders on every update
		for _, d := range dest {
			abs, err := absFolder(d)
			if err != nil {
				return err
			}
			app.Destination = append(app.Destination, abs)
		}
		app.Type = "archive"
	case len(pos) == 2: // a download kept in this folder; its names tell the version you have
		abs, err := absFolder(pos[1])
		if err != nil {
			return err
		}
		app.Type, app.Folder, app.InstalledVersion = "download", abs, core.DetectVersion(abs, repo)
	default: // worked out from what's installed and what the release has
		if err := c.engine(path).Prepare(app, []string{downloadsFolder()}); err != nil {
			return err
		}
	}
	if app.Name == "" {
		app.Name = repo[strings.LastIndex(repo, "/")+1:]
	}

	old := applist.Find(apps, app.Name)
	if old != nil && !*force {
		return fmt.Errorf("'%s' is already in the list. Use a different --name, or --force to replace it", app.Name)
	}
	if old != nil {
		if strings.EqualFold(old.Repository, repo) { // replacing the same repository: keep what was learned
			app.HadDigest, app.Publisher = old.HadDigest, old.Publisher
			if !app.IsInstaller() && app.InstalledVersion == "" {
				app.InstalledVersion = old.InstalledVersion
			}
		}
		*old = *app
	} else {
		apps = append(apps, app)
	}
	if err := applist.Save(path, apps); err != nil {
		return err
	}
	switch {
	case app.IsDownload() && app.InstalledVersion != "":
		fmt.Fprintf(c.stdout, "Tracking %s (%s): you have %s in %s; updates are downloaded there\n", app.Name, repo, app.InstalledVersion, app.Folder)
	case app.IsDownload():
		fmt.Fprintf(c.stdout, "Tracking %s (%s): the next update downloads it into %s\n", app.Name, repo, app.Folder)
	case app.IsArchive():
		fmt.Fprintf(c.stdout, "Tracking %s (%s): updates are copied into %s\n", app.Name, repo, strings.Join(app.Destination, ", "))
	default:
		fmt.Fprintf(c.stdout, "Tracking %s (%s): updated with its installer\n", app.Name, repo)
	}
	return nil
}

// absFolder is the full path of an existing folder.
func absFolder(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if info, statErr := os.Stat(abs); err != nil || statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("folder not found: %s", dir)
	}
	return abs, nil
}

func runInstall(c *cli, args []string) error {
	fs := c.flags("install")
	name := fs.String("name", "", "track it by this name (as shown in Installed apps) instead of the one it gets")
	pattern := fs.String("pattern", "", "pick the release file yourself, like '*setup.exe' (default: the .msi when there is one)")
	silentArgs := fs.String("silent-args", "", "silent-install arguments for an installer that isn't recognised")
	prerelease := fs.Bool("prerelease", false, "install and follow beta releases")
	trusted := fs.Bool("allow-unverified", false, "trust this app: install releases without a checksum without asking")
	yes := fs.Bool("yes", false, "don't ask before installing (downloads without a checksum are still asked about)")
	dryRun := fs.Bool("dry-run", false, "only show what would be installed")
	asJSON := fs.Bool("json", false, "print the results as JSON")
	fs.BoolVar(&c.verbose, "verbose", false, "show more detail")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 || (*name != "" && len(pos) > 1) {
		fs.Usage()
		return errUsage
	}
	apps, path, err := c.load()
	if err != nil {
		return err
	}
	e := c.engine(path)
	e.DryRun = *dryRun
	if !*dryRun {
		e.CleanWorkRoot()
	}
	var results []core.Result
	for _, arg := range pos {
		repo, ok := github.ParseRepo(arg)
		if !ok {
			c.warn(fmt.Sprintf("'%s' isn't a GitHub repository. Use owner/repo, for example storytold/photocraft.", arg))
			c.failed = true
			continue
		}
		if tracked := trackedAs(apps, repo, *name); tracked != "" {
			c.warn(fmt.Sprintf("'%s' is already in the list. Update it with: reforge update \"%s\"", tracked, tracked))
			c.failed = true
			continue
		}
		app := &applist.App{Name: *name, Repository: repo, Pattern: *pattern, SilentArgs: *silentArgs, Prerelease: *prerelease, AllowUnverified: *trusted}
		r, track := e.Install(app, *yes)
		if track {
			if applist.Find(apps, app.Name) != nil {
				c.warn(fmt.Sprintf("'%s' is already in the list, so this app isn't tracked. Track it with: reforge add %s --name \"<another name>\"", app.Name, repo))
			} else {
				apps = append(apps, app)
				if err := applist.Save(path, apps); err != nil {
					return err
				}
			}
		}
		c.failed = c.failed || r.Status == core.Failed
		results = append(results, r)
	}
	return c.printResults(results, *asJSON)
}

// trackedAs returns the name an installer app from repo (or an app called name) is tracked as.
func trackedAs(apps []*applist.App, repo, name string) string {
	for _, a := range apps {
		if (strings.EqualFold(a.Repository, repo) && a.IsInstaller()) || (name != "" && strings.EqualFold(a.Name, name)) {
			return a.Name
		}
	}
	return ""
}

// listEntry is one app in 'reforge list': what's tracked, and what checking its latest release found.
type listEntry struct {
	Name        string   `json:"name"`
	Repository  string   `json:"repository"`
	Type        string   `json:"type"`
	Installed   string   `json:"installed"`
	Latest      string   `json:"latest"`
	Status      string   `json:"status"`
	Note        string   `json:"note"`
	Destination []string `json:"destination,omitempty"`
	Folder      string   `json:"folder,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Trusted     bool     `json:"trusted"`
}

func runList(c *cli, args []string) error {
	fs := c.flags("list")
	offline := fs.Bool("offline", false, "only list, without checking GitHub for updates")
	asJSON := fs.Bool("json", false, "print the list as JSON")
	names, err := parse(fs, args)
	if err != nil {
		return err
	}
	apps, path, err := c.load()
	if err != nil {
		return err
	}
	apps = matching(apps, names)
	if len(apps) == 0 && !*asJSON {
		fmt.Fprintln(c.stderr, "No apps tracked yet. Add one with 'reforge add', or find installed ones with 'reforge find'.")
		return nil
	}
	var e *core.Engine
	if !*offline {
		e = c.engine(path)
		e.DryRun = true // a check: nothing is downloaded, and list never saves the list
	}
	entries := []listEntry{}
	var rows [][]string
	for _, a := range apps {
		le := listEntry{Name: a.Name, Repository: a.Repository, Type: "Installer", Destination: a.Destination, Publisher: a.Publisher, Trusted: a.AllowUnverified}
		switch {
		case a.IsArchive():
			le.Type, le.Installed = "Archive", a.InstalledVersion
		case a.IsDownload():
			le.Type, le.Installed, le.Folder = "Download", a.InstalledVersion, a.Folder
		}
		if e != nil {
			r := e.Update(a)
			le.Installed, le.Latest, le.Status, le.Note = r.Installed, r.Latest, string(r.Status), r.Note
			c.failed = c.failed || r.Status == core.Failed
		}
		entries = append(entries, le)
		folder := strings.Join(le.Destination, "; ")
		if le.Folder != "" {
			folder = le.Folder
		}
		rows = append(rows, []string{le.Name, le.Installed, le.Latest, statusText(le), folder})
	}
	if *asJSON {
		return c.json(entries)
	}
	c.table([]string{"Name", "Installed", "Latest", "Status", "Folder"}, rows)
	return nil
}

func statusText(le listEntry) string {
	switch s := core.Status(le.Status); s {
	case core.UpdateAvailable:
		if strings.HasPrefix(le.Note, "No checksum") {
			return readable(s) + " (no checksum)"
		}
		return readable(s)
	case core.UpToDate, core.NotInstalled:
		return readable(s)
	case core.Failed:
		return "Error: " + le.Note
	}
	return le.Note // skipped (like "Folder not found"), or nothing with --offline
}

func runUpdate(c *cli, args []string) error {
	fs := c.flags("update")
	asJSON := fs.Bool("json", false, "print the results as JSON")
	fs.BoolVar(&c.verbose, "verbose", false, "show more detail")
	names, err := parse(fs, args)
	if err != nil {
		return err
	}
	apps, path, err := c.load()
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		c.warn("no apps in the list yet. Add one with 'reforge add', or find installed ones with 'reforge find'.")
		return nil
	}
	for _, n := range names {
		if len(matching(apps, []string{n})) == 0 {
			c.warn(fmt.Sprintf("no app named '%s' in the list", n))
			c.failed = true
		}
	}
	e := c.engine(path)
	e.CleanWorkRoot()
	var results []core.Result
	for _, app := range matching(apps, names) {
		before, _ := json.Marshal(app)
		e.Progress = func(msg string) { c.progress(app.Name + ": " + msg) }
		r := e.Update(app)
		if after, _ := json.Marshal(app); !bytes.Equal(before, after) {
			if err := applist.Save(path, apps); err != nil {
				return err
			}
		}
		c.failed = c.failed || r.Status == core.Failed
		results = append(results, r)
	}
	return c.printResults(results, *asJSON)
}

func (c *cli) printResults(results []core.Result, asJSON bool) error {
	if asJSON {
		if results == nil {
			results = []core.Result{}
		}
		return c.json(results)
	}
	if len(results) == 0 {
		return nil
	}
	var rows [][]string
	for _, r := range results {
		rows = append(rows, []string{r.Name, r.Installed, r.Latest, readable(r.Status), r.Note})
	}
	c.table([]string{"Name", "Installed", "Latest", "Status", "Note"}, rows)
	return nil
}

// readable spells a status out for people ("Up to date"); --json keeps the exact values for scripts.
func readable(s core.Status) string {
	switch s {
	case core.UpToDate:
		return "Up to date"
	case core.UpdateAvailable:
		return "Update available"
	case core.NotInstalled:
		return "Not installed"
	case core.NeedsAdmin:
		return "Needs admin"
	}
	return string(s)
}

func runFind(c *cli, args []string) error {
	fs := c.flags("find")
	add := fs.Bool("add", false, "ask about each app found, and track the ones you pick")
	asJSON := fs.Bool("json", false, "print what's found as JSON")
	folders, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(folders) == 0 {
		folders = []string{downloadsFolder()}
	}
	apps, path, err := c.load()
	if err != nil {
		return err
	}
	tracked := func(repo string) bool {
		for _, a := range apps {
			if strings.EqualFold(a.Repository, repo) {
				return true
			}
		}
		return false
	}
	found, err := c.engine(path).Find(folders, tracked)
	if err != nil {
		return err
	}
	if *asJSON {
		if found == nil {
			found = []core.Candidate{}
		}
		return c.json(found)
	}
	if len(found) == 0 {
		fmt.Fprintln(c.stderr, "Nothing new from GitHub found.")
		return nil
	}
	var rows [][]string
	for _, f := range found {
		rows = append(rows, []string{f.Name, f.Repository, f.Version, f.Type})
	}
	c.table([]string{"Name", "Repository", "Version", "Type"}, rows)
	if !*add {
		fmt.Fprintln(c.stderr, "\nTrack them with 'reforge find --add' (it asks about each one).")
		return nil
	}
	added := 0
	for _, f := range found {
		question := fmt.Sprintf("Track %s (%s)?", f.Name, f.Repository)
		app := &applist.App{Name: f.Name, Repository: f.Repository}
		if f.Type == "Archive" {
			// A download: new versions are downloaded into the same folder
			question = fmt.Sprintf("Keep %s (%s) up to date in %s?", f.Name, f.Repository, f.Folder)
			app.Type, app.Folder, app.InstalledVersion = "download", f.Folder, f.Version
		}
		if applist.Find(apps, f.Name) != nil || c.Choose(question, []string{"Yes", "No"}, 1) != 0 {
			continue
		}
		apps = append(apps, app)
		added++
	}
	if added > 0 {
		if err := applist.Save(path, apps); err != nil {
			return err
		}
		fmt.Fprintf(c.stdout, "Tracking %d more app(s).\n", added)
	}
	return nil
}

func runRemove(c *cli, args []string) error {
	fs := c.flags("remove")
	names, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fs.Usage()
		return errUsage
	}
	apps, path, err := c.load()
	if err != nil {
		return err
	}
	var keep []*applist.App
	removed := 0
	for _, a := range apps {
		if len(matching([]*applist.App{a}, names)) == 1 {
			fmt.Fprintf(c.stdout, "Stopped tracking %s\n", a.Name)
			removed++
		} else {
			keep = append(keep, a)
		}
	}
	for _, n := range names {
		if len(matching(apps, []string{n})) == 0 {
			c.warn(fmt.Sprintf("no app named '%s' in the list. Check the names with 'reforge list'.", n))
			c.failed = true
		}
	}
	if removed == 0 {
		return nil
	}
	return applist.Save(path, keep)
}

// matching returns the apps whose names match any of the patterns (all apps without patterns).
func matching(apps []*applist.App, patterns []string) []*applist.App {
	if len(patterns) == 0 {
		return apps
	}
	var found []*applist.App
	for _, a := range apps {
		for _, p := range patterns {
			if wildcard.Match(p, a.Name) {
				found = append(found, a)
				break
			}
		}
	}
	return found
}
