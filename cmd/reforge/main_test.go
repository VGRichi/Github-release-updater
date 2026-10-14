package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/core"
	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/system"
)

type fakeReleases struct{ release *github.Release }

func (f *fakeReleases) Latest(string, bool) (*github.Release, error) { return f.release, nil }
func (f *fakeReleases) Download(a github.Asset, dir string) (string, error) {
	file := filepath.Join(dir, a.Name)
	return file, os.WriteFile(file, []byte("bytes"), 0o644)
}

type fakeSystem struct{ programs []system.Program }

func (s *fakeSystem) Programs() ([]system.Program, error)      { return s.programs, nil }
func (s *fakeSystem) RunInstaller(string, string) (int, error) { return 0, nil }
func (s *fakeSystem) IsAdmin() bool                            { return false }
func (s *fakeSystem) Signer(string) (string, error)            { return "", nil }
func (s *fakeSystem) DownloadOrigin(string) string             { return "" }
func (s *fakeSystem) WingetPackages() []system.Package         { return nil }

// useFakes makes the commands use a fake GitHub (one release) and a PC with these installed apps.
func useFakes(t *testing.T, rel *github.Release, programs ...system.Program) {
	real := newEngine
	t.Cleanup(func() { newEngine = real })
	newEngine = func(c *cli) *core.Engine {
		return &core.Engine{
			GitHub: &fakeReleases{rel}, System: &fakeSystem{programs}, Ask: c, Arch: "amd64",
			WorkRoot: t.TempDir(), BackupsToKeep: 3, Progress: c.progress, Verbose: c.detail, Warn: c.warn,
		}
	}
}

// reforge runs the command with a list file in a temp folder and returns exit code, stdout, stderr.
func reforge(t *testing.T, config string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append(args, "--config", config), strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

func configIn(t *testing.T) string { return filepath.Join(t.TempDir(), "apps.json") }

func TestHelpVersionAndUnknownCommands(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, nil, &out, &errOut); code != 0 || !strings.Contains(out.String(), "Commands:") {
		t.Errorf("no arguments: %d %q", code, out.String())
	}
	out.Reset()
	if code := run([]string{"version"}, nil, &out, &errOut); code != 0 || out.String() != "reforge dev\n" {
		t.Errorf("version: %d %q", code, out.String())
	}
	if code := run([]string{"bogus"}, nil, &out, &errOut); code != 2 {
		t.Errorf("unknown command: %d", code)
	}
	errOut.Reset()
	if code := run([]string{"add", "--help"}, nil, &out, &errOut); code != 0 || !strings.Contains(errOut.String(), "--dest <value>") {
		t.Errorf("add --help: %d %q", code, errOut.String())
	}
	if code := run([]string{"add", "--nope"}, nil, &out, &errOut); code != 2 {
		t.Errorf("unknown flag: %d", code)
	}
}

func TestAddListRemove(t *testing.T) {
	config := configIn(t)
	game := t.TempDir()
	if code, out, errOut := reforge(t, config, "add", "https://github.com/storytold/photocraft", "--name", "PhotoCraft"); code != 0 || out != "Tracking PhotoCraft (storytold/photocraft)\n" {
		t.Fatalf("add: %d %q %q", code, out, errOut)
	}
	// Flags before or after the repository, given more than once
	code, _, errOut := reforge(t, config, "add", "--dest", game, "doitsujin/dxvk", "--subfolder", "x64",
		"--keep", "*.conf", "--rename", "a.dll=b.dll", "--include", "*.dll", "--include", "*.conf", "--name", "DXVK (Game)")
	if code != 0 {
		t.Fatalf("add archive: %d %q", code, errOut)
	}
	apps, _ := applist.Load(config)
	dxvk := apps[1]
	if !dxvk.IsArchive() || dxvk.Destination[0] != game || dxvk.Subfolder != "x64" || dxvk.Rename["a.dll"] != "b.dll" || len(dxvk.Include) != 2 {
		t.Fatalf("stored: %+v", dxvk)
	}

	_, out, _ := reforge(t, config, "list", "DXVK*", "--offline")
	if !strings.Contains(out, "DXVK (Game)") || strings.Contains(out, "PhotoCraft") || !strings.Contains(out, game) {
		t.Errorf("list: %q", out)
	}

	code, out, errOut = reforge(t, config, "remove", "DXVK*", "Nope")
	if code != 1 || out != "Stopped tracking DXVK (Game)\n" || !strings.Contains(errOut, "no app named 'Nope'") {
		t.Errorf("remove: %d %q %q", code, out, errOut)
	}
	if apps, _ := applist.Load(config); len(apps) != 1 || apps[0].Name != "PhotoCraft" {
		t.Errorf("left: %v", apps)
	}
}

func TestAddRefuses(t *testing.T) {
	config := configIn(t)
	reforge(t, config, "add", "storytold/photocraft")
	tests := map[string][]string{
		"a name already in the list":  {"add", "o/photocraft"},
		"something that isn't a repo": {"add", "not a repo"},
		"a folder that doesn't exist": {"add", "o/r", "--dest", filepath.Join(t.TempDir(), "missing")},
		"a bad --rename":              {"add", "o/r", "--rename", "nope"},
		"no repository":               {"add"},
	}
	for desc, args := range tests {
		if code, _, _ := reforge(t, config, args...); code == 0 {
			t.Errorf("%s: want an error", desc)
		}
	}
}

