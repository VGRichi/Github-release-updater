package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/system"
)

// A Downloads\Mods folder with DXVK 3.1.1's archive, and a newer release with a wrapper folder inside.
func downloadCase(t *testing.T) (*testEngine, *applist.App, string) {
	te := newEngine(t)
	te.gh.payload = zipOf(map[string]string{"dxvk-3.1.2/x64/d3d11.dll": "new", "dxvk-3.1.2/x32/d3d11.dll": "new32"})
	te.gh.release = release("v3.1.2", asset("dxvk-3.1.2.zip", te.gh.payload), asset("dxvk-3.1.2-src.zip", nil))
	mods := writeTree(t, t.TempDir(), map[string]string{"dxvk-3.1.1.zip": "old archive", "other.txt": "x"})
	return te, &applist.App{Name: "dxvk", Repository: "doitsujin/dxvk", Type: "download", Folder: mods, InstalledVersion: "v3.1.1"}, mods
}

func TestDownloadKeepsOldArchivesAndAddsTheNewOne(t *testing.T) {
	te, app, mods := downloadCase(t)
	r := te.Update(app)
	if r.Status != Updated || r.Installed != "v3.1.2" || r.Note != "Downloaded dxvk-3.1.2.zip. Checksum verified" || app.InstalledVersion != "v3.1.2" {
		t.Fatalf("got %+v, version %q", r, app.InstalledVersion)
	}
	if readFile(t, filepath.Join(mods, "dxvk-3.1.1.zip")) != "old archive" {
		t.Error("the old archive should stay")
	}
	if _, err := os.Stat(filepath.Join(mods, "dxvk-3.1.2.zip")); err != nil {
		t.Error("the new archive wasn't saved in the folder")
	}
	if _, err := os.Stat(filepath.Join(mods, "dxvk-3.1.2")); err == nil {
		t.Error("nothing was extracted before, so nothing should be extracted now")
	}
	if r := te.Update(app); r.Status != UpToDate {
		t.Errorf("second run: %+v", r)
	}
}

func TestDownloadExtractsWhenTheOldOneWasExtracted(t *testing.T) {
	te, app, mods := downloadCase(t)
	writeTree(t, mods, map[string]string{"dxvk-3.1.1/x64/d3d11.dll": "old", "dxvk-3.1.1/my-notes.txt": "mine"})
	r := te.Update(app)
	if r.Status != Updated || !strings.Contains(r.Note, "extracted to dxvk-3.1.2") {
		t.Fatalf("got %+v", r)
	}
	if readFile(t, filepath.Join(mods, "dxvk-3.1.2", "x64", "d3d11.dll")) != "new" {
		t.Error("the new version wasn't extracted next to the archives")
	}
	if _, err := os.Stat(filepath.Join(mods, "dxvk-3.1.1")); !os.IsNotExist(err) {
		t.Error("the old extracted folder should be gone from the folder")
	}
	backup := filepath.Join(te.BackupRoot, "dxvk", "2026-10-14_12-00-00", "dxvk-3.1.1", "my-notes.txt")
	if readFile(t, backup) != "mine" {
		t.Error("the old folder, with your changes, should be in the backups")
	}
}

func TestDownloadExtractsAnArchiveWithoutAWrapperFolder(t *testing.T) {
	te, app, mods := downloadCase(t)
	te.gh.payload = zipOf(map[string]string{"OptiScaler.dll": "new", "OptiScaler.ini": "default"})
	te.gh.release = release("v0.9.5", asset("Optiscaler_0.9.5.zip", te.gh.payload))
	app.Name, app.Repository, app.InstalledVersion = "OptiScaler", "optiscaler/OptiScaler", "v0.9.4"
	writeTree(t, mods, map[string]string{"Optiscaler_0.9.4/OptiScaler.ini": "my settings"})
	if r := te.Update(app); r.Status != Updated {
		t.Fatalf("got %+v", r)
	}
	if readFile(t, filepath.Join(mods, "Optiscaler_0.9.5", "OptiScaler.dll")) != "new" {
		t.Error("an archive without one folder inside should be extracted into a folder named after it")
	}
}

