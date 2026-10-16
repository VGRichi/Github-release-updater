// Package pick chooses which file of a release to download.
package pick

import (
	"regexp"
	"strings"

	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/wildcard"
)

// Arch is the architecture to pick for, as Go names it: "amd64" or "arm64".
type Arch string

// How release file names spell architectures. Go's regexp has no lookbehind, so a bare "x86"
// (not "x86_64", not "_x86") is checked separately by hasBareX86.
var (
	x64Names = regexp.MustCompile(`(?i)x64|amd64|x86_64|win64|64-?bit`)
	armNames = regexp.MustCompile(`(?i)arm64|aarch64`)
	x86Names = regexp.MustCompile(`(?i)ia32|i386|32-?bit`)
)

func (a Arch) mine(name string) bool {
	if a == "arm64" {
		return armNames.MatchString(name)
	}
	return x64Names.MatchString(name)
}

func (a Arch) other(name string) bool {
	if a == "arm64" {
		return x64Names.MatchString(name) || x86Names.MatchString(name) || hasBareX86(name, true)
	}
	return armNames.MatchString(name) || x86Names.MatchString(name) || hasBareX86(name, false)
}

// hasBareX86 finds "x86" that isn't part of "_x86" or, unless allow64, "x86_64".
func hasBareX86(name string, allow64 bool) bool {
	lower := strings.ToLower(name)
	for i := strings.Index(lower, "x86"); i >= 0; {
		before := i > 0 && lower[i-1] == '_'
		after := strings.HasPrefix(lower[i+3:], "_64")
		if !before && (allow64 || !after) {
			return true
		}
		next := strings.Index(lower[i+3:], "x86")
		if next < 0 {
			break
		}
		i += 3 + next
	}
	return false
}

// words matches any of the words, but not inside a longer word: "emacs" doesn't count as "mac",
// but "linux64" counts as "linux".
func words(list ...string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(^|[^a-z])(` + strings.Join(list, "|") + `)([^a-z]|$)`)
}

var (
	installerExt  = regexp.MustCompile(`(?i)\.(msi|exe)$`)
	notInstaller  = regexp.MustCompile(`(?i)portable|\.(7z|zip)\.exe$`) // portable builds, self-extracting archives
	archiveExt    = regexp.MustCompile(`(?i)\.(zip|7z|tar\.gz|tgz|tar\.xz)$`)
	setupWord     = regexp.MustCompile(`(?i)setup|install`)
	windowsWord   = regexp.MustCompile(`(?i)win`)
	otherPlatform = words("mac", "macos", "osx", "darwin", "linux")
	notForArchive = words("source", "src", "debug", "pdb", "symbols", "linux", "mac", "macos", "osx", "darwin", "native", "steamrt")
)

// IsArchive reports whether the file is an archive Reforge can extract.
func IsArchive(name string) bool { return archiveExt.MatchString(name) }

// Installer picks the Windows installer for this PC: the right architecture, and the same kind (.msi or
// .exe) as what's installed. pattern, when set, picks by wildcard instead.
func Installer(assets []github.Asset, pattern string, wantMSI bool, arch Arch) *github.Asset {
	if pattern != "" {
		return byPattern(assets, pattern)
	}
	return best(assets, func(a github.Asset) (int, bool) {
		if !installerExt.MatchString(a.Name) || notInstaller.MatchString(a.Name) || otherPlatform.MatchString(a.Name) {
			return 0, false
		}
		score := archScore(a.Name, arch)
		if strings.HasSuffix(strings.ToLower(a.Name), ".msi") == wantMSI {
			score += 20
		}
		if setupWord.MatchString(a.Name) {
			score += 2
		}
		return score, true
	})
}

// Archive picks the .zip, .7z or .tar.gz for Windows on this PC, skipping source code, debug symbols
// and builds for other platforms.
func Archive(assets []github.Asset, pattern string, arch Arch) *github.Asset {
	if pattern != "" {
		return byPattern(assets, pattern)
	}
	return best(assets, func(a github.Asset) (int, bool) {
		if !archiveExt.MatchString(a.Name) ||
			notForArchive.MatchString(a.Name) {
			return 0, false
		}
		score := archScore(a.Name, arch)
		if windowsWord.MatchString(a.Name) {
			score += 5
		}
		return score, true
	})
}

func archScore(name string, arch Arch) int {
	switch {
	case arch.mine(name):
		return 10
	case arch.other(name):
		return -100
	}
	return 0
}

// best returns the highest-scoring asset, never one for another architecture (a negative score).
// Ties go to the earlier file in the release.
func best(assets []github.Asset, score func(github.Asset) (int, bool)) *github.Asset {
	var top *github.Asset
	high := -1
	for i := range assets {
		if s, ok := score(assets[i]); ok && s > high {
			top, high = &assets[i], s
		}
	}
	return top
}

func byPattern(assets []github.Asset, pattern string) *github.Asset {
	for i := range assets {
		if wildcard.Match(pattern, assets[i].Name) {
			return &assets[i]
		}
	}
	return nil
}
