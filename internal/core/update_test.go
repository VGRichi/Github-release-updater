package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/system"
)

// An installer app at 1.0.0, installed for the current user; the release has a verified x64 setup.
func installerCase(t *testing.T) (*testEngine, *applist.App) {
	te := newEngine(t)
	te.sys.programs = []system.Program{program("PhotoCraft", "1.0.0", true, false)}
	te.gh.release = release("v1.1.0", asset("PhotoCraft-1.1.0-x64-setup.exe", te.gh.payload), asset("PhotoCraft-1.1.0-linux.AppImage", nil))
	return te, &applist.App{Name: "PhotoCraft", Repository: "storytold/photocraft"}
}

func TestUpdateInstallsAVerifiedUpdate(t *testing.T) {
	te, app := installerCase(t)
	r := te.Update(app)
	want := Result{Name: "PhotoCraft", Repository: "storytold/photocraft", Installed: "1.0.0", Latest: "v1.1.0", Status: Updated, Note: "Checksum verified"}
	if r != want {
		t.Fatalf("got %+v", r)
	}
	if len(te.sys.runs) != 1 || filepath.Base(te.sys.runs[0].file) != "PhotoCraft-1.1.0-x64-setup.exe" {
		t.Fatalf("runs: %v", te.sys.runs)
	}
	if !app.HadDigest {
		t.Error("HadDigest should be learned")
	}
	if entries, _ := os.ReadDir(te.WorkRoot); len(entries) != 0 {
		t.Error("the work folder was left behind")
	}
}

func TestUpdateUpToDate(t *testing.T) {
	te, app := installerCase(t)
	te.sys.programs[0].Version = "1.1.0.0" // written differently, still the same version
	if r := te.Update(app); r.Status != UpToDate || len(te.gh.downloads) != 0 {
		t.Fatalf("got %+v, downloads %v", r, te.gh.downloads)
	}
}

func TestUpdateNotInstalled(t *testing.T) {
	te, app := installerCase(t)
	te.sys.programs = nil
	if r := te.Update(app); r.Status != NotInstalled || len(te.gh.latest) != 0 {
		t.Fatalf("got %+v; GitHub shouldn't be asked", r)
	}
}

func TestUpdateFindsTheAppByNameWithBrackets(t *testing.T) {
	te, app := installerCase(t)
	te.sys.programs = []system.Program{program("Tool [Beta] 1.0.0", "1.0.0", true, false)}
	app.Name = "Tool [Beta]"
	if r := te.Update(app); r.Status != Updated {
		t.Fatalf("got %+v", r)
	}
}

func TestUpdateAllUsersAppNeedsAdmin(t *testing.T) {
	te, app := installerCase(t)
	te.sys.programs[0].PerUser = false
	if r := te.Update(app); r.Status != NeedsAdmin || len(te.gh.downloads) != 0 {
		t.Fatalf("got %+v", r)
	}
	te.sys.admin = true
	if r := te.Update(app); r.Status != Updated {
		t.Fatalf("as admin: got %+v", r)
	}
}

func TestUpdatePicksTheMsiForAnMsiInstall(t *testing.T) {
	te, app := installerCase(t)
	te.sys.programs[0].MSI = true
	te.gh.release.Assets = append(te.gh.release.Assets, asset("PhotoCraft-1.1.0-x64.msi", te.gh.payload))
	te.Update(app)
	if len(te.sys.runs) != 1 || te.sys.runs[0].file != "msiexec.exe" || !strings.HasPrefix(te.sys.runs[0].args, `/i "`) ||
		!strings.HasSuffix(te.sys.runs[0].args, `PhotoCraft-1.1.0-x64.msi" /qn /norestart`) {
		t.Fatalf("runs: %v", te.sys.runs)
	}
}