func TestAddForceKeepsWhatWasLearned(t *testing.T) {
	config := configIn(t)
	applist.Save(config, []*applist.App{{Name: "Tool", Repository: "o/tool", Type: "archive", Destination: applist.Strings{t.TempDir()}, HadDigest: true, InstalledVersion: "v1"}})
	reforge(t, config, "add", "o/tool", "--name", "Tool", "--dest", t.TempDir(), "--force")
	apps, _ := applist.Load(config)
	if len(apps) != 1 || !apps[0].HadDigest || apps[0].InstalledVersion != "v1" {
		t.Errorf("got %+v", apps[0])
	}
}

func TestUpdate(t *testing.T) {
	config := configIn(t)
	applist.Save(config, []*applist.App{{Name: "PhotoCraft", Repository: "storytold/photocraft"}, {Name: "Missing", Repository: "o/missing"}})
	useFakes(t, &github.Release{Tag: "v1.1.0", Assets: []github.Asset{{Name: "PhotoCraft-x64.msi", Digest: "sha256:x"}}},
		system.Program{Name: "PhotoCraft", Version: "1.0.0", PerUser: true, MSI: true})

	// list checks for updates, in words, and changes nothing
	code, out, _ := reforge(t, config, "list")
	if code != 0 || !strings.Contains(out, "1.0.0  ") || !strings.Contains(out, "v1.1.0") ||
		!strings.Contains(out, "Update available") || !strings.Contains(out, "Not installed") {
		t.Fatalf("list: %d %q", code, out)
	}
	if apps, _ := applist.Load(config); apps[0].HadDigest {
		t.Error("list must not save anything")
	}
	var entries []listEntry
	if _, out, _ := reforge(t, config, "list", "--json"); json.Unmarshal([]byte(out), &entries) != nil || entries[0].Status != "UpdateAvailable" {
		t.Fatalf("list --json: %q", out)
	}
	if code, _, _ := reforge(t, config, "update", "--dry-run"); code != 2 {
		t.Error("update --dry-run is gone; reforge list does that")
	}

	code, out, _ = reforge(t, config, "update", "Photo*", "--json")
	var results []core.Result
	if err := json.Unmarshal([]byte(out), &results); err != nil || code != 0 || len(results) != 1 || results[0].Status != core.Updated {
		t.Fatalf("json: %d %q %v", code, out, err)
	}
	if apps, _ := applist.Load(config); !apps[0].HadDigest {
		t.Error("what an update learns should be saved")
	}

	if code, _, errOut := reforge(t, config, "update", "Nope"); code != 1 || !strings.Contains(errOut, "no app named 'Nope'") {
		t.Errorf("unknown name: %d %q", code, errOut)
	}
}

func TestUpdateWithNobodyToAsk(t *testing.T) {
	// Input isn't a terminal (like a scheduled task), so an unverified download gets the safe answer: No
	config := configIn(t)
	applist.Save(config, []*applist.App{{Name: "PhotoCraft", Repository: "storytold/photocraft"}})
	useFakes(t, &github.Release{Tag: "v1.1.0", Assets: []github.Asset{{Name: "PhotoCraft-x64.exe"}}},
		system.Program{Name: "PhotoCraft", Version: "1.0.0", PerUser: true})
	code, out, errOut := reforge(t, config, "update")
	if code != 0 || !strings.Contains(out, "Unverified download declined") || !strings.Contains(errOut, "can't ask here") {
		t.Errorf("%d %q %q", code, out, errOut)
	}
}

func TestInstallTracksTheApp(t *testing.T) {
	config := configIn(t)
	useFakes(t, &github.Release{Tag: "v1.1.0", Assets: []github.Asset{{Name: "PhotoCraft-x64.msi"}}})
	// No checksum and nobody to ask: nothing is installed or tracked
	if code, out, _ := reforge(t, config, "install", "storytold/photocraft", "--name", "PhotoCraft", "--allow-unverified", "--yes"); code != 0 || !strings.Contains(out, "Installed") {
		t.Fatalf("%d %q", code, out)
	}
	apps, _ := applist.Load(config)
	if len(apps) != 1 || apps[0].Name != "PhotoCraft" || !apps[0].AllowUnverified {
		t.Fatalf("list: %v", apps)
	}
	if code, _, errOut := reforge(t, config, "install", "storytold/photocraft"); code != 1 || !strings.Contains(errOut, "already in the list") {
		t.Errorf("second install: %d %q", code, errOut)
	}
}

func TestFindAddAsksWithNobodyToAnswer(t *testing.T) {
	config := configIn(t)
	useFakes(t, nil, system.Program{Name: "PhotoCraft", Version: "1.0", Links: []string{"https://github.com/storytold/photocraft"}})
	code, out, _ := reforge(t, config, "find", t.TempDir(), "--add")
	if code != 0 || !strings.Contains(out, "storytold/photocraft") {
		t.Fatalf("%d %q", code, out)
	}
	if apps, _ := applist.Load(config); len(apps) != 0 {
		t.Error("nobody answered, so nothing should be tracked")
	}
}

func TestParseFlagsAnywhere(t *testing.T) {
	c := newCLI(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	fs := c.flags("list")
	asJSON := fs.Bool("json", false, "")
	pos, err := parse(fs, []string{"a", "--json", "b"})
	if err != nil || !*asJSON || strings.Join(pos, ",") != "a,b" {
		t.Errorf("got %v %v %v", pos, *asJSON, err)
	}
}
