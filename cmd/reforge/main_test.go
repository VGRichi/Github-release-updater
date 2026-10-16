package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/core"
	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/system"
)

type fakeReleases struct {
	release *github.Release
	repos   []github.Repo
	err     error // what Latest fails with, like GitHub's hourly limit
}

func (f *fakeReleases) Latest(string, bool) (*github.Release, error) { return f.release, f.err }
func (f *fakeReleases) Search(string) ([]github.Repo, error)         { return f.repos, nil }
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

// useFakes makes the commands use a fake GitHub (one release), a PC with these installed apps, and an
// empty Downloads folder, which it returns.
func useFakes(t *testing.T, rel *github.Release, programs ...system.Program) string {
	realEngine, realDownloads := newEngine, downloadsFolder
	t.Cleanup(func() { newEngine, downloadsFolder = realEngine, realDownloads })
	downloads := t.TempDir()
	downloadsFolder = func() string { return downloads }
	newEngine = func(c *cli) *core.Engine {
		return &core.Engine{
			GitHub: &fakeReleases{release: rel}, System: &fakeSystem{programs}, Ask: c, Arch: "amd64",
			WorkRoot: t.TempDir(), BackupsToKeep: 3, Progress: c.progress, Verbose: c.detail, Warn: c.warn,
		}
	}
	return downloads
}

