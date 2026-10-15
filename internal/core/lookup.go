package core

import (
	"path"
	"sort"

	"github.com/VGRichi/Reforge/internal/github"
)

// LocalRepo finds the GitHub repository for a name on this PC: an installed app named like it that
// links to GitHub, or a download from a GitHub release named like it. It returns "" with neither.
func (e *Engine) LocalRepo(name string, folders []string) (string, error) {
	programs, err := e.installedPrograms()
	if err != nil {
		return "", err
	}
	if p := FindProgram(programs, name); p != nil {
		if repo := repoFromLinks(p.Links); repo != "" {
			return repo, nil
		}
	}
	for _, d := range e.downloads(folders) {
		if sameName(name, path.Base(d.repo)) {
			return d.repo, nil
		}
	}
	return "", nil
}

// SearchRepos searches GitHub for repositories with the name, most likely first: exactly that name
// first, then by stars; archived ones last.
func (e *Engine) SearchRepos(name string) ([]github.Repo, error) {
	repos, err := e.GitHub.Search(searchWords(name))
	if err != nil {
		return nil, err
	}
	rank := func(r github.Repo) int {
		switch {
		case r.Archived:
			return 2
		case sameName(name, path.Base(r.FullName)):
			return 0
		}
		return 1
	}
	sort.SliceStable(repos, func(i, j int) bool {
		if ri, rj := rank(repos[i]), rank(repos[j]); ri != rj {
			return ri < rj
		}
		return repos[i].Stars > repos[j].Stars
	})
	return repos, nil
}
