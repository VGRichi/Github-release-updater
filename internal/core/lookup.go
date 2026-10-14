package core

import (
	"sort"
	"strings"

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
		if m := repoLink.FindStringSubmatch(strings.Join(p.Links, " ")); m != nil && !notOwners[strings.ToLower(m[1])] {
			return m[1] + "/" + strings.TrimSuffix(m[2], ".git"), nil
		}
	}
	for _, d := range e.downloads(folders) {
		if sameName(name, repoLeaf(d.repo)) {
			return d.repo, nil
		}
	}
	return "", nil
}

// sameName reports whether a typed name and a repository's name are the same, ignoring case and
// punctuation, with + spelled out ("optiscaler" and "OptiScaler", "notepad++" and "notepad-plus-plus").
func sameName(name, leaf string) bool {
	clean := func(s string) string { return strings.ToLower(notAlphanumeric.ReplaceAllString(s, "")) }
	return clean(name) != "" && clean(searchWords(name)) == clean(leaf)
}

// searchWords is a name as words GitHub's search understands: it ignores symbols, so "notepad++" would
// find nothing, while "notepad plus plus" finds notepad-plus-plus.
func searchWords(name string) string {
	return strings.Join(strings.Fields(notAlphanumeric.ReplaceAllString(strings.ReplaceAll(name, "+", " plus "), " ")), " ")
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
		case sameName(name, repoLeaf(r.FullName)):
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
