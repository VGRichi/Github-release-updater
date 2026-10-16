package main

import (
	"fmt"
	"os"
	"os/exec"
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

// What the commands share: the engine, the list file, and matching names. The commands are in add.go
// (add) and list.go (list, update, remove).

// downloadsFolder is where add looks for downloads; tests replace it.
var downloadsFolder = system.DownloadsFolder

// newEngine connects the engine to the real GitHub and OS; tests replace it.
var newEngine = func(c *cli) *core.Engine {
	return &core.Engine{
		GitHub: github.New(githubToken), System: system.New(), Ask: c, Arch: pick.Arch(runtime.GOARCH),
		WorkRoot: os.TempDir(), BackupsToKeep: 3,
		Progress: c.progress, Verbose: c.detail, Warn: c.warn,
	}
}

// engine returns the command's engine (one per command, so the installed apps are read once), with
// backups and a cache of recent GitHub answers next to the list file. run saves the cache at the end.
func (c *cli) engine(listPath string) *core.Engine {
	if c.eng != nil {
		return c.eng
	}
	c.eng = newEngine(c)
	dir := filepath.Dir(listPath)
	c.eng.BackupRoot = filepath.Join(dir, "backups")
	if gh, ok := c.eng.GitHub.(*github.Client); ok {
		gh.Cache = github.OpenCache(filepath.Join(dir, "cache.json"))
		c.cache = gh.Cache
	}
	return c.eng
}

// githubToken is the token to use with GitHub's API: GITHUB_TOKEN or GH_TOKEN, or else the GitHub CLI's
// sign-in (gh auth token) if it's installed. With one, GitHub allows 5,000 requests an hour instead of 60.
func githubToken() string {
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if token := os.Getenv(name); token != "" {
			return token
		}
	}
	if gh, err := exec.LookPath("gh"); err == nil {
		if out, err := exec.Command(gh, "auth", "token").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

// load reads the list file: --config, or the default one.
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

// matching returns the apps whose names match any of the patterns (all apps without patterns), and
// warns about patterns that match nothing.
func (c *cli) matching(apps []*applist.App, patterns []string) []*applist.App {
	if len(patterns) == 0 {
		return apps
	}
	var found []*applist.App
	used := make([]bool, len(patterns))
	for _, a := range apps {
		matched := false
		for i, p := range patterns {
			if wildcard.Match(p, a.Name) {
				matched, used[i] = true, true
			}
		}
		if matched {
			found = append(found, a)
		}
	}
	for i, p := range patterns {
		if !used[i] {
			c.warn(fmt.Sprintf("no app named '%s' in the list. Check the names with 'reforge list'.", p))
			c.failed = true
		}
	}
	return found
}

// isTracked reports whether an app from repo is in the list.
func isTracked(apps []*applist.App, repo string) bool {
	for _, a := range apps {
		if strings.EqualFold(a.Repository, repo) {
			return true
		}
	}
	return false
}

// absFolder is the full path of an existing folder.
func absFolder(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if info, statErr := os.Stat(abs); err != nil || statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("folder not found: %s", dir)
	}
	return abs, nil
}
