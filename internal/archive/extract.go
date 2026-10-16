// Package archive extracts release archives and copies their files into place, with backups and rollback.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Extract unpacks file into dest. .zip and .tar.gz are handled in Go; .7z and .tar.xz need 7-Zip or
// the system's tar (built into Windows 10 and later, macOS and Linux).
func Extract(file, dest string) error {
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	// A small archive that unpacks to far more (a "zip bomb") would fill the drive
	left := max(info.Size()*maxRatio, minLimit)
	lower := strings.ToLower(file)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return unzip(file, dest, &left)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return untarGz(file, dest, &left)
	}
	if err := extractWithTool(file, dest); err != nil {
		return err
	}
	return noLinks(dest, filepath.Base(file))
}

// The unpacked size allowed: up to 100 times the archive's, or 2 GB (tests lower it).
var maxRatio, minLimit int64 = 100, 2 << 30

// limited reads r, failing once more than *left bytes were read from all of an archive's files.
type limited struct {
	r    io.Reader
	left *int64
}

func (l limited) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	if *l.left -= int64(n); *l.left < 0 {
		return n, errors.New("the archive unpacks to far more than its size (a zip bomb?); not extracting it")
	}
	return n, err
}

// noLinks refuses links (and Windows junctions) that 7-Zip or tar extracted: copying through one could
// read or write files outside the folder. The Go extractors skip links.
func noLinks(dir, name string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return fmt.Errorf("%s has a link in it (%s); not using it", name, d.Name())
		}
		return err
	})
}

func unzip(file, dest string, left *int64) error {
	r, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target, err := inside(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFile(target, limited{src, left}, f.Mode())
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untarGz(file, dest string, left *int64) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := inside(dest, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(target, limited{tr, left}, h.FileInfo().Mode()); err != nil {
				return err
			}
		} // links and special files are skipped
	}
}

// inside joins name onto dest, refusing names that would land outside it (like "../../evil.dll").
func inside(dest, name string) (string, error) {
	target := filepath.Join(dest, filepath.FromSlash(name))
	if rel, err := filepath.Rel(dest, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("the archive has a file outside its folder (%s); not extracting it", name)
	}
	return target, nil
}

func writeFile(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	perm := mode.Perm() | 0o600
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, r)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}

func extractWithTool(file, dest string) error {
	name := filepath.Base(file)
	is7z := strings.HasSuffix(strings.ToLower(file), ".7z")
	if sevenZip := findSevenZip(); is7z && sevenZip != "" {
		if out, err := exec.Command(sevenZip, "x", file, "-o"+dest, "-y").CombinedOutput(); err != nil {
			return fmt.Errorf("7-Zip could not extract %s: %s", name, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := exec.Command(systemTar(), "-xf", file, "-C", dest).Run(); err != nil {
		if is7z {
			return fmt.Errorf("could not extract %s. Install 7-Zip and try again (on Windows: winget install --id 7zip.7zip)", name)
		}
		return fmt.Errorf("could not extract %s: %w", name, err)
	}
	return nil
}

func findSevenZip() string {
	if runtime.GOOS == "windows" {
		for _, dir := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
			if exe := filepath.Join(dir, "7-Zip", "7z.exe"); dir != "" && fileExists(exe) {
				return exe
			}
		}
	}
	for _, name := range []string{"7z", "7zz"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// systemTar is Windows' own tar (bsdtar, which reads .7z on recent Windows 11), not Git's GNU tar.
func systemTar() string {
	if root := os.Getenv("SystemRoot"); runtime.GOOS == "windows" && root != "" {
		if exe := filepath.Join(root, "System32", "tar.exe"); fileExists(exe) {
			return exe
		}
	}
	return "tar"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
