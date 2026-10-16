package github

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		"/repos/o/r/releases?per_page=5": `[{"tag_name":"v3-draft","draft":true},{"tag_name":"v2.0.0-beta","prerelease":true}]`,
	}, nil)
	for _, pre := range []bool{false, true} {
		r, err := c.Latest("o/r", pre)
		if err != nil || r.Tag != "v2.0.0-beta" {
			t.Fatalf("prerelease=%v: got %v, %v", pre, r, err)
		}
	}
}

func TestLatestWithNoReleases(t *testing.T) {
	c := fakeGitHub(t, map[string]string{"/repos/o/r/releases?per_page=5": `[]`}, nil)
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

// releasesServer answers the API for o/r with one release (or only a prerelease), counting requests.
type releasesServer struct {
	published  time.Time
	onlyBeta   bool
	requests   int
	stableHits int
}

func (s *releasesServer) start(t *testing.T) *Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests++
		release := fmt.Sprintf(`{"tag_name":"v1.0.0","prerelease":%v,"published_at":%q}`, s.onlyBeta, s.published.Format(time.RFC3339))
		switch r.URL.RequestURI() {
		case "/repos/o/r/releases/latest":
			s.stableHits++
			if s.onlyBeta {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(release))
		case "/repos/o/r/releases?per_page=5":
			w.Write([]byte("[" + release + "]"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), API: srv.URL}
}

// withClock gives a cache a clock the test moves.
func withClock(c *Cache, now *time.Time) *Cache {
	c.now = func() time.Time { return *now }
	return c
}

func TestCacheReusesRecentChecks(t *testing.T) {
	now := time.Now()
	s := &releasesServer{published: now.Add(-48 * time.Hour)}
	c := s.start(t)
	path := filepath.Join(t.TempDir(), "cache.json")
	for range 3 { // later runs read the cache file again
		c.Cache = withClock(OpenCache(path), &now)
		if r, err := c.Latest("o/r", false); err != nil || r.Tag != "v1.0.0" {
			t.Fatalf("got %+v, %v", r, err)
		}
		c.Cache.Save()
	}
	if s.requests != 1 {
		t.Errorf("%d requests; checks within the hour should reuse the first", s.requests)
	}
	now = now.Add(61 * time.Minute) // an hour later, GitHub is asked again
	c.Latest("o/r", false)
	if s.requests != 2 {
		t.Errorf("%d requests after an hour", s.requests)
	}
}

func TestCacheReusesANewReleaseOnlyBriefly(t *testing.T) {
	// Published just now: its files may still be uploading, so the answer is reused for minutes, not an hour
	now := time.Now()
	s := &releasesServer{published: now}
	c := s.start(t)
	c.Cache = withClock(OpenCache(filepath.Join(t.TempDir(), "cache.json")), &now)
	c.Latest("o/r", false)
	now = now.Add(2 * time.Minute)
	c.Latest("o/r", false)
	now = now.Add(5 * time.Minute)
	c.Latest("o/r", false)
	if s.requests != 2 {
		t.Errorf("%d requests", s.requests)
	}
}

func TestCacheRemembersThereIsNoStableRelease(t *testing.T) {
	now := time.Now()
	s := &releasesServer{published: now.Add(-48 * time.Hour), onlyBeta: true}
	c := s.start(t)
	c.Cache = withClock(OpenCache(filepath.Join(t.TempDir(), "cache.json")), &now)
	c.Latest("o/r", false) // asks for the stable release, gets none, then the list: 2 requests
	now = now.Add(2 * time.Hour)
	if r, err := c.Latest("o/r", false); err != nil || r.Tag != "v1.0.0" {
		t.Fatalf("got %+v, %v", r, err)
	}
	if s.requests != 3 || s.stableHits != 1 {
		t.Errorf("%d requests, %d for the stable release; the second check should skip it", s.requests, s.stableHits)
	}
}

func TestBrokenCacheIsIgnored(t *testing.T) {
	s := &releasesServer{published: time.Now().Add(-48 * time.Hour)}
	c := s.start(t)
	path := filepath.Join(t.TempDir(), "cache.json")
	os.WriteFile(path, []byte("{ not json"), 0o644)
	c.Cache = OpenCache(path)
	if r, err := c.Latest("o/r", false); err != nil || r.Tag != "v1.0.0" {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestLimitMessage(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "60")
	h.Set("X-RateLimit-Reset", "1000700") // 11 minutes 40 seconds from now
	c := &Client{now: func() time.Time { return now }}
	if got := c.limitMessage(h); got != "GitHub's limit of 60 checks an hour was reached; it resets in 12 minutes. For 5,000 an hour, sign in with the GitHub CLI (gh auth login) or set GITHUB_TOKEN to a GitHub token" {
		t.Errorf("without a token: %q", got)
	}
	c.Token = "x"
	h.Set("X-RateLimit-Limit", "5000")
	h.Set("X-RateLimit-Reset", "1000030")
	if got := c.limitMessage(h); got != "GitHub's limit of 5000 checks an hour was reached; it resets in 1 minute" {
		t.Errorf("with a token: %q", got)
	}
}

func TestStopsAskingOnceTheLimitIsReached(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), API: srv.URL}
	for _, repo := range []string{"o/a", "o/b", "o/c"} {
		if _, err := c.Latest(repo, false); err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("%s: got %v", repo, err)
		}
	}
	if requests != 1 {
		t.Errorf("%d requests; after the limit, GitHub shouldn't be asked again", requests)
	}
}

func TestTokenIsLookedUpOnlyWhenNeeded(t *testing.T) {
	lookups := 0
	c := fakeGitHub(t, map[string]string{"/repos/o/r/releases/latest": `{"tag_name":"v1"}`}, nil)
	c.Auth = func() string { lookups++; return "secret" }
	c.Cache = OpenCache(filepath.Join(t.TempDir(), "cache.json"))
	c.Cache.put("o/r", false, &Release{Tag: "v1"})
	c.Latest("o/r", false) // from the cache: no request, so no token needed (no 'gh auth token' run)
	if lookups != 0 {
		t.Fatalf("%d token lookups for a cached answer", lookups)
	}
	c.Latest("o/r", true)
	c.Latest("o/s", true)
	if lookups != 1 {
		t.Errorf("%d token lookups; the token is found once", lookups)
	}
}

func TestCacheIgnoresTimesInTheFuture(t *testing.T) {
	now := time.Now()
	s := &releasesServer{published: now.Add(-48 * time.Hour)}
	c := s.start(t)
	path := filepath.Join(t.TempDir(), "cache.json")
	os.WriteFile(path, fmt.Appendf(nil, `{"o/r stable":{"release":{"tag_name":"v0.0.1"},"checked":%q}}`, now.Add(1000*time.Hour).Format(time.RFC3339)), 0o644)
	c.Cache = OpenCache(path)
	if r, _ := c.Latest("o/r", false); r == nil || r.Tag != "v1.0.0" || s.requests != 1 {
		t.Fatalf("got %+v after %d requests", r, s.requests)
	}
}

func TestDownloadFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("0123456789")) // more than listed
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	for path, want := range map[string]string{"/missing": "404", "/long": "the download is broken"} {
		dir := t.TempDir()
		_, err := c.Download(Asset{Name: "a.zip", URL: srv.URL + path, Size: 4}, dir)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", path, err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("%s: a failed download must leave nothing", path)
		}
	}
}
