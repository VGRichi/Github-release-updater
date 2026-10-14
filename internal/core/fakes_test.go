package core

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/VGRichi/Reforge/internal/github"
	"github.com/VGRichi/Reforge/internal/system"
)

// fakeGitHub serves one release; downloads write payload and check the digest like the real client.
type fakeGitHub struct {
	release   *github.Release
	err       error
	payload   []byte
	downloads []string
	latest    []bool // the prerelease flag of each Latest call
}

func (f *fakeGitHub) Latest(repo string, prerelease bool) (*github.Release, error) {
	f.latest = append(f.latest, prerelease)
	return f.release, f.err
}

func (f *fakeGitHub) Download(a github.Asset, dir string) (string, error) {
	f.downloads = append(f.downloads, a.Name)
	if a.Digest != "" && a.Digest != digest(f.payload) {
		return "", errors.New("checksum mismatch for " + a.Name + ". Nothing was installed")
	}
	file := filepath.Join(dir, a.Name)
	return file, os.WriteFile(file, f.payload, 0o644)
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type run struct{ file, args string }

// fakeSystem is a Windows PC: installed apps, and installers that change them.
type fakeSystem struct {
	programs    []system.Program
	after       []system.Program // the installed apps once an installer has run, if set
	admin       bool
	signer      string // who signed the installers
	signerErr   error  // a broken signature
	exitCode    int
	runs        []run
	origins     map[string]string // file -> download note
	winget      []system.Package
	wingetCalls int
}

func (s *fakeSystem) Programs() ([]system.Program, error) { return s.programs, nil }

func (s *fakeSystem) RunInstaller(file, args string) (int, error) {
	s.runs = append(s.runs, run{file, args})
	if s.after != nil {
		s.programs = s.after
	}
	return s.exitCode, nil
}

func (s *fakeSystem) IsAdmin() bool                     { return s.admin }
func (s *fakeSystem) Signer(string) (string, error)     { return s.signer, s.signerErr }
func (s *fakeSystem) DownloadOrigin(file string) string { return s.origins[file] }
func (s *fakeSystem) WingetPackages() []system.Package {
	s.wingetCalls++
	return s.winget
}

// fakeAsker answers questions in order; with no answers left it fails the test.
type fakeAsker struct {
	t         *testing.T
	answers   []int
	questions []string
}

func (a *fakeAsker) Choose(question string, answers []string, def int) int {
	a.questions = append(a.questions, question)
	if len(a.answers) == 0 {
		a.t.Errorf("unexpected question: %s", question)
		return def
	}
	ans := a.answers[0]
	a.answers = a.answers[1:]
	return ans
}

type testEngine struct {
	*Engine
	gh       *fakeGitHub
	sys      *fakeSystem
	ask      *fakeAsker
	warnings []string
}

func newEngine(t *testing.T) *testEngine {
	t.Helper()
	te := &testEngine{
		gh:  &fakeGitHub{payload: []byte("installer bytes")},
		sys: &fakeSystem{},
		ask: &fakeAsker{t: t},
	}
	te.Engine = &Engine{
		GitHub: te.gh, System: te.sys, Ask: te.ask, Arch: "amd64",
		WorkRoot: t.TempDir(), BackupRoot: filepath.Join(t.TempDir(), "backups"), BackupsToKeep: 3,
		Warn: func(s string) { te.warnings = append(te.warnings, s) },
		Now:  func() time.Time { return time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC) },
	}
	return te
}

func asset(name string, payload []byte) github.Asset {
	a := github.Asset{Name: name, URL: "https://example.test/" + name}
	if payload != nil {
		a.Digest = digest(payload)
	}
	return a
}

func release(tag string, assets ...github.Asset) *github.Release {
	return &github.Release{Tag: tag, Assets: assets}
}

func program(name, version string, perUser, msi bool) system.Program {
	return system.Program{Name: name, Version: version, Key: `HKCU\Uninstall\` + name, PerUser: perUser, MSI: msi}
}

// zipOf builds a .zip in memory from relative path -> content.
func zipOf(files map[string]string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, _ := w.Create(name)
		f.Write([]byte(content))
	}
	w.Close()
	return buf.Bytes()
}

// writeTree writes files (relative path -> content) under root.
func writeTree(t *testing.T, root string, files map[string]string) string {
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
