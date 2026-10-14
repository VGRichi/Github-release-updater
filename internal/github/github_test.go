package github

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGitHub answers API paths from a map; anything else is a 404.
func fakeGitHub(t *testing.T, replies map[string]string, check func(*http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		body, ok := replies[r.URL.RequestURI()]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), API: srv.URL}
}

func TestLatestStable(t *testing.T) {
	c := fakeGitHub(t, map[string]string{"/repos/o/r/releases/latest": `{"tag_name":"v1.0.0"}`}, nil)
	r, err := c.Latest("o/r", false)
	if err != nil || r.Tag != "v1.0.0" {
		t.Fatalf("got %v, %v", r, err)
	}
}

func TestLatestFallsBackToPrereleaseWithoutStable(t *testing.T) {
	c := fakeGitHub(t, map[string]string{
		"/repos/o/r/releases?per_page=10": `[{"tag_name":"v3-draft","draft":true},{"tag_name":"v2.0.0-beta","prerelease":true}]`,
	}, nil)
	for _, pre := range []bool{false, true} {
		r, err := c.Latest("o/r", pre)
		if err != nil || r.Tag != "v2.0.0-beta" {
			t.Fatalf("prerelease=%v: got %v, %v", pre, r, err)
		}
	}
}

func TestLatestWithNoReleases(t *testing.T) {
	c := fakeGitHub(t, map[string]string{"/repos/o/r/releases?per_page=10": `[]`}, nil)
	if r, err := c.Latest("o/r", false); r != nil || err != nil {
		t.Fatalf("got %v, %v", r, err)
	}
}

func TestLatestRepositoryNotFound(t *testing.T) {
	c := fakeGitHub(t, nil, nil)
	if _, err := c.Latest("o/missing", false); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("got %v", err)
	}
}

func TestRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), API: srv.URL}
	if _, err := c.Latest("o/r", false); err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("got %v", err)
	}
}

func TestTokenIsSentToTheAPI(t *testing.T) {
	var auth string
	c := fakeGitHub(t, map[string]string{"/repos/o/r/releases/latest": `{"tag_name":"v1"}`}, func(r *http.Request) { auth = r.Header.Get("Authorization") })
	c.Token = "secret"
	c.Latest("o/r", false)
	if auth != "Bearer secret" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestDownload(t *testing.T) {
	payload := []byte("installer bytes")
	sum := sha256.Sum256(payload)
	good := "sha256:" + hex.EncodeToString(sum[:])
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Write(payload)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), Token: "secret"}

	for _, digest := range []string{good, strings.ToUpper(good[:7]) + good[7:], ""} {
		dir := t.TempDir()
		file, err := c.Download(Asset{Name: "App.msi", URL: srv.URL + "/App.msi", Digest: digest}, dir)
		if err != nil || filepath.Base(file) != "App.msi" {
			t.Fatalf("digest %q: %v, %v", digest, file, err)
		}
	}
	if auth != "" {
		t.Error("the token must not be sent with downloads")
	}

	dir := t.TempDir()
	_, err := c.Download(Asset{Name: "App.msi", URL: srv.URL + "/App.msi", Digest: "sha256:0000"}, dir)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("got %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Error("a file that fails the checksum must be deleted")
	}
}

func TestDownloadChecksTheSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("12345")) }))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	if _, err := c.Download(Asset{Name: "a.zip", URL: srv.URL, Size: 5}, t.TempDir()); err != nil {
		t.Fatalf("right size: %v", err)
	}
	dir := t.TempDir()
	_, err := c.Download(Asset{Name: "a.zip", URL: srv.URL, Size: 9}, dir) // GitHub lists another size
	if err == nil || !strings.Contains(err.Error(), "the download is broken") {
		t.Fatalf("wrong size: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Error("a broken download must be deleted")
	}
}

func TestParseRepo(t *testing.T) {
	good := map[string]string{
		"storytold/photocraft":                    "storytold/photocraft",
		" https://github.com/doitsujin/dxvk/ ":    "doitsujin/dxvk",
		"https://www.github.com/o/r.git":          "o/r",
		"HTTP://GitHub.com/optiscaler/OptiScaler": "optiscaler/OptiScaler",
	}
	for in, want := range good {
		if got, ok := ParseRepo(in); !ok || got != want {
			t.Errorf("ParseRepo(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"not a repo", "photocraft", "https://gitlab.com/o/r", "o/r/extra"} {
		if _, ok := ParseRepo(in); ok {
			t.Errorf("ParseRepo(%q) should fail", in)
		}
	}
}
