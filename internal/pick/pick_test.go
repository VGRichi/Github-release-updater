package pick

import (
	"testing"

	"github.com/VGRichi/Reforge/internal/github"
)

func assets(names ...string) []github.Asset {
	list := make([]github.Asset, len(names))
	for i, n := range names {
		list[i] = github.Asset{Name: n}
	}
	return list
}

func name(a *github.Asset) string {
	if a == nil {
		return "<nothing>"
	}
	return a.Name
}

func TestInstallerOnX64(t *testing.T) {
	tests := []struct {
		desc    string
		files   []string
		pattern string
		msi     bool
		want    string
	}{
		{"the x64 .msi when an .msi is installed", []string{"App-1.1-arm64.msi", "App-1.1-x64.msi", "App-1.1-x64-setup.exe"}, "", true, "App-1.1-x64.msi"},
		{"the x64 .exe when an .exe is installed", []string{"App-1.1-arm64.msi", "App-1.1-x64.msi", "App-1.1-x64-setup.exe"}, "", false, "App-1.1-x64-setup.exe"},
		{"x86_64 is x64", []string{"App_x86_64.exe"}, "", false, "App_x86_64.exe"},
		{"no architecture in the name", []string{"App-Setup.exe"}, "", false, "App-Setup.exe"},
		{"never another architecture", []string{"App-arm64.exe", "App-x86.exe", "App-ia32.msi", "App-32bit.exe"}, "", false, "<nothing>"},
		{"no portable builds, other platforms or non-installers", []string{"App-portable-x64.exe", "App-x64.dmg", "App-linux-x64.exe", "App-x64.zip"}, "", false, "<nothing>"},
		{"the pattern instead", []string{"App-x64.msi", "App-arm64.msi"}, "*arm64.msi", true, "App-arm64.msi"},
		{"emacs isn't macOS", []string{"emacs-29.4-installer.exe"}, "", false, "emacs-29.4-installer.exe"},
		{"no self-extracting archives", []string{"PortableGit-2.56.0.2-64-bit.7z.exe", "Tool-x64.zip.exe"}, "", false, "<nothing>"},
	}
	for _, tt := range tests {
		if got := name(Installer(assets(tt.files...), tt.pattern, tt.msi, "amd64")); got != tt.want {
			t.Errorf("%s: got %s, want %s", tt.desc, got, tt.want)
		}
	}
	for _, f := range []string{"App-macos-setup.exe", "App_osx.exe", "App-linux64.exe", "App.Portable.exe"} {
		if got := Installer(assets(f), "", false, "amd64"); got != nil {
			t.Errorf("%s should be ignored", f)
		}
	}
}

func TestInstallerOnArm64(t *testing.T) {
	if got := name(Installer(assets("App-x64.exe", "App-arm64.exe"), "", false, "arm64")); got != "App-arm64.exe" {
		t.Errorf("got %s", got)
	}
	if got := Installer(assets("App-x64.exe"), "", false, "arm64"); got != nil {
		t.Errorf("an x64-only release should be skipped on ARM64, got %s", got.Name)
	}
}

