package applist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingOrEmpty(t *testing.T) {
	dir := t.TempDir()
	for _, content := range []string{"", "  \r\n ", "\xef\xbb\xbf"} {
		path := filepath.Join(dir, "apps.json")
		os.WriteFile(path, []byte(content), 0o644)
		if apps, err := Load(path); err != nil || len(apps) != 0 {
			t.Errorf("Load(%q) = %v, %v; want an empty list", content, apps, err)
		}
	}
	if apps, err := Load(filepath.Join(dir, "missing.json")); err != nil || apps != nil {
		t.Errorf("missing file: %v, %v", apps, err)
	}
}

func TestLoadPowerShellList(t *testing.T) {
	// As written by the PowerShell module in Windows PowerShell 5.1: a BOM, and a single string
	// where a list is expected
	path := filepath.Join(t.TempDir(), "apps.json")
	os.WriteFile(path, []byte("\xef\xbb\xbf"+`[
  { "Name": "PhotoCraft", "Repository": "storytold/photocraft", "AllowUnverified": true },
  { "Name": "DXVK (Witcher)", "Repository": "doitsujin/dxvk", "Type": "archive",
    "Destination": "C:\\Games\\W3", "Keep": ["*.ini"], "Rename": { "a.dll": "b.dll" }, "InstalledVersion": "v2.5" }
]`), 0o644)
	apps, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 || apps[0].Name != "PhotoCraft" || !apps[0].AllowUnverified || apps[0].IsArchive() {
		t.Fatalf("first app: %+v", apps[0])
	}
	dxvk := apps[1]
	if !dxvk.IsArchive() || len(dxvk.Destination) != 1 || dxvk.Destination[0] != `C:\Games\W3` ||
		dxvk.Keep[0] != "*.ini" || dxvk.Rename["a.dll"] != "b.dll" || dxvk.InstalledVersion != "v2.5" {
		t.Fatalf("second app: %+v", dxvk)
	}
}

func TestLoadSingleObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.json")
	os.WriteFile(path, []byte(`{ "Name": "One", "Repository": "o/one" }`), 0o644)
	apps, err := Load(path)
	if err != nil || len(apps) != 1 || apps[0].Name != "One" {
		t.Fatalf("got %v, %v", apps, err)
	}
}

func TestLoadBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.json")
	os.WriteFile(path, []byte(`[ { "Name": `), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("want an error for a broken list, so it's never overwritten")
	}
}

func TestSaveRoundTripWithoutTempFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "apps.json")
	apps := []*App{{Name: "Old", Repository: "o/old"}}
	if err := Save(path, apps); err != nil {
		t.Fatal(err)
	}
	apps = []*App{{Name: "New", Repository: "o/new", Destination: Strings{`C:\A`}}}
	if err := Save(path, apps); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || len(got) != 1 || got[0].Name != "New" || got[0].Destination[0] != `C:\A` {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("the temp file was left behind")
	}
}

func TestSaveOmitsEmptyFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.json")
	Save(path, []*App{{Name: "A", Repository: "o/a"}})
	data, _ := os.ReadFile(path)
	want := "[\n  {\n    \"Name\": \"A\",\n    \"Repository\": \"o/a\"\n  }\n]\n"
	if string(data) != want {
		t.Errorf("got\n%s\nwant\n%s", data, want)
	}
}

func TestFind(t *testing.T) {
	apps := []*App{{Name: "PhotoCraft"}, {Name: "DXVK"}}
	if Find(apps, "photocraft") != apps[0] || Find(apps, "nope") != nil {
		t.Error("Find should match names ignoring case")
	}
}
