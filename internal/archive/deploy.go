package archive

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/VGRichi/Reforge/internal/wildcard"
)

// ContentRoot finds the folder to copy from: subfolder (like DXVK's x64) if given, after stepping into
// single wrapper folders like dxvk-2.5/.
func ContentRoot(extracted, subfolder string) (string, error) {
	root := extracted
	for {
		if subfolder != "" {
			if direct := filepath.Join(root, subfolder); isDir(direct) {
				return direct, nil
			}
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return "", err
		}
		if len(entries) != 1 || !entries[0].IsDir() {
			break
		}
		root = filepath.Join(root, entries[0].Name())
	}
	if subfolder == "" {
		return root, nil
	}
	leaf := filepath.Base(filepath.FromSlash(subfolder))
	if filepath.Base(root) == leaf {
		return root, nil
	}
	var found string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root && d.Name() == leaf {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("folder '%s' was not found inside the archive", subfolder)
	}
	return found, nil
}

// Options choose which files are copied and how.
type Options struct {
	Include []string          // only files matching these wildcards (by name or path), if set
	Keep    []string          // files that are added if missing but never overwritten, like settings
	Rename  map[string]string // file name -> new name
}

// Change is one file copied into place: Backup holds the file it replaced, or is empty if it was new.
type Change struct{ Target, Backup string }

// Journal lists every change, so Undo can put things back.
type Journal []Change

// Copy copies the files under src into dest, backing up each file it replaces into backupDir.
// It stops at the first error; the journal then holds what was already done.
func Copy(src, dest, backupDir string, opt Options, journal *Journal) (int, error) {
	copied := 0
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		name := d.Name()
		if len(opt.Include) > 0 && !matchAny(opt.Include, name, rel) {
			return nil
		}
		leaf := name
		if to, ok := opt.Rename[name]; ok {
			leaf = to
		}
		relTarget := filepath.Join(filepath.Dir(rel), leaf)
		target := filepath.Join(dest, relTarget)

		backup := ""
		if fileExists(target) {
			if matchAny(opt.Keep, leaf, name, relTarget, rel) {
				return nil // your settings stay as they are
			}
			backup = filepath.Join(backupDir, relTarget)
			if err := copyFile(target, backup); err != nil {
				return err
			}
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		*journal = append(*journal, Change{target, backup})
		copied++
		return nil
	})
	return copied, err
}

// Undo puts back every change, newest first: replaced files from their backup, added files deleted.
// It returns the files it couldn't put back.
func (j Journal) Undo() []error {
	var errs []error
	for i := len(j) - 1; i >= 0; i-- {
		var err error
		if j[i].Backup != "" {
			err = copyFile(j[i].Backup, j[i].Target)
		} else {
			err = os.Remove(j[i].Target)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("couldn't put back %s: %w", j[i].Target, err))
		}
	}
	return errs
}

var backupName = regexp.MustCompile(`^\d{4}-\d\d-\d\d_\d\d-\d\d-\d\d$`)

// BackupFolderName is the name of a backup folder for an update at time t; sorting names sorts by time.
const BackupFolderName = "2006-01-02_15-04-05"

// PruneBackups deletes all but the newest keep backup folders in appBackupDir.
func PruneBackups(appBackupDir string, keep int) {
	entries, _ := os.ReadDir(appBackupDir)
	var names []string
	for _, e := range entries {
		if e.IsDir() && backupName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i := keep; i < len(names); i++ {
		os.RemoveAll(filepath.Join(appBackupDir, names[i]))
	}
}

func matchAny(patterns []string, names ...string) bool {
	for _, p := range patterns {
		for _, n := range names {
			if wildcard.MatchPath(p, n) {
				return true
			}
		}
	}
	return false
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	return writeFile(dst, in, info.Mode())
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
