package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// tree writes files (relative path -> content) under root.
func tree(t *testing.T, root string, files map[string]string) string {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func listFiles(root string) []string {
	var files []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func makeZip(t *testing.T, path string, files map[string]string) string {
	t.Helper()
	f, _ := os.Create(path)
	w := zip.NewWriter(f)
	for name, content := range files {
		fw, _ := w.Create(name)
		fw.Write([]byte(content))
	}
	w.Close()
	f.Close()
	return path
}

func makeTarGz(t *testing.T, path string, files map[string]string) string {
	t.Helper()
	f, _ := os.Create(path)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg})
		tw.Write([]byte(content))
	}
	tw.Close()
	gz.Close()
	f.Close()
	return path
}

func TestExtractZipAndTarGz(t *testing.T) {
	files := map[string]string{"dxvk-2.5/x64/d3d11.dll": "new x64", "dxvk-2.5/x32/d3d11.dll": "new x32"}
	dir := t.TempDir()
	for _, file := range []string{
		makeZip(t, filepath.Join(dir, "a.zip"), files),
		makeTarGz(t, filepath.Join(dir, "a.tar.gz"), files),
		makeTarGz(t, filepath.Join(dir, "a.tgz"), files),
	} {
		dest := filepath.Join(dir, "out-"+filepath.Base(file))
		if err := Extract(file, dest); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if got := read(t, filepath.Join(dest, "dxvk-2.5", "x64", "d3d11.dll")); got != "new x64" {
			t.Errorf("%s: got %q", file, got)
		}
	}
}

func TestExtractRefusesPathsOutsideTheFolder(t *testing.T) {
	dir := t.TempDir()
	for _, file := range []string{
		makeZip(t, filepath.Join(dir, "evil.zip"), map[string]string{"../evil.dll": "x"}),
		makeTarGz(t, filepath.Join(dir, "evil.tar.gz"), map[string]string{"../../evil.dll": "x"}),
	} {
		err := Extract(file, filepath.Join(dir, "out"))
		if err == nil || !strings.Contains(err.Error(), "outside its folder") {
			t.Errorf("%s: got %v", file, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.dll")); err == nil {
		t.Error("a file was written outside the folder")
	}
}

func TestContentRoot(t *testing.T) {
	tests := []struct {
		desc      string
		files     map[string]string
		subfolder string
		want      string // relative to the extracted folder
	}{
		{"steps into a single wrapper folder", map[string]string{"dxvk-2.5/x64/a.dll": "", "dxvk-2.5/x32/a.dll": ""}, "", "dxvk-2.5"},
		{"the subfolder inside the wrapper", map[string]string{"dxvk-2.5/x64/a.dll": "", "dxvk-2.5/x32/a.dll": ""}, "x64", "dxvk-2.5/x64"},
		{"a nested subfolder by its last name", map[string]string{"pkg/bin/x64/a.dll": "", "pkg/readme.txt": ""}, "x64", "pkg/bin/x64"},
		{"files at the top", map[string]string{"a.dll": "", "b.dll": ""}, "", "."},
	}
	for _, tt := range tests {
		root := tree(t, t.TempDir(), tt.files)
		got, err := ContentRoot(root, tt.subfolder)
		want := filepath.Join(root, filepath.FromSlash(tt.want))
		if err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %q", tt.desc, got, err, want)
		}
	}
	root := tree(t, t.TempDir(), map[string]string{"a/b.dll": "", "c.dll": ""})
	if _, err := ContentRoot(root, "x64"); err == nil || !strings.Contains(err.Error(), "'x64' was not found") {
		t.Errorf("missing subfolder: %v", err)
	}
}

func TestCopyBacksUpKeepsRenamesAndIncludes(t *testing.T) {
	src := tree(t, t.TempDir(), map[string]string{
		"d3d11.dll": "new d3d11", "dxgi.dll": "new dxgi", "OptiScaler.dll": "new opti",
		"OptiScaler.ini": "default settings", "readme.txt": "docs", "bin/tool.dll": "new tool",
	})
	dest := tree(t, t.TempDir(), map[string]string{"d3d11.dll": "old d3d11", "OptiScaler.ini": "my settings"})
	backup := filepath.Join(t.TempDir(), "backup")
	var j Journal
	n, err := Copy(src, dest, backup, Options{
		Include: []string{"*.dll", "*.ini"},
		Keep:    []string{"*.ini"},
		Rename:  map[string]string{"OptiScaler.dll": "dxgi-opti.dll"},
	}, &j)
	if err != nil {
		t.Fatal(err)
	}
	if got := listFiles(dest); strings.Join(got, ",") != "OptiScaler.ini,bin/tool.dll,d3d11.dll,dxgi-opti.dll,dxgi.dll" {
		t.Errorf("files: %v", got)
	}
	if read(t, filepath.Join(dest, "OptiScaler.ini")) != "my settings" {
		t.Error("-Keep: the settings file was overwritten")
	}
	if read(t, filepath.Join(dest, "dxgi-opti.dll")) != "new opti" {
		t.Error("-Rename didn't rename")
	}
	if read(t, filepath.Join(backup, "d3d11.dll")) != "old d3d11" {
		t.Error("the replaced file wasn't backed up")
	}
	if n != 4 || len(j) != 4 {
		t.Errorf("copied %d, journal %d; want 4", n, len(j))
	}
}

func TestCopyIncludeByPath(t *testing.T) {
	src := tree(t, t.TempDir(), map[string]string{"bin/a.dll": "a", "lib/b.dll": "b"})
	dest := t.TempDir()
	var j Journal
	Copy(src, dest, t.TempDir(), Options{Include: []string{`bin\*`}}, &j)
	if got := listFiles(dest); len(got) != 1 || got[0] != "bin/a.dll" {
		t.Errorf("files: %v", got)
	}
}

func TestUndoPutsEverythingBack(t *testing.T) {
	// The copy fails partway (a folder sits where a file should go, like a locked file would),
	// and Undo restores the files already replaced and deletes the ones added
	src := tree(t, t.TempDir(), map[string]string{"a.dll": "new a", "b.dll": "new b", "z.dll": "new z"})
	dest := tree(t, t.TempDir(), map[string]string{"a.dll": "old a"})
	os.MkdirAll(filepath.Join(dest, "z.dll", "blocker"), 0o755)
	var j Journal
	_, err := Copy(src, dest, filepath.Join(t.TempDir(), "backup"), Options{}, &j)
	if err == nil {
		t.Fatal("want the copy to fail")
	}
	if errs := j.Undo(); len(errs) != 0 {
		t.Fatal(errs)
	}
	if read(t, filepath.Join(dest, "a.dll")) != "old a" {
		t.Error("the replaced file wasn't put back")
	}
	if _, err := os.Stat(filepath.Join(dest, "b.dll")); !os.IsNotExist(err) {
		t.Error("the added file wasn't deleted")
	}
}

func TestPruneBackups(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2026-01-01_10-00-00", "2026-03-01_10-00-00", "2026-02-01_10-00-00", "2026-04-01_10-00-00", "not-a-backup"} {
		os.MkdirAll(filepath.Join(dir, name), 0o755)
	}
	PruneBackups(dir, 3)
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "2026-02-01_10-00-00,2026-03-01_10-00-00,2026-04-01_10-00-00,not-a-backup" {
		t.Errorf("left: %v", names)
	}
}

