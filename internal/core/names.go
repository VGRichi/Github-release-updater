package core

import (
	"path"
	"regexp"
	"strings"

	"github.com/VGRichi/Reforge/internal/system"
)

// Matching names: apps' names in Installed apps, repositories' names, and file and folder names.

var (
	notAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)
	versionInName   = regexp.MustCompile(`(?i)(^|[^\w.])v?\d+(?:\.\d+)+[\w.+-]*`)
	spaces          = regexp.MustCompile(`\s{2,}`)
	repoLink        = regexp.MustCompile(`(?i)github\.com/([\w.-]+)/([\w.-]+)`)
	notOwners       = map[string]bool{"sponsors": true, "orgs": true, "apps": true, "features": true, "settings": true}
)

// simple is a name in lower case without punctuation, for comparing: "Optiscaler_0.9" is "optiscaler09".
func simple(s string) string { return strings.ToLower(notAlphanumeric.ReplaceAllString(s, "")) }

// firstWord is the simple form of a name's first word: "notepad" for "Notepad++ (64-bit x64)".
func firstWord(name string) string {
	if fields := strings.Fields(name); len(fields) > 0 {
		return simple(fields[0])
	}
	return ""
}

// BaseName leaves the version out of an installed name ("PhotoCraft 1.2.3 (x64)" is "PhotoCraft (x64)"),
// since it changes with every update.
func BaseName(name string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(versionInName.ReplaceAllString(name, "$1"), " "))
}

// FindProgram finds the installed app for a tracked name, closest first: the same name, then the same
// name once version numbers are left out, then the shortest name with the name in it (so "Git" is Git,
// not "GitHub Desktop").
func FindProgram(programs []system.Program, name string) *system.Program {
	lower, base := strings.ToLower(name), BaseName(name)
	var sameBase, part *system.Program
	for i := range programs {
		p := &programs[i]
		switch {
		case strings.EqualFold(p.Name, name):
			return p
		case base != "" && strings.EqualFold(BaseName(p.Name), base):
			if sameBase == nil {
				sameBase = p
			}
		case strings.Contains(strings.ToLower(p.Name), lower):
			if part == nil || len(p.Name) < len(part.Name) {
				part = p
			}
		}
	}
	if sameBase != nil {
		return sameBase
	}
	return part
}

// repoFromLinks is the GitHub repository an installed app's links point to, or "".
func repoFromLinks(links []string) string {
	m := repoLink.FindStringSubmatch(strings.Join(links, " "))
	if m == nil || notOwners[strings.ToLower(m[1])] {
		return ""
	}
	return m[1] + "/" + strings.TrimSuffix(m[2], ".git")
}

// namedLike reports whether a file or folder name contains the repository's name ("Optiscaler_0.9.4"
// for optiscaler/OptiScaler).
func namedLike(name, repo string) bool {
	leaf := simple(path.Base(repo))
	return leaf != "" && strings.Contains(simple(name), leaf)
}

// sameName reports whether a typed name and a repository's name are the same, with + spelled out
// ("optiscaler" and "OptiScaler", "notepad++" and "notepad-plus-plus").
func sameName(name, leaf string) bool {
	return simple(name) != "" && simple(searchWords(name)) == simple(leaf)
}

// searchWords is a name as words GitHub's search understands: it ignores symbols, so "notepad++" would
// find nothing, while "notepad plus plus" finds notepad-plus-plus.
func searchWords(name string) string {
	return strings.Join(strings.Fields(notAlphanumeric.ReplaceAllString(strings.ReplaceAll(name, "+", " plus "), " ")), " ")
}