func TestInstallerRealReleases(t *testing.T) {
	// Copied from the actual releases (October 2026)
	git := assets("Git-2.56.0.2-64-bit.exe", "Git-2.56.0.2-64-bit.tar.bz2", "Git-2.56.0.2-arm64.exe", "Git-2.56.0.2-arm64.tar.bz2",
		"MinGit-2.56.0.2-32-bit.zip", "MinGit-2.56.0.2-64-bit.zip", "MinGit-2.56.0.2-arm64.zip",
		"pdbs-for-git-arm64-2.56.0.2-1.zip", "PortableGit-2.56.0.2-64-bit.7z.exe", "PortableGit-2.56.0.2-arm64.7z.exe")
	npp := assets("npp.8.9.8.1.checksums.sha256", "npp.8.9.8.1.Installer.arm64.exe", "npp.8.9.8.1.Installer.exe", "npp.8.9.8.1.Installer.x64.exe",
		"npp.8.9.8.1.Installer.x64.exe.sig", "npp.8.9.8.1.Installer.x64.msi", "npp.8.9.8.1.portable.x64.7z", "npp.8.9.8.1.portable.x64.zip")
	tests := []struct {
		release []github.Asset
		arch    Arch
		msi     bool
		want    string
	}{
		{git, "amd64", false, "Git-2.56.0.2-64-bit.exe"},
		{git, "arm64", false, "Git-2.56.0.2-arm64.exe"},
		{npp, "amd64", false, "npp.8.9.8.1.Installer.x64.exe"},
		{npp, "amd64", true, "npp.8.9.8.1.Installer.x64.msi"},
		{npp, "arm64", false, "npp.8.9.8.1.Installer.arm64.exe"},
	}
	for _, tt := range tests {
		if got := name(Installer(tt.release, "", tt.msi, tt.arch)); got != tt.want {
			t.Errorf("%s msi=%v: got %s, want %s", tt.arch, tt.msi, got, tt.want)
		}
	}
}

func TestArchive(t *testing.T) {
	tests := []struct {
		desc    string
		files   []string
		pattern string
		want    string
	}{
		{"the Windows x64 archive over source, Linux and generic ones", []string{"tool-1.0.tar.gz", "tool-src.zip", "tool-linux-x64.tar.gz", "tool-win-x64.zip"}, "", "tool-win-x64.zip"},
		{"DXVK, not dxvk-native", []string{"dxvk-native-2.5-steamrt-sniper.tar.gz", "dxvk-2.5.tar.gz"}, "", "dxvk-2.5.tar.gz"},
		{"no unsupported formats or debug symbols", []string{"tool.rar", "tool-win-x64-pdb.zip", "tool-debug.zip"}, "", "<nothing>"},
		{"the pattern instead", []string{"tool-win-x64.zip", "tool-full.7z"}, "*full.7z", "tool-full.7z"},
		{"not arm64 when a generic Windows one exists", []string{"tool-win-arm64.zip", "tool-win.zip"}, "", "tool-win.zip"},
		{"nothing rather than another architecture", []string{"tool-win-arm64.zip", "tool-win-x86.zip"}, "", "<nothing>"},
		{"win32 is a Windows name, not 32-bit", []string{"tool-win32.zip"}, "", "tool-win32.zip"},
		{"Emacs and Macro aren't macOS", []string{"emacs-29-win64.zip"}, "", "emacs-29-win64.zip"},
		{"Macro isn't macOS", []string{"MacroTool-win64.zip"}, "", "MacroTool-win64.zip"},
	}
	for _, tt := range tests {
		if got := name(Archive(assets(tt.files...), tt.pattern, "amd64")); got != tt.want {
			t.Errorf("%s: got %s, want %s", tt.desc, got, tt.want)
		}
	}
	for _, f := range []string{"tool.zip", "tool.7z", "tool.tar.gz", "tool.tgz", "tool.tar.xz"} {
		if got := name(Archive(assets(f), "", "amd64")); got != f {
			t.Errorf("%s should be accepted, got %s", f, got)
		}
	}
}

func TestHasBareX86(t *testing.T) {
	tests := []struct {
		name    string
		allow64 bool
		want    bool
	}{
		{"App-x86.exe", false, true},
		{"App_x86_64.exe", false, false},
		{"App-x86_64.exe", false, false},
		{"App-x86_64.exe", true, true}, // on ARM64, x86_64 is another architecture too
		{"App_x86.exe", false, false},
		{"App.exe", false, false},
	}
	for _, tt := range tests {
		if got := hasBareX86(tt.name, tt.allow64); got != tt.want {
			t.Errorf("hasBareX86(%q, %v) = %v", tt.name, tt.allow64, got)
		}
	}
}