// photoCraft is a release with an installer and an archive, for an app that's installed.
func photoCraft(t *testing.T) string {
	return useFakes(t, &github.Release{Tag: "v1.1.0", Assets: []github.Asset{{Name: "PhotoCraft-x64.msi"}, {Name: "PhotoCraft-win64.zip"}}},
		system.Program{Name: "PhotoCraft 1.0.0", Version: "1.0.0", PerUser: true, MSI: true})
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
	if strings.Contains(out.String(), "  version ") {
		t.Error("the banner shows the version; no version command in the help")
	}
	out.Reset()
	if code := run([]string{"--version"}, nil, &out, &errOut); code != 0 || out.String() != "reforge dev\n" {
		t.Errorf("--version: %d %q", code, out.String())
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
	photoCraft(t)
	config := configIn(t)
	game := t.TempDir()
	// Installed, and its release has an installer: tracked by the name Installed apps shows
	if code, out, errOut := reforge(t, config, "add", "https://github.com/storytold/photocraft"); code != 0 ||
		out != "Tracking PhotoCraft (storytold/photocraft): updated with its installer\n" {
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
	if !apps[0].IsInstaller() || !dxvk.IsArchive() || dxvk.Destination[0] != game || dxvk.Subfolder != "x64" || dxvk.Rename["a.dll"] != "b.dll" || len(dxvk.Include) != 2 {
		t.Fatalf("stored: %+v %+v", apps[0], dxvk)
	}

	_, out, _ := reforge(t, config, "list", "DXVK*")
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
	photoCraft(t)
	config := configIn(t)
	reforge(t, config, "add", "storytold/photocraft")
	tests := map[string][]string{
		"a name already in the list":  {"add", "o/photocraft"},
		"something that isn't a repo": {"add", "not a repo"},
		"a folder that doesn't exist": {"add", "o/r", filepath.Join(t.TempDir(), "missing")},
		"a --dest that doesn't exist": {"add", "o/r", "--dest", filepath.Join(t.TempDir(), "missing")},
		"a folder and --dest":         {"add", "o/r", t.TempDir(), "--dest", t.TempDir()},
		"a bad --rename":              {"add", "o/r", "--rename", "nope"},
		"--dest without a repository": {"add", "--dest", t.TempDir()},
	}
	for desc, args := range tests {
		if code, _, _ := reforge(t, config, args...); code == 0 {
			t.Errorf("%s: want an error", desc)
		}
	}
}

func TestAddFindsADownloadedToolInDownloads(t *testing.T) {
	// DXVK's releases are archives; you extracted 3.1.1 into Downloads\Mods
	downloads := useFakes(t, &github.Release{Tag: "v3.1.2", Assets: []github.Asset{{Name: "dxvk-3.1.2.tar.gz"}}})
	mods := filepath.Join(downloads, "Mods")
	os.MkdirAll(filepath.Join(mods, "dxvk-3.1.1", "x64"), 0o755)
	config := configIn(t)
	code, out, errOut := reforge(t, config, "add", "doitsujin/dxvk")
	if code != 0 || !strings.Contains(out, "you have 3.1.1 in "+mods) {
		t.Fatalf("%d %q %q", code, out, errOut)
	}
	apps, _ := applist.Load(config)
	if !apps[0].IsDownload() || apps[0].Folder != mods || apps[0].InstalledVersion != "3.1.1" || apps[0].Name != "dxvk" {
		t.Fatalf("stored: %+v", apps[0])
	}
	if _, out, _ := reforge(t, config, "list"); !strings.Contains(out, "3.1.1") || !strings.Contains(out, mods) {
		t.Errorf("list: %q", out)
	}

	// Not downloaded yet: it goes into Downloads
	_, out, _ = reforge(t, configIn(t), "add", "optiscaler/OptiScaler")
	if !strings.Contains(out, "the next update downloads it into "+downloads) {
		t.Errorf("not downloaded yet: %q", out)
	}
}

func TestAddWithAFolder(t *testing.T) {
	useFakes(t, nil) // no release check needed: the folder says it's a download
	config := configIn(t)
	mods := t.TempDir()
	os.WriteFile(filepath.Join(mods, "Optiscaler_0.9.4-final.7z"), nil, 0o644)
	if code, out, errOut := reforge(t, config, "add", "optiscaler/OptiScaler", mods); code != 0 || !strings.Contains(out, "you have 0.9.4") {
		t.Fatalf("%d %q %q", code, out, errOut)
	}
}

func TestAddInstallsAnAppThatIsNotInstalled(t *testing.T) {
	useFakes(t, &github.Release{Tag: "v1.1.0", Assets: []github.Asset{{Name: "PhotoCraft-x64.msi", Digest: "sha256:x"}}})
	config := configIn(t)
	// Nobody to answer "Install it?": nothing is installed or tracked
	if code, out, errOut := reforge(t, config, "add", "storytold/photocraft"); code != 0 || !strings.Contains(out, "Not installed") || !strings.Contains(errOut, "can't ask here: Install PhotoCraft-x64.msi") {
		t.Fatalf("%d %q %q", code, out, errOut)
	}
	if apps, _ := applist.Load(config); len(apps) != 0 {
		t.Fatalf("tracked without installing: %v", apps)
	}
	// With --yes it's installed and tracked
	if code, out, _ := reforge(t, config, "add", "storytold/photocraft", "--name", "PhotoCraft", "--yes"); code != 0 || !strings.Contains(out, "Installed") {
		t.Fatalf("%d %q", code, out)
	}
	if apps, _ := applist.Load(config); len(apps) != 1 || apps[0].Name != "PhotoCraft" {
		t.Fatalf("list: %v", apps)
	}
}

func TestAddForceKeepsWhatWasLearned(t *testing.T) {
	config := configIn(t)
	useFakes(t, nil) // never the real GitHub
	applist.Save(config, []*applist.App{{Name: "Tool", Repository: "o/tool", Type: "archive", Destination: applist.Strings{t.TempDir()}, HadDigest: true, InstalledVersion: "v1"}})
	if code, _, errOut := reforge(t, config, "add", "o/tool", "--name", "Tool", "--dest", t.TempDir(), "--force"); code != 0 {
		t.Fatalf("%d %q", code, errOut)
	}
	if code, _, _ := reforge(t, config, "add", "o/tool", "--dest", t.TempDir(), "--rename", `a.dll=..\b.dll`); code != 2 {
		t.Error("a rename to a path must be refused")
	}
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

func TestAddWithoutANameListsWhatItFinds(t *testing.T) {
	config := configIn(t)
	useFakes(t, nil, system.Program{Name: "PhotoCraft", Version: "1.0", Links: []string{"https://github.com/storytold/photocraft"}})
	code, out, errOut := reforge(t, config, "add")
	if code != 0 || !strings.Contains(out, "storytold/photocraft") || !strings.Contains(errOut, "Run 'reforge add' in a terminal") {
		t.Fatalf("%d %q %q", code, out, errOut)
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

func TestAddByNameAsksAboutGitHubMatches(t *testing.T) {
	useFakes(t, &github.Release{Tag: "v3.1.1", Assets: []github.Asset{{Name: "dxvk-3.1.1.tar.gz"}}})
	repos := []github.Repo{{FullName: "ValveSoftware/dxvk", Stars: 488}, {FullName: "doitsujin/dxvk", Stars: 18285, Description: "Vulkan-based D3D"}}
	realEngine := newEngine
	newEngine = func(c *cli) *core.Engine {
		e := realEngine(c)
		e.GitHub.(*fakeReleases).repos = repos
		return e
	}

	// The likeliest is asked about first (doitsujin/dxvk, more stars); someone answers No, then Yes
	var out, errOut bytes.Buffer
	c := newCLI(strings.NewReader("n\ny\n"), &out, &errOut)
	c.interactive = true
	repo, err := c.lookUp(configIn(t), "dxvk")
	if err != nil || repo != "ValveSoftware/dxvk" {
		t.Fatalf("got %q, %v", repo, err)
	}
	if q := errOut.String(); !strings.Contains(q, "Is it doitsujin/dxvk? ★ 18285: Vulkan-based D3D") {
		t.Errorf("questions: %q", q)
	}

	// Nobody to answer: the safe answer is No, asked once
	code, _, errOut2 := reforge(t, configIn(t), "add", "dxvk")
	if code != 1 || strings.Count(errOut2, "can't ask here") != 1 || !strings.Contains(errOut2, "add it as owner/repo") {
		t.Errorf("%d %q", code, errOut2)
	}
}

func TestChooseStopsAskingWhenInputEnds(t *testing.T) {
	var errOut bytes.Buffer
	c := newCLI(strings.NewReader(""), &bytes.Buffer{}, &errOut)
	c.interactive = true // looked like a terminal, but the input is empty
	for range 3 {
		if c.Choose("Track it?", []string{"Yes", "No"}, 1) != 1 {
			t.Fatal("want the default, No")
		}
	}
	if strings.Count(errOut.String(), "[Y] Yes") != 1 || strings.Count(errOut.String(), "can't ask here") != 2 {
		t.Errorf("asked once, then answered No without asking: %q", errOut.String())
	}
}

// lookUpWith runs the name lookup with GitHub returning these matches, the latest release (nil: none)
// or an error, and someone answering with the given lines.
func lookUpWith(t *testing.T, repos []github.Repo, release *github.Release, err error, answers string) (string, string, error) {
	useFakes(t, release)
	realEngine := newEngine
	newEngine = func(c *cli) *core.Engine {
		e := realEngine(c)
		gh := e.GitHub.(*fakeReleases)
		gh.repos, gh.err = repos, err
		return e
	}
	var errOut bytes.Buffer
	c := newCLI(strings.NewReader(answers), &bytes.Buffer{}, &errOut)
	c.interactive = true
	repo, lookErr := c.lookUp(configIn(t), "dxvk")
	return repo, errOut.String(), lookErr
}

func TestAddByNameShowsGitHubErrors(t *testing.T) {
	limit := errors.New("GitHub's limit of 60 checks an hour was reached; it resets in 9 minutes")
	_, _, err := lookUpWith(t, []github.Repo{{FullName: "doitsujin/dxvk", Stars: 18285}}, nil, limit, "y\n")
	if err != limit {
		t.Errorf("want GitHub's error, not %v", err)
	}
}

func TestAddByNameChecksOnlyTheChosenMatch(t *testing.T) {
	// Picked, but it has no releases: says so, and asks about the next match
	repos := []github.Repo{{FullName: "ValveSoftware/dxvk", Stars: 488}, {FullName: "doitsujin/dxvk", Stars: 18285}}
	repo, errOut, err := lookUpWith(t, repos, nil, nil, "y\ny\n")
	if repo != "" || err == nil || !strings.Contains(errOut, "ValveSoftware/dxvk has no releases") || !strings.Contains(errOut, "Is it doitsujin/dxvk?") {
		t.Errorf("got %q, %v, %q", repo, err, errOut)
	}
}

func TestAddWithoutANameSaysWhyNothingIsNew(t *testing.T) {
	useFakes(t, nil)
	config := configIn(t)
	if _, _, errOut := reforge(t, config, "add"); !strings.Contains(errOut, "Nothing from GitHub found on this PC") {
		t.Errorf("empty list: %q", errOut)
	}
	applist.Save(config, []*applist.App{{Name: "A", Repository: "o/a"}, {Name: "B", Repository: "o/b"}})
	if _, _, errOut := reforge(t, config, "add"); !strings.Contains(errOut, "already tracked (2 in your list)") {
		t.Errorf("tracked apps: %q", errOut)
	}
}