func TestDownloadBrokenArchiveChangesNothing(t *testing.T) {
	te, app, mods := downloadCase(t)
	writeTree(t, mods, map[string]string{"dxvk-3.1.1/x64/d3d11.dll": "old"})
	te.gh.payload = []byte("not a zip")
	te.gh.release = release("v3.1.2", asset("dxvk-3.1.2.zip", te.gh.payload))
	if r := te.Update(app); r.Status != Failed {
		t.Fatalf("got %+v", r)
	}
	if readFile(t, filepath.Join(mods, "dxvk-3.1.1", "x64", "d3d11.dll")) != "old" || app.InstalledVersion != "v3.1.1" {
		t.Error("a failed update must leave the old version in place")
	}
	if _, err := os.Stat(filepath.Join(mods, "dxvk-3.1.2.zip")); err == nil {
		t.Error("a broken archive must not be saved")
	}
}

func TestDownloadUpToDateWrittenDifferently(t *testing.T) {
	te, app, _ := downloadCase(t)
	app.InstalledVersion = "3.1.2" // detected from a name like dxvk-3.1.2
	if r := te.Update(app); r.Status != UpToDate || len(te.gh.downloads) != 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestDownloadSkips(t *testing.T) {
	te, app, _ := downloadCase(t)
	app.Folder = filepath.Join(t.TempDir(), "missing")
	if r := te.Update(app); r.Status != Skipped || !strings.HasPrefix(r.Note, "Folder not found: ") {
		t.Errorf("got %+v", r)
	}
	te, app, _ = downloadCase(t)
	te.DryRun = true
	if r := te.Update(app); r.Status != UpdateAvailable || len(te.gh.downloads) != 0 {
		t.Errorf("dry run: %+v", r)
	}
}

func TestDetectVersionAndExtractedFolder(t *testing.T) {
	dir := writeTree(t, t.TempDir(), map[string]string{
		"Optiscaler_0.9.3.7z": "", "Optiscaler_0.9.4-final.20260718._MM.7z": "", "Optiscaler_0.9.10.txt": "",
		"dxvk-3.1.1/x64/d3d11.dll": "", "readme-1.0.zip": "",
	})
	if got := DetectVersion(dir, "optiscaler/OptiScaler"); got != "0.9.4" {
		t.Errorf("OptiScaler: got %q (a .txt isn't an archive)", got)
	}
	if got := DetectVersion(dir, "doitsujin/dxvk"); got != "3.1.1" {
		t.Errorf("dxvk: got %q", got)
	}
	// Alone, so a newer file can't hide it: the 7 of .7z isn't part of the version
	only := writeTree(t, t.TempDir(), map[string]string{"Optiscaler_0.9.4.7z": "", "tool-1.2.tar.gz": ""})
	if got := DetectVersion(only, "optiscaler/OptiScaler"); got != "0.9.4" {
		t.Errorf("Optiscaler_0.9.4.7z: got %q", got)
	}
	if got := DetectVersion(only, "o/tool"); got != "1.2" {
		t.Errorf("tool-1.2.tar.gz: got %q", got)
	}
	if got := DetectVersion(dir, "storytold/photocraft"); got != "" {
		t.Errorf("nothing there: got %q", got)
	}
	if got := ExtractedFolder(dir, "doitsujin/dxvk", "v3.1.1"); got != filepath.Join(dir, "dxvk-3.1.1") {
		t.Errorf("extracted folder: got %q", got)
	}
	if got := ExtractedFolder(dir, "doitsujin/dxvk", "v3.1.0"); got != "" {
		t.Errorf("another version: got %q", got)
	}
}

func TestPrepareWorksOutHowToTrackAnApp(t *testing.T) {
	downloads := t.TempDir()
	writeTree(t, downloads, map[string]string{"Mods/dxvk-3.1.0.tar.gz": "", "Mods/Old/dxvk-2.0/x64/a.dll": "", "Mods/dxvk-3.1.1/x64/a.dll": ""})
	tests := []struct {
		desc      string
		assets    []string
		installed string
		want      applist.App
		wantErr   string
	}{
		{"installed, with an installer", []string{"Tool-x64.msi", "Tool-win64.zip"}, "Tool 1.0", applist.App{Name: "Tool"}, ""},
		{"a release of archives", []string{"tool-1.1.zip"}, "", applist.App{Name: "tool", Type: "download", Folder: downloads}, ""},
		{"an installer that isn't installed, with a portable zip", []string{"Tool-x64.msi", "Tool-win64.zip"}, "", applist.App{}, "tool isn't installed"},
		{"only an installer, not installed", []string{"Tool-x64-setup.exe"}, "", applist.App{}, "tool isn't installed"},
		{"nothing usable", []string{"tool.dmg"}, "", applist.App{}, "no Windows installer"},
	}
	for _, tt := range tests {
		te := newEngine(t)
		var assets []github.Asset
		for _, a := range tt.assets {
			assets = append(assets, asset(a, nil))
		}
		te.gh.release = release("v1.1", assets...)
		if tt.installed != "" {
			te.sys.programs = []system.Program{program(tt.installed, "1.0", true, true)}
		}
		app := &applist.App{Repository: "o/tool"}
		err := te.Prepare(app, []string{downloads})
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: got %v", tt.desc, err)
			}
			continue
		}
		if err != nil || app.Name != tt.want.Name || app.Type != tt.want.Type || app.Folder != tt.want.Folder {
			t.Errorf("%s: got %+v, %v", tt.desc, app, err)
		}
	}

	// DXVK is found where you keep it, with the newest version you have
	te := newEngine(t)
	te.gh.release = release("v3.1.2", asset("dxvk-3.1.2.tar.gz", nil))
	app := &applist.App{Repository: "doitsujin/dxvk"}
	if err := te.Prepare(app, []string{downloads}); err != nil || app.Folder != filepath.Join(downloads, "Mods") || app.InstalledVersion != "3.1.1" {
		t.Errorf("dxvk: got %+v, %v", app, err)
	}
}