func TestUpdateWithoutChecksum(t *testing.T) {
	tests := []struct {
		answer  int
		status  Status
		note    string
		trusted bool
	}{
		{0, Updated, "Unverified", false},                   // Yes
		{1, Updated, "Unverified", true},                    // Always: trusted from now on
		{2, Skipped, "Unverified download declined", false}, // No
	}
	for _, tt := range tests {
		te, app := installerCase(t)
		te.gh.release.Assets[0].Digest = ""
		te.ask.answers = []int{tt.answer}
		r := te.Update(app)
		if r.Status != tt.status || r.Note != tt.note || app.AllowUnverified != tt.trusted || len(te.ask.questions) != 1 {
			t.Errorf("answer %d: got %+v, trusted %v, questions %v", tt.answer, r, app.AllowUnverified, te.ask.questions)
		}
	}
}

func TestUpdateTrustedAppWithoutChecksumDoesNotAsk(t *testing.T) {
	te, app := installerCase(t)
	te.gh.release.Assets[0].Digest = ""
	app.AllowUnverified = true
	if r := te.Update(app); r.Status != Updated || r.Note != "Unverified" {
		t.Fatalf("got %+v", r)
	}
}

func TestUpdateAlwaysAsksWhenEarlierReleasesHadAChecksum(t *testing.T) {
	te, app := installerCase(t)
	te.gh.release.Assets[0].Digest = ""
	app.HadDigest, app.AllowUnverified = true, true // trusting never skips this
	te.ask.answers = []int{1}                       // No
	r := te.Update(app)
	if r.Status != Skipped || len(te.warnings) != 1 || !strings.Contains(te.warnings[0], "unusual") {
		t.Fatalf("got %+v, warnings %v", r, te.warnings)
	}
	if verificationNote(app, &te.gh.release.Assets[0]) != "No checksum (unusual: earlier releases had one)" {
		t.Error("wrong note")
	}
}

func TestUpdateLearnsWhoSignedTheApp(t *testing.T) {
	te, app := installerCase(t)
	te.sys.signer = "Storytold Ltd"
	r := te.Update(app)
	if r.Status != Updated || r.Note != "Checksum verified, signed by Storytold Ltd" || app.Publisher != "Storytold Ltd" {
		t.Fatalf("got %+v, publisher %q", r, app.Publisher)
	}
}

func TestUpdateAsksWhenTheSignerChanges(t *testing.T) {
	tests := []struct {
		signer, warning string
		answer          int
		status          Status
		publisher       string
	}{
		{"Someone Else", "is signed by Someone Else", 1, Skipped, "Storytold Ltd"},   // No: nothing installed
		{"", "isn't signed", 1, Skipped, "Storytold Ltd"},                            // no longer signed
		{"Storytold Inc", "is signed by Storytold Inc", 0, Updated, "Storytold Inc"}, // Yes: the new signer is remembered
	}
	for _, tt := range tests {
		te, app := installerCase(t)
		app.Publisher, app.AllowUnverified = "Storytold Ltd", true // trusting never skips this
		te.sys.signer = tt.signer
		te.ask.answers = []int{tt.answer}
		r := te.Update(app)
		if r.Status != tt.status || app.Publisher != tt.publisher || len(te.warnings) == 0 || !strings.Contains(te.warnings[0], tt.warning) {
			t.Errorf("signer %q: got %+v, publisher %q, warnings %v", tt.signer, r, app.Publisher, te.warnings)
		}
		if tt.status == Skipped && len(te.sys.runs) != 0 {
			t.Errorf("signer %q: the installer ran", tt.signer)
		}
	}
}

func TestUpdateBrokenSignatureInstallsNothing(t *testing.T) {
	te, app := installerCase(t)
	te.sys.signerErr = errors.New("its digital signature isn't valid")
	r := te.Update(app)
	if r.Status != Failed || !strings.Contains(r.Note, "signature isn't valid") || len(te.sys.runs) != 0 {
		t.Fatalf("got %+v, runs %v", r, te.sys.runs)
	}
}

