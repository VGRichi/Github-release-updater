package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/system"
)

// A first install: an unrelated app before, PhotoCraft as well after the installer runs.
func installCase(t *testing.T) (*testEngine, *applist.App) {
	te := newEngine(t)
	other := program("Other App", "2.0", true, false)
	te.sys.programs = []system.Program{other}
	te.sys.after = []system.Program{other, program("PhotoCraft 1.1.0 (x64)", "1.1.0", true, true)}
	te.gh.release = release("v1.1.0", asset("PhotoCraft-1.1.0-x64-setup.exe", te.gh.payload), asset("PhotoCraft-1.1.0-x64.msi", te.gh.payload))
	return te, &applist.App{Repository: "storytold/photocraft"}
}

func TestInstallAsksThenInstallsTheMsiAndLearnsTheName(t *testing.T) {
	te, app := installCase(t)
	te.ask.answers = []int{0}
	r, track := te.Install(app, false)
	want := Result{Name: "PhotoCraft (x64)", Repository: "storytold/photocraft", Installed: "1.1.0", Latest: "v1.1.0", Status: Installed, Note: "Checksum verified"}
	if r != want || !track {
		t.Fatalf("got %+v, track %v", r, track)
	}
	if app.Name != "PhotoCraft (x64)" || !app.HadDigest {
		t.Errorf("app: %+v", app)
	}
	if len(te.ask.questions) != 1 || !strings.Contains(te.ask.questions[0], "PhotoCraft-1.1.0-x64.msi from storytold/photocraft") {
		t.Errorf("questions: %v", te.ask.questions)
	}
	if len(te.sys.runs) != 1 || te.sys.runs[0].file != "msiexec.exe" {
		t.Errorf("runs: %v", te.sys.runs)
	}
}

func TestInstallKeepsAGivenName(t *testing.T) {
	te, app := installCase(t)
	app.Name = "PhotoCraft"
	if r, track := te.Install(app, true); r.Name != "PhotoCraft" || app.Name != "PhotoCraft" || !track {
		t.Fatalf("got %+v", r)
	}
}