func TestInstalledFrom(t *testing.T) {
	programs := []system.Program{
		{Name: "GitHub Desktop"},
		{Name: "Notepad++ (64-bit x64)"},
		{Name: "Some Tool", Links: []string{"https://github.com/acme/widget/releases"}},
		{Name: "GridCraft"},
	}
	tests := map[string]string{
		"storytold/gridcraft":                 "GridCraft",              // by name
		"notepad-plus-plus/notepad-plus-plus": "Notepad++ (64-bit x64)", // first word starts the repository's name
		"acme/widget":                         "Some Tool",              // by its link
		"git-for-windows/git":                 "",                       // "git" isn't "GitHub Desktop"
		"o/note":                              "",                       // "notepad" doesn't start "note"
	}
	for repo, want := range tests {
		got := ""
		if p := installedFrom(programs, "", repo); p != nil {
			got = p.Name
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", repo, got, want)
		}
	}
}

func TestLocalRepo(t *testing.T) {
	te, dir, add := downloadsCase(t)
	te.sys.programs = append(te.sys.programs, system.Program{Name: "PhotoCraft 1.0", Links: []string{"https://github.com/storytold/photocraft"}})
	add("Mods/Optiscaler_0.9.4.7z", "https://github.com/optiscaler/OptiScaler/releases/tag/v0.9.4", 0)
	tests := map[string]string{
		"photocraft": "storytold/photocraft",  // an installed app that links to GitHub
		"optiscaler": "optiscaler/OptiScaler", // a download from GitHub
		"gridcraft":  "",                      // installed, but no GitHub link
		"dxvk":       "",                      // nothing on this PC
	}
	for name, want := range tests {
		if got, err := te.LocalRepo(name, []string{dir}); err != nil || got != want {
			t.Errorf("%s: got %q, %v", name, got, err)
		}
	}
}

func TestSearchReposRanksTheLikeliestFirst(t *testing.T) {
	te := newEngine(t)
	te.gh.repos = []github.Repo{
		{FullName: "NVIDIAGameWorks/dxvk-remix", Stars: 430},
		{FullName: "Sporif/dxvk-async", Stars: 829, Archived: true},
		{FullName: "ValveSoftware/dxvk", Stars: 488},
		{FullName: "doitsujin/dxvk", Stars: 18285},
		{FullName: "jp7677/dxvk-nvapi", Stars: 663},
	}
	repos, _ := te.SearchRepos("dxvk")
	var got []string
	for _, r := range repos {
		got = append(got, r.FullName)
	}
	want := "doitsujin/dxvk ValveSoftware/dxvk jp7677/dxvk-nvapi NVIDIAGameWorks/dxvk-remix Sporif/dxvk-async"
	if strings.Join(got, " ") != want {
		t.Errorf("got  %v\nwant %s", got, want)
	}
}

func TestSearchWords(t *testing.T) {
	tests := map[string]string{"notepad++": "notepad plus plus", "dxvk": "dxvk", "  Photo-Craft ": "Photo Craft", "c#": "c"}
	for in, want := range tests {
		if got := searchWords(in); got != want {
			t.Errorf("searchWords(%q) = %q, want %q", in, got, want)
		}
	}
	if !sameName("notepad++", "notepad-plus-plus") || !sameName("OptiScaler", "optiscaler") || sameName("notepad", "notepad-plus-plus") {
		t.Error("sameName")
	}
}