func TestUpdateNeverSignedAppIsUnaffected(t *testing.T) {
	te, app := installerCase(t)
	if r := te.Update(app); r.Status != Updated || r.Note != "Checksum verified" || app.Publisher != "" {
		t.Fatalf("got %+v", r)
	}
}

func TestUpdateChecksumMismatchInstallsNothing(t *testing.T) {
	te, app := installerCase(t)
	te.gh.payload = []byte("tampered")
	r := te.Update(app)
	if r.Status != Failed || !strings.HasPrefix(r.Note, "Checksum mismatch") || len(te.sys.runs) != 0 {
		t.Fatalf("got %+v, runs %v", r, te.sys.runs)
	}
}

func TestUpdateInstallerExitCodes(t *testing.T) {
	tests := []struct {
		code   int
		status Status
		note   string
	}{
		{3010, Updated, "Checksum verified. Restart needed to finish"},
		{1641, Updated, "Checksum verified. Restart needed to finish"},
		{1603, Failed, "Installer exit code 1603"},
	}
	for _, tt := range tests {
		te, app := installerCase(t)
		te.sys.exitCode = tt.code
		if r := te.Update(app); r.Status != tt.status || r.Note != tt.note {
			t.Errorf("exit %d: got %+v", tt.code, r)
		}
	}
}

func TestUpdateDryRunDownloadsNothing(t *testing.T) {
	te, app := installerCase(t)
	te.DryRun = true
	te.sys.programs[0].PerUser = false // a dry run needs no admin rights
	r := te.Update(app)
	if r.Status != UpdateAvailable || r.Note != "Checksum available" || len(te.gh.downloads) != 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestUpdateGitHubErrorFails(t *testing.T) {
	te, app := installerCase(t)
	te.gh.err = errors.New("GitHub replied 500")
	r := te.Update(app)
	if r.Status != Failed || r.Note != "GitHub replied 500." || len(te.warnings) != 1 {
		t.Fatalf("got %+v, warnings %v", r, te.warnings)
	}
}

func TestUpdateSkips(t *testing.T) {
	te, app := installerCase(t)
	te.sys.programs[0].Version = "nightly"
	if r := te.Update(app); r.Status != Skipped || r.Note != "Can't compare versions" {
		t.Errorf("got %+v", r)
	}
	te, app = installerCase(t)
	te.gh.release.Assets = te.gh.release.Assets[1:]
	if r := te.Update(app); r.Status != Skipped || r.Note != "No matching Windows installer in this release" {
		t.Errorf("got %+v", r)
	}
	te, app = installerCase(t)
	te.gh.release = nil
	if r := te.Update(app); r.Status != Skipped || r.Note != "No releases found" {
		t.Errorf("got %+v", r)
	}
}

func TestUpdateFollowsPrereleases(t *testing.T) {
	te, app := installerCase(t)
	app.Prerelease = true
	te.Update(app)
	if len(te.gh.latest) != 1 || !te.gh.latest[0] {
		t.Fatalf("Latest calls: %v", te.gh.latest)
	}
}

func TestSilentArgs(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]string{"inno.exe": "MZ...Inno Setup...", "nsis.exe": "MZ...Nullsoft Install System...", "other.exe": "MZ..."}
	want := map[string]string{"inno.exe": "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART", "nsis.exe": "/S", "other.exe": ""}
	for name, content := range tests {
		path := filepath.Join(dir, name)
		os.WriteFile(path, []byte(content), 0o644)
		if got := silentArgs(path); got != want[name] {
			t.Errorf("%s: got %q", name, got)
		}
	}
}

func TestUpdateUsesSilentArgsOrWarns(t *testing.T) {
	te, app := installerCase(t)
	app.SilentArgs = "--quiet"
	te.Update(app)
	if te.sys.runs[0].args != "--quiet" {
		t.Errorf("args: %q", te.sys.runs[0].args)
	}
	te, app = installerCase(t)
	te.Update(app) // "installer bytes" is no known installer type
	if te.sys.runs[0].args != "" || len(te.warnings) != 1 || !strings.Contains(te.warnings[0], "opening it normally") {
		t.Errorf("args %q, warnings %v", te.sys.runs[0].args, te.warnings)
	}
}