func TestInstallNoInstallsNothing(t *testing.T) {
	te, app := installCase(t)
	te.ask.answers = []int{1}
	r, track := te.Install(app, false)
	if r.Status != Skipped || r.Note != "Not installed" || track || len(te.gh.downloads) != 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestInstallForceDoesNotAsk(t *testing.T) {
	te, app := installCase(t)
	if r, _ := te.Install(app, true); r.Status != Installed || len(te.ask.questions) != 0 {
		t.Fatalf("got %+v, questions %v", r, te.ask.questions)
	}
}

func TestInstallWithoutChecksumAsksTheChecksumQuestionEvenWithForce(t *testing.T) {
	te, app := installCase(t)
	te.gh.release = release("v1.1.0", asset("PhotoCraft-1.1.0-x64.msi", nil))
	te.ask.answers = []int{2} // No
	if r, _ := te.Install(app, true); r.Status != Skipped || len(te.sys.runs) != 0 {
		t.Fatalf("got %+v", r)
	}
	if len(te.ask.questions) != 1 || !strings.Contains(te.ask.questions[0], "no checksum") {
		t.Fatalf("questions: %v", te.ask.questions)
	}

	te, app = installCase(t)
	te.gh.release = release("v1.1.0", asset("PhotoCraft-1.1.0-x64.msi", nil))
	te.ask.answers = []int{1} // Always
	if r, _ := te.Install(app, false); r.Note != "Unverified" || !app.AllowUnverified {
		t.Fatalf("got %+v, trusted %v", r, app.AllowUnverified)
	}
}

func TestInstallTrustedAppGetsTheNormalQuestion(t *testing.T) {
	te, app := installCase(t)
	te.gh.release = release("v1.1.0", asset("PhotoCraft-1.1.0-x64.msi", nil))
	app.AllowUnverified = true
	te.ask.answers = []int{0}
	te.Install(app, false)
	if len(te.ask.questions) != 1 || !strings.HasPrefix(te.ask.questions[0], "Install ") {
		t.Fatalf("questions: %v", te.ask.questions)
	}
}

func TestInstallDryRun(t *testing.T) {
	te, app := installCase(t)
	te.DryRun = true
	r, track := te.Install(app, false)
	if r.Status != Available || r.Note != "Checksum available" || track || len(te.gh.downloads) != 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestInstallFailedSuggestsAdmin(t *testing.T) {
	te, app := installCase(t)
	te.sys.exitCode = 1603
	r, track := te.Install(app, true)
	if r.Status != Failed || r.Note != "Installer exit code 1603. It may need to run as administrator" || track {
		t.Fatalf("got %+v", r)
	}
}

func TestInstallNothingNewAppearsIsNotTracked(t *testing.T) {
	te, app := installCase(t)
	te.sys.after = te.sys.programs // like reinstalling an app that's already there
	r, track := te.Install(app, true)
	if r.Status != Installed || r.Note != "Checksum verified. Not tracked" || track || r.Name != "photocraft" {
		t.Fatalf("got %+v, track %v", r, track)
	}
	if len(te.warnings) != 1 || !strings.Contains(te.warnings[0], "reforge add storytold/photocraft --name") {
		t.Errorf("warnings: %v", te.warnings)
	}
}

func TestInstallPrefersTheAppNamedLikeTheRepository(t *testing.T) {
	te, app := installCase(t)
	te.sys.after = append(te.sys.programs, program("Some Runtime", "9.0", false, true), program("PhotoCraft", "1.1.0", true, true))
	if r, _ := te.Install(app, true); r.Name != "PhotoCraft" {
		t.Fatalf("got %+v", r)
	}
}

func TestInstallNoticesAnUpdatedApp(t *testing.T) {
	te, app := installCase(t)
	te.sys.programs = []system.Program{program("PhotoCraft", "1.0.0", true, false)}
	te.sys.after = []system.Program{program("PhotoCraft", "1.1.0", true, false)}
	if r, _ := te.Install(app, true); r.Name != "PhotoCraft" || r.Installed != "1.1.0" {
		t.Fatalf("got %+v", r)
	}
}

func TestInstallWithoutAnInstaller(t *testing.T) {
	te, app := installCase(t)
	te.gh.release = release("v3.1", asset("dxvk-3.1.tar.gz", nil))
	r, _ := te.Install(app, true)
	if r.Status != Skipped || !strings.Contains(r.Note, "reforge add --dest") {
		t.Fatalf("got %+v", r)
	}
}

func TestBaseNameAndFindProgram(t *testing.T) {
	names := map[string]string{
		"PhotoCraft 1.2.3 (x64)": "PhotoCraft (x64)",
		"7-Zip 23.01 (x64)":      "7-Zip (x64)",
		"Python 3.12.1 (64-bit)": "Python (64-bit)",
		"App v2.0-beta1":         "App",
		"Notepad++ (64-bit x64)": "Notepad++ (64-bit x64)",
		"BCUninstaller 6.3.0.0":  "BCUninstaller",
		"PhotoCraft":             "PhotoCraft",
	}
	for in, want := range names {
		if got := BaseName(in); got != want {
			t.Errorf("BaseName(%q) = %q, want %q", in, got, want)
		}
	}
	programs := []system.Program{{Name: "PhotoCraft 1.3.0 (x64)"}, {Name: "Notepad++ (64-bit x64)"}}
	if p := FindProgram(programs, "notepad++"); p == nil || p.Name != "Notepad++ (64-bit x64)" {
		t.Error("a partial name should match")
	}
	if p := FindProgram(programs, "PhotoCraft 1.2.0 (x64)"); p == nil || p.Name != "PhotoCraft 1.3.0 (x64)" {
		t.Error("a name with an old version should match by its base name")
	}
	if FindProgram(programs, "Other") != nil {
		t.Error("no match expected")
	}
}

func notTracked(string) bool { return false }

func findCase(t *testing.T) *testEngine {
	te := newEngine(t)
	te.sys.programs = []system.Program{
		{Name: "PhotoCraft", Version: "1.0.0", Links: []string{"https://github.com/storytold/photocraft"}},
		{Name: "PdfCraft 2.0", Version: "2.0", Links: []string{"https://github.com/storytold/pdfcraft.git"}},
		{Name: "HelpOnly", Version: "3.0", Links: []string{"https://example.com", "https://github.com/someone/helponly/issues"}},
		{Name: "PhotoCraft Helper", Version: "1.0.0", Links: []string{"https://github.com/storytold/photocraft"}},
		{Name: "Sponsored", Links: []string{"https://github.com/sponsors/someone"}},
		{Name: "Elsewhere", Links: []string{"https://example.com"}},
	}
	return te
}

func TestFindInstalledApps(t *testing.T) {
	te := findCase(t)
	found, err := te.Find(nil, notTracked)
	if err != nil {
		t.Fatal(err)
	}
	want := []Candidate{
		{"PhotoCraft", "storytold/photocraft", "1.0.0", "Installer"},
		{"PdfCraft", "storytold/pdfcraft", "2.0", "Installer"},
		{"HelpOnly", "someone/helponly", "3.0", "Installer"},
	}
	if len(found) != len(want) {
		t.Fatalf("got %v", found)
	}
	for i := range want {
		if found[i] != want[i] {
			t.Errorf("got %+v, want %+v", found[i], want[i])
		}
	}
}

func TestFindLeavesOutTrackedApps(t *testing.T) {
	te := findCase(t)
	found, _ := te.Find(nil, func(repo string) bool { return strings.EqualFold(repo, "storytold/PhotoCraft") })
	for _, c := range found {
		if c.Repository == "storytold/photocraft" {
			t.Fatal("a tracked app was suggested")
		}
	}
}

func TestFindLeavesOutWingetApps(t *testing.T) {
	te := newEngine(t)
	te.sys.programs = []system.Program{
		{Name: "PowerShell 7-x64", Version: "7.6.6.0", Links: []string{"https://github.com/PowerShell/PowerShell"}},
		{Name: "PhotoCraft", Version: "1.0.0", Links: []string{"https://github.com/storytold/photocraft"}},
	}
	// winget's "PowerShell" is PowerShell 7-x64 (same version); its "Photo" isn't PhotoCraft (other version)
	te.sys.winget = []system.Package{{Name: "PowerShell", Version: "7.6.6.0"}, {Name: "Photo", Version: "9.9"}}
	found, _ := te.Find(nil, notTracked)
	if len(found) != 1 || found[0].Name != "PhotoCraft" {
		t.Fatalf("got %v", found)
	}
	if te.sys.wingetCalls != 1 {
		t.Errorf("winget asked %d times", te.sys.wingetCalls)
	}
}

func TestFindDoesNotAskWingetWithNothingToCheck(t *testing.T) {
	te := newEngine(t)
	te.sys.programs = []system.Program{{Name: "Elsewhere", Links: []string{"https://example.com"}}}
	te.Find(nil, notTracked)
	if te.sys.wingetCalls != 0 {
		t.Error("winget is slow; don't ask it for nothing")
	}
}

// downloadsCase has GridCraft and Notepad++ installed (no GitHub links) and a Downloads folder.
func downloadsCase(t *testing.T) (*testEngine, string, func(rel, referrer string, age time.Duration)) {
	te := newEngine(t)
	te.sys.programs = []system.Program{
		{Name: "GridCraft", Version: "0.5.0", Links: []string{"https://getartcraft.com"}},
		{Name: "Notepad++ (64-bit x64)", Version: "8.9.8.1"},
	}
	te.sys.origins = map[string]string{}
	dir := t.TempDir()
	add := func(rel, referrer string, age time.Duration) {
		path := writeTree(t, dir, map[string]string{rel: "bytes"})
		path = filepath.Join(path, filepath.FromSlash(rel))
		when := time.Now().Add(-age)
		os.Chtimes(path, when, when)
		te.sys.origins[path] = "[ZoneTransfer]\r\nZoneId=3\r\nReferrerUrl=" + referrer +
			"\r\nHostUrl=https://release-assets.githubusercontent.com/github-production-release-asset/1/2?sig=x\r\n"
	}
	return te, dir, add
}

func TestFindDownloadedArchiveTool(t *testing.T) {
	te, dir, add := downloadsCase(t)
	add("Optiscaler_0.9.3.7z", "https://github.com/optiscaler/OptiScaler/releases/tag/v0.9.3", 30*24*time.Hour)
	add("Mods/Optiscaler_0.9.4.7z", "https://github.com/optiscaler/OptiScaler/releases/tag/v0.9.4", 0)
	found, _ := te.Find([]string{dir}, notTracked)
	if len(found) != 1 || found[0] != (Candidate{"OptiScaler", "optiscaler/OptiScaler", "v0.9.4", "Archive"}) {
		t.Fatalf("got %v", found)
	}
}

func TestFindDownloadedInstallerOfAnInstalledApp(t *testing.T) {
	te, dir, add := downloadsCase(t)
	add("GridCraft_0.5.0_x64-setup.exe", "https://github.com/storytold/gridcraft/releases/tag/v0.5.0", 0)
	add("npp.8.9.8.1.Installer.x64.exe", "https://github.com/notepad-plus-plus/notepad-plus-plus/releases/tag/v8.9.8.1", time.Hour)
	found, _ := te.Find([]string{dir}, notTracked)
	if len(found) != 2 || found[0] != (Candidate{"GridCraft", "storytold/gridcraft", "0.5.0", "Installer"}) ||
		found[1].Name != "Notepad++ (64-bit x64)" {
		t.Fatalf("got %v", found)
	}
}

func TestFindIgnoresDownloads(t *testing.T) {
	tests := map[string]struct{ rel, referrer string }{
		"its app isn't installed":   {"PhotoCraft-1.0.0-setup.exe", "https://github.com/storytold/photocraft/releases/tag/v1.0.0"},
		"another installed version": {"GridCraft_0.4.0_x64-setup.exe", "https://github.com/storytold/gridcraft/releases/tag/v0.4.0"},
		"another site":              {"tool.zip", "https://example.com/tool.zip"},
		"source code":               {"repo-1.0.zip", "https://github.com/owner/repo/archive/refs/tags/v1.0.zip"},
		"not an app file":           {"notes.txt", "https://github.com/owner/repo/releases/tag/v1.0"},
		"too deep":                  {"a/b/c/tool.zip", "https://github.com/owner/repo/releases/tag/v1.0"},
	}
	for desc, d := range tests {
		te, dir, add := downloadsCase(t)
		add(d.rel, d.referrer, 0)
		if found, _ := te.Find([]string{dir}, notTracked); len(found) != 0 {
			t.Errorf("%s: got %v", desc, found)
		}
	}
}

func TestFindDirectDownloadLinkAndTrackedRepos(t *testing.T) {
	te, dir, add := downloadsCase(t)
	add("dxvk-2.5.tar.gz", "https://github.com/doitsujin/dxvk/releases/download/v2.5/dxvk-2.5.tar.gz", 0)
	add("Optiscaler_0.9.4.7z", "https://github.com/optiscaler/OptiScaler/releases/tag/v0.9.4", 0)
	found, _ := te.Find([]string{dir}, func(repo string) bool { return repo == "optiscaler/OptiScaler" })
	if len(found) != 1 || found[0].Repository != "doitsujin/dxvk" || found[0].Version != "v2.5" {
		t.Fatalf("got %v", found)
	}
}

func TestFindWarnsAboutAMissingFolder(t *testing.T) {
	te := newEngine(t)
	te.Find([]string{filepath.Join(t.TempDir(), "missing")}, notTracked)
	if len(te.warnings) != 1 || !strings.HasPrefix(te.warnings[0], "Folder not found") {
		t.Errorf("warnings: %v", te.warnings)
	}
}
