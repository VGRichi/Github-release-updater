package core

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/VGRichi/Reforge/internal/pick"
	"github.com/VGRichi/Reforge/internal/system"
	"github.com/VGRichi/Reforge/internal/version"
)

// FindProgram finds the installed app for a tracked name: the name as part of the installed name, or
// else the same name once version numbers are left out.
func FindProgram(programs []system.Program, name string) *system.Program {
	lower := strings.ToLower(name)
	for i := range programs {
		if strings.Contains(strings.ToLower(programs[i].Name), lower) {
			return &programs[i]
		}
	}
	if base := BaseName(name); base != "" {
		for i := range programs {
			if strings.EqualFold(BaseName(programs[i].Name), base) {
				return &programs[i]
			}
		}
	}
	return nil
}

var (
	versionInName = regexp.MustCompile(`(?i)(^|[^\w.])v?\d+(?:\.\d+)+[\w.+-]*`)
	spaces        = regexp.MustCompile(`\s{2,}`)
)

// BaseName leaves the version out of an installed name ("PhotoCraft 1.2.3 (x64)" is "PhotoCraft (x64)"),
// since it changes with every update.
func BaseName(name string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(versionInName.ReplaceAllString(name, "$1"), " "))
}

// Candidate is an app or tool from GitHub that isn't tracked yet.
type Candidate struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Version    string `json:"version"`
	Type       string `json:"type"`             // "Installer" or "Archive"
	Folder     string `json:"folder,omitempty"` // archives: the folder the download is in
}

var (
	repoLink     = regexp.MustCompile(`(?i)github\.com/([\w.-]+)/([\w.-]+)`)
	notOwners    = map[string]bool{"sponsors": true, "orgs": true, "apps": true, "features": true, "settings": true}
	releasePage  = regexp.MustCompile(`https://github\.com/([\w.-]+/[\w.-]+)/releases/(?:tag|download)/([^/?#\s]+)`)
	downloadExts = regexp.MustCompile(`(?i)\.(msi|exe|zip|7z|tar\.gz|tgz|tar\.xz)$`)
)

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
	seen := map[string]bool{}
	skip := func(repo string) bool { return tracked(repo) || seen[strings.ToLower(repo)] }
	var found []Candidate

	for i := range programs {
		p := &programs[i]
		m := repoLink.FindStringSubmatch(strings.Join(p.Links, " "))
		if m == nil || notOwners[strings.ToLower(m[1])] {
			continue
		}
		repo := m[1] + "/" + strings.TrimSuffix(m[2], ".git")
		if skip(repo) || isWinget(p) {
			continue
		}
		seen[strings.ToLower(repo)] = true
		found = append(found, Candidate{Name: BaseName(p.Name), Repository: repo, Version: p.Version, Type: "Installer"})
	}

	for _, d := range e.downloads(folders) {
		if skip(d.repo) {
			continue
		}
		if pick.IsArchive(d.file) {
			seen[strings.ToLower(d.repo)] = true
			found = append(found, Candidate{Name: repoLeaf(d.repo), Repository: d.repo, Version: d.tag, Type: "Archive", Folder: d.dir})
			continue
		}
		p := downloadedProgram(programs, d)
		if p == nil || isWinget(p) {
			continue
		}
		seen[strings.ToLower(d.repo)] = true
		found = append(found, Candidate{Name: BaseName(p.Name), Repository: d.repo, Version: p.Version, Type: "Installer"})
	}
	return found, nil
}

type download struct{ file, dir, repo, tag string }

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
		if _, err := os.Stat(folder); err != nil {
			e.warn("Folder not found: %s", folder)
			continue
		}
		filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(folder, path)
			depth := strings.Count(rel, string(filepath.Separator))
			if d.IsDir() {
				if depth >= 2 && path != folder {
					return filepath.SkipDir // two levels of subfolders, like Downloads\Mods\Game
				}
				return nil
			}
			if downloadExts.MatchString(d.Name()) {
				if info, err := d.Info(); err == nil {
					files = append(files, file{path, info.ModTime().UnixNano()})
				}
			}
			return nil
		})
	}
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

var notAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

// downloadedProgram finds the installed app a downloaded installer belongs to: the release's version,
// and its name (first word, like "Notepad++") in the file or repository name. Both must match, so a
// download alone never suggests an app.
func downloadedProgram(programs []system.Program, d download) *system.Program {
	if version.Parse(d.tag) == nil {
		return nil
	}
	names := strings.ToLower(notAlphanumeric.ReplaceAllString(d.file+d.repo, ""))
	for i := range programs {
		fields := strings.Fields(programs[i].Name)
		if len(fields) == 0 {
			continue
		}
		word := strings.ToLower(notAlphanumeric.ReplaceAllString(fields[0], ""))
		if len(word) >= 3 && strings.Contains(names, word) && version.Same(programs[i].Version, d.tag) {
			return &programs[i]
		}
	}
	return nil
}