// An archive tool (DXVK) whose game folder has an old d3d11.dll and a settings file.
func archiveCase(t *testing.T) (*testEngine, *applist.App, string) {
	te := newEngine(t)
	te.gh.payload = zipOf(map[string]string{
		"dxvk-2.5/x64/d3d11.dll": "new d3d11 x64", "dxvk-2.5/x64/dxgi.dll": "new dxgi x64", "dxvk-2.5/x64/dxvk.conf": "default",
		"dxvk-2.5/x32/d3d11.dll": "new d3d11 x32",
	})
	te.gh.release = release("v2.5", asset("dxvk-2.5.zip", te.gh.payload), asset("dxvk-2.5-src.zip", nil))
	game := writeTree(t, t.TempDir(), map[string]string{"d3d11.dll": "old d3d11", "dxvk.conf": "my settings"})
	app := &applist.App{Name: "DXVK (Test)", Repository: "doitsujin/dxvk", Type: "archive", Destination: applist.Strings{game}, Subfolder: "x64", Keep: applist.Strings{"*.conf"}}
	return te, app, game
}

func TestArchiveUpdate(t *testing.T) {
	te, app, game := archiveCase(t)
	r := te.Update(app)
	if r.Status != Updated || r.Installed != "v2.5" || r.Note != "Checksum verified" || app.InstalledVersion != "v2.5" {
		t.Fatalf("got %+v, version %q", r, app.InstalledVersion)
	}
	if readFile(t, filepath.Join(game, "d3d11.dll")) != "new d3d11 x64" || readFile(t, filepath.Join(game, "dxgi.dll")) != "new dxgi x64" {
		t.Error("files weren't updated from the x64 folder")
	}
	if readFile(t, filepath.Join(game, "dxvk.conf")) != "my settings" {
		t.Error("the kept settings file was overwritten")
	}
	backup := filepath.Join(te.BackupRoot, "DXVK (Test)", "2026-10-14_12-00-00", "1", "d3d11.dll")
	if readFile(t, backup) != "old d3d11" {
		t.Error("the replaced file wasn't backed up")
	}
	if r := te.Update(app); r.Status != UpToDate {
		t.Errorf("second run: %+v", r)
	}
}

func TestArchiveUpdateEveryDestination(t *testing.T) {
	te, app, _ := archiveCase(t)
	second := writeTree(t, t.TempDir(), map[string]string{"d3d11.dll": "old other"})
	app.Destination = append(app.Destination, second)
	te.Update(app)
	if readFile(t, filepath.Join(second, "d3d11.dll")) != "new d3d11 x64" {
		t.Error("the second folder wasn't updated")
	}
	if readFile(t, filepath.Join(te.BackupRoot, "DXVK (Test)", "2026-10-14_12-00-00", "2", "d3d11.dll")) != "old other" {
		t.Error("the second folder has no backup")
	}
}

func TestArchiveUpdateSkips(t *testing.T) {
	te, app, _ := archiveCase(t)
	app.Destination = applist.Strings{filepath.Join(t.TempDir(), "missing")}
	if r := te.Update(app); r.Status != Skipped || !strings.HasPrefix(r.Note, "Folder not found: ") {
		t.Errorf("got %+v", r)
	}
	app.Destination = nil
	if r := te.Update(app); r.Status != Skipped || r.Note != "No destination folder set" {
		t.Errorf("got %+v", r)
	}
}

