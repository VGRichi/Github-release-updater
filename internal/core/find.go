package core

import (
	"io/fs"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/VGRichi/Reforge/internal/archive"
	"github.com/VGRichi/Reforge/internal/pick"
	"github.com/VGRichi/Reforge/internal/system"
	"github.com/VGRichi/Reforge/internal/version"
)

// Candidate is an app or tool from GitHub that isn't tracked yet.
type Candidate struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Version    string `json:"version"`
	Type       string `json:"type"`             // "Installer" or "Archive"
	Folder     string `json:"folder,omitempty"` // archives: the folder the download is in
}

// Find lists apps and tools from GitHub that aren't tracked: installed apps whose uninstall information
// links to GitHub, and files downloaded from GitHub releases into the folders (two levels deep).
// Apps winget updates are left out, so they aren't updated twice.
func (e *Engine) Find(folders []string, tracked func(repo string) bool) ([]Candidate, error) {
	programs, err := e.installedPrograms()
	if err != nil {
		return nil, err
	}
	var winget []system.Package
	wingetLoaded := false
	isWinget := func(p *system.Program) bool {
		if !wingetLoaded { // winget is slow to ask, so only when there's something to check
			winget, wingetLoaded = e.System.WingetPackages(), true
		}
		// winget shortens names ("PowerShell 7-x64" is "PowerShell"), so match the start of the name and the version
		for _, w := range winget {
			if w.Name != "" && strings.HasPrefix(strings.ToLower(p.Name), strings.ToLower(w.Name)) && w.Version == p.Version {
				return true
			}
		}
		return false
	}
	var found []Candidate
	seen := map[string]bool{}
	add := func(c Candidate) {
		seen[strings.ToLower(c.Repository)] = true
		found = append(found, c)
	}
	skip := func(repo string) bool { return tracked(repo) || seen[strings.ToLower(repo)] }

	for i := range programs {
		p := &programs[i]
		if repo := repoFromLinks(p.Links); repo != "" && !skip(repo) && !isWinget(p) {
			add(Candidate{Name: BaseName(p.Name), Repository: repo, Version: p.Version, Type: "Installer"})
		}
	}
	for _, d := range e.downloads(folders) {
		if skip(d.repo) {
			continue
		}
		if pick.IsArchive(d.file) {
			add(Candidate{Name: path.Base(d.repo), Repository: d.repo, Version: d.tag, Type: "Archive", Folder: d.dir})
		} else if p := downloadedProgram(programs, d); p != nil && !isWinget(p) {
			add(Candidate{Name: BaseName(p.Name), Repository: d.repo, Version: p.Version, Type: "Installer"})
		}
	}
	return found, nil
}

type download struct{ file, dir, repo, tag string }

var (
	releasePage  = regexp.MustCompile(`https://github\.com/([\w.-]+/[\w.-]+)/releases/(?:tag|download)/([^/?#\s]+)`)
	downloadExts = regexp.MustCompile(`(?i)\.(msi|exe|zip|7z|tar\.gz|tgz|tar\.xz)$`)
)

// downloads lists files downloaded from GitHub releases, newest first so each repository's latest
// download wins. Browsers note where each download came from (on Windows, the Mark of the Web);
// GitHub serves release files from another host, so the repository and tag come from the release page.
func (e *Engine) downloads(folders []string) []download {
	type file struct {
		path string
		mod  int64
	}
	var files []file
	for _, folder := range folders {
		if !archive.IsDir(folder) {
			e.warn("Folder not found: %s", folder)
		}
	}
	eachFolder(folders, func(dir string, entries []fs.DirEntry) {
		for _, entry := range entries {
			if entry.IsDir() || !downloadExts.MatchString(entry.Name()) {
				continue
			}
			if info, err := entry.Info(); err == nil {
				files = append(files, file{filepath.Join(dir, entry.Name()), info.ModTime().UnixNano()})
			}
		}
	})
	sort.SliceStable(files, func(i, j int) bool { return files[i].mod > files[j].mod })
	var list []download
	for _, f := range files {
		m := releasePage.FindStringSubmatch(e.System.DownloadOrigin(f.path))
		if m == nil {
			continue
		}
		tag, err := url.PathUnescape(m[2])
		if err != nil {
			tag = m[2]
		}
		list = append(list, download{filepath.Base(f.path), filepath.Dir(f.path), m[1], tag})
	}
	return list
}

// downloadedProgram finds the installed app a downloaded installer belongs to: the release's version,
// and its name (first word, like "Notepad++") in the file or repository name. Both must match, so a
// download alone never suggests an app.
func downloadedProgram(programs []system.Program, d download) *system.Program {
	if version.Parse(d.tag) == nil {
		return nil
	}
	names := simple(d.file + d.repo)
	for i := range programs {
		if word := firstWord(programs[i].Name); len(word) >= 3 && strings.Contains(names, word) && version.Same(programs[i].Version, d.tag) {
			return &programs[i]
		}
	}
	return nil
}
