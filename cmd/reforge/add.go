package main

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/core"
	"github.com/VGRichi/Reforge/internal/github"
)

// runAdd tracks an app, mod or tool: by name or owner/repo, installing it first if it isn't installed
// yet. With no name, it finds what's on this PC and asks about each one.
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
	yes := fs.Bool("yes", false, "install an app that isn't installed yet without asking (a download without a checksum is still asked about)")
	fs.BoolVar(&c.verbose, "verbose", false, "show more detail")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 2 || (len(pos) == 2 && len(dest) > 0) || (len(pos) == 0 && (len(dest) > 0 || *name != "")) {
		fs.Usage()
		return errUsage
	}
	apps, listPath, err := c.load()
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return c.addFound(listPath, apps)
	}
	repo, err := c.repository(listPath, pos[0])
	if err != nil {
		return err
	}
	app := &applist.App{
		Name: *name, Repository: repo, Subfolder: *subfolder, Pattern: *pattern, SilentArgs: *silentArgs,
		Include: applist.Strings(include), Keep: applist.Strings(keep), Prerelease: *prerelease, AllowUnverified: *trusted,
	}
	if len(rename) > 0 {
		app.Rename = rename
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
		err := c.engine(listPath).Prepare(app, []string{downloadsFolder()})
		var notInstalled *core.NotInstalledError
		if errors.As(err, &notInstalled) {
			return c.install(listPath, apps, app, *yes)
		}
		if err != nil {
			return err
		}
	}
	if app.Name == "" {
		app.Name = path.Base(repo)
	}
	if err := c.track(listPath, apps, app, *force); err != nil {
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

// track adds the app to the list and saves it. An app with the same name is only replaced with force,
// keeping what was learned about it when it's the same repository.
func (c *cli) track(listPath string, apps []*applist.App, app *applist.App, force bool) error {
	old := applist.Find(apps, app.Name)
	switch {
	case old == nil:
		apps = append(apps, app)
	case !force:
		return fmt.Errorf("'%s' is already in the list. Use a different --name, or --force to replace it", app.Name)
	default:
		if strings.EqualFold(old.Repository, app.Repository) {
			app.HadDigest, app.Publisher = old.HadDigest, old.Publisher
			if !app.IsInstaller() && app.InstalledVersion == "" {
				app.InstalledVersion = old.InstalledVersion
			}
		}
		*old = *app
	}
	return applist.Save(listPath, apps)
}

// repository is owner/repo, from what was typed: owner/repo or a GitHub link as it is, or a name looked
// up on this PC and then on GitHub.
func (c *cli) repository(listPath, typed string) (string, error) {
	if repo, ok := github.ParseRepo(typed); ok {
		return repo, nil
	}
	if strings.Contains(typed, "/") {
		return "", fmt.Errorf("'%s' isn't a GitHub repository. Use owner/repo, for example storytold/photocraft", typed)
	}
	return c.lookUp(listPath, typed)
}

// lookUp finds the repository for a name: on this PC first (an installed app linking to GitHub, or a
// download from GitHub), then by searching GitHub, asking about the likeliest matches one at a time.
// Only the match you pick is checked for releases (one API request, not one per match); one without
// releases has nothing to track, so the next match is asked about.
func (c *cli) lookUp(listPath, name string) (string, error) {
	e := c.engine(listPath)
	repo, err := e.LocalRepo(name, []string{downloadsFolder()})
	if err != nil || repo != "" {
		if repo != "" {
			fmt.Fprintf(c.stderr, "Found on this PC: %s\n", repo)
		}
		return repo, err
	}
	fmt.Fprintf(c.stderr, "Searching GitHub for '%s'...\n", name)
	repos, err := e.SearchRepos(name)
	if err != nil {
		return "", err
	}
	for i, r := range repos {
		if i == 5 || (i > 0 && !c.interactive) {
			break
		}
		about := fmt.Sprintf("★ %d", r.Stars)
		if r.Description != "" {
			about += ": " + shorten(r.Description, 70)
		}
		if c.Choose(fmt.Sprintf("Is it %s? %s", r.FullName, about), []string{"Yes", "No"}, 1) != 0 {
			continue
		}
		release, err := e.GitHub.Latest(r.FullName, false)
		if err != nil { // like GitHub's hourly limit: say so, rather than "no repository chosen"
			return "", err
		}
		if release != nil {
			return r.FullName, nil
		}
		fmt.Fprintf(c.stderr, "%s has no releases, so there's nothing to track.\n", r.FullName)
	}
	return "", fmt.Errorf("no repository chosen for '%s'. If you know it, add it as owner/repo, like reforge add doitsujin/dxvk", name)
}

// install installs an app that isn't installed yet from its latest release (asking first unless yes)
// and tracks it, by the name it gets in Installed apps.
func (c *cli) install(listPath string, apps []*applist.App, app *applist.App, yes bool) error {
	for _, a := range apps {
		if (a.IsInstaller() && strings.EqualFold(a.Repository, app.Repository)) || (app.Name != "" && strings.EqualFold(a.Name, app.Name)) {
			return fmt.Errorf("'%s' is already in the list. Update it with: reforge update \"%s\"", a.Name, a.Name)
		}
	}
	e := c.engine(listPath)
	e.CleanWorkRoot()
	r, track := e.Install(app, yes)
	if track {
		if applist.Find(apps, app.Name) != nil {
			c.warn(fmt.Sprintf("'%s' is already in the list, so this app isn't tracked. Track it with: reforge add %s --name \"<another name>\"", app.Name, app.Repository))
		} else if err := applist.Save(listPath, append(apps, app)); err != nil {
			return err
		}
	}
	c.failed = r.Status == core.Failed
	return c.printResults([]core.Result{r}, false)
}

// addFound finds apps and tools from GitHub on this PC that aren't tracked yet (installed apps that link
// to GitHub, and downloads from GitHub releases in the Downloads folder), and asks about each one.
func (c *cli) addFound(listPath string, apps []*applist.App) error {
	found, err := c.engine(listPath).Find([]string{downloadsFolder()}, func(repo string) bool { return isTracked(apps, repo) })
	if err != nil {
		return err
	}
	if len(found) == 0 {
		if len(apps) == 0 {
			fmt.Fprintln(c.stderr, "Nothing from GitHub found on this PC. Add apps and mods by name: reforge add <name>")
		} else {
			fmt.Fprintf(c.stderr, "Nothing new: everything from GitHub that Reforge can see is already tracked (%d in your list).\n", len(apps))
		}
		return nil
	}
	var rows [][]string
	for _, f := range found {
		rows = append(rows, []string{f.Name, f.Repository, f.Version, f.Type})
	}
	c.table([]string{"Name", "Repository", "Version", "Type"}, rows)
	if !c.interactive {
		fmt.Fprintln(c.stderr, "\nRun 'reforge add' in a terminal to pick which to track, or add one by name: reforge add <name>")
		return nil
	}
	added := 0
	for _, f := range found {
		question := fmt.Sprintf("Track %s (%s)?", f.Name, f.Repository)
		app := &applist.App{Name: f.Name, Repository: f.Repository}
		if f.Type == "Archive" { // a download: new versions are downloaded into the same folder
			question = fmt.Sprintf("Keep %s (%s) up to date in %s?", f.Name, f.Repository, f.Folder)
			app.Type, app.Folder, app.InstalledVersion = "download", f.Folder, f.Version
		}
		if applist.Find(apps, f.Name) == nil && c.Choose(question, []string{"Yes", "No"}, 1) == 0 {
			apps = append(apps, app)
			added++
		}
	}
	if added == 0 {
		return nil
	}
	if err := applist.Save(listPath, apps); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "Tracking %d more app(s).\n", added)
	return nil
}

func shorten(s string, n int) string {
	if r := []rune(strings.TrimSpace(s)); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return strings.TrimSpace(s)
}