func TestExtractRefusesZipBombs(t *testing.T) {
	realRatio, realMin := maxRatio, minLimit
	t.Cleanup(func() { maxRatio, minLimit = realRatio, realMin })
	maxRatio, minLimit = 1, 1000
	dir := t.TempDir()
	big := strings.Repeat("a", 5000) // compresses to far less than 1000 bytes
	for _, file := range []string{
		makeZip(t, filepath.Join(dir, "bomb.zip"), map[string]string{"a.txt": big}),
		makeTarGz(t, filepath.Join(dir, "bomb.tar.gz"), map[string]string{"a.txt": big}),
	} {
		if err := Extract(file, filepath.Join(dir, "out")); err == nil || !strings.Contains(err.Error(), "zip bomb") {
			t.Errorf("%s: got %v", file, err)
		}
	}
}

func TestNoLinks(t *testing.T) {
	dir := tree(t, t.TempDir(), map[string]string{"a/b.dll": "x"})
	if err := noLinks(dir, "tool.7z"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, "link")); err != nil {
		t.Skip("can't make links here:", err)
	}
	if err := noLinks(dir, "tool.7z"); err == nil || !strings.Contains(err.Error(), "link") {
		t.Errorf("got %v", err)
	}
}

func TestRenameAndSubfolderStayInside(t *testing.T) {
	src := tree(t, t.TempDir(), map[string]string{"a.dll": "x"})
	var j Journal
	for _, to := range []string{`..\..\evil.dll`, "../evil.dll", "sub/a.dll", ".."} {
		if _, err := Copy(src, t.TempDir(), t.TempDir(), Options{Rename: map[string]string{"a.dll": to}}, &j); err == nil {
			t.Errorf("rename to %q was allowed", to)
		}
	}
	if _, err := ContentRoot(src, "../.."); err == nil {
		t.Error("a subfolder outside the archive was allowed")
	}
}

func TestMoveKeepsTheSourceWhenItCantRename(t *testing.T) {
	src := tree(t, t.TempDir(), map[string]string{"a/b.dll": "x"})
	if err := Move(src, filepath.Join(t.TempDir(), "missing", "folder", "x")); err == nil {
		t.Fatal("a move into a missing folder should fail")
	}
	if read(t, filepath.Join(src, "a", "b.dll")) != "x" {
		t.Error("the source must be left as it was")
	}
}