func TestArchiveUpdateIsAllOrNothing(t *testing.T) {
	te, app, game := archiveCase(t)
	// The second folder can't take dxgi.dll (a folder is in the way, like a file locked by a running game)
	second := writeTree(t, t.TempDir(), map[string]string{"d3d11.dll": "old other"})
	os.MkdirAll(filepath.Join(second, "dxgi.dll", "blocker"), 0o755)
	app.Destination = append(app.Destination, second)
	r := te.Update(app)
	if r.Status != Failed || !strings.Contains(r.Note, "Files already replaced were put back") {
		t.Fatalf("got %+v", r)
	}
	if readFile(t, filepath.Join(game, "d3d11.dll")) != "old d3d11" || readFile(t, filepath.Join(second, "d3d11.dll")) != "old other" {
		t.Error("replaced files weren't put back")
	}
	if _, err := os.Stat(filepath.Join(game, "dxgi.dll")); !os.IsNotExist(err) {
		t.Error("an added file wasn't removed")
	}
	if app.InstalledVersion != "" {
		t.Error("the version must not change when the update failed")
	}
}

func TestArchiveUpdateKeepsThreeBackups(t *testing.T) {
	te, app, _ := archiveCase(t)
	appBackups := filepath.Join(te.BackupRoot, "DXVK (Test)")
	for _, old := range []string{"2026-01-01_00-00-00", "2026-02-01_00-00-00", "2026-03-01_00-00-00"} {
		os.MkdirAll(filepath.Join(appBackups, old), 0o755)
	}
	te.Update(app)
	entries, _ := os.ReadDir(appBackups)
	if len(entries) != 3 || entries[0].Name() != "2026-02-01_00-00-00" {
		t.Errorf("backups: %v", entries)
	}
}

func TestCleanWorkRoot(t *testing.T) {
	te := newEngine(t)
	for _, name := range []string{"reforge-123", "reforge-456", "other"} {
		os.MkdirAll(filepath.Join(te.WorkRoot, name), 0o755)
	}
	old := te.now().Add(-48 * time.Hour)
	os.Chtimes(filepath.Join(te.WorkRoot, "reforge-123"), old, old)
	os.Chtimes(filepath.Join(te.WorkRoot, "other"), old, old)
	te.CleanWorkRoot()
	entries, _ := os.ReadDir(te.WorkRoot)
	if len(entries) != 2 || entries[0].Name() != "other" || entries[1].Name() != "reforge-456" {
		t.Errorf("left: %v", entries)
	}
}

func TestCheckAsksOnceForEachRepositoryAndUpdateUsesIt(t *testing.T) {
	te := newEngine(t)
	te.gh.release = release("v1.1.0")
	te.sys.programs = []system.Program{program("PhotoCraft", "1.1.0", true, true)}
	folder := t.TempDir()
	apps := []*applist.App{
		{Name: "PhotoCraft", Repository: "storytold/photocraft"},
		{Name: "Missing", Repository: "o/missing"}, // not installed: not asked about
		{Name: "Tool", Repository: "o/tool", Type: "download", Folder: folder, InstalledVersion: "v1.1.0"},
		{Name: "Tool too", Repository: "O/Tool", Type: "download", Folder: folder, InstalledVersion: "v1.1.0"},
	}
	te.Check(apps)
	if len(te.gh.latest) != 2 {
		t.Fatalf("%d checks; want one each for photocraft and tool", len(te.gh.latest))
	}
	for _, app := range apps {
		te.Update(app)
	}
	if len(te.gh.latest) != 2 {
		t.Errorf("%d checks; Update should use Check's answers", len(te.gh.latest))
	}
}

func TestToolUpToDateWrittenDifferently(t *testing.T) {
	te := newEngine(t)
	te.gh.release = release("v2.0")
	app := &applist.App{Name: "Tool", Repository: "o/tool", Type: "archive", Destination: applist.Strings{t.TempDir()}, InstalledVersion: "2.0.0"}
	if r := te.Update(app); r.Status != UpToDate {
		t.Errorf("got %s: %s", r.Status, r.Note)
	}
}
