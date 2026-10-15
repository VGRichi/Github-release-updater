package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cache remembers recent answers about repositories' latest releases, so asking again soon after
// (reforge list, then reforge update) doesn't spend GitHub's hourly limit again. Like a package
// manager's catalog, it's refreshed after MaxAge; set MaxAge to 0 to always ask GitHub.
type Cache struct {
	MaxAge   time.Duration
	path     string
	releases map[string]cacheEntry
	now      func() time.Time
}

type cacheEntry struct {
	Release *Release  `json:"release"` // nil: the repository has no releases
	Checked time.Time `json:"checked"`
}

const (
	// DefaultMaxAge is how long a check is reused.
	DefaultMaxAge = time.Hour
	// A release published shortly before it was checked may still be getting its files attached
	// (Reforge's own releases are), so that answer is only reused briefly.
	newRelease, newReleaseMaxAge = time.Hour, 5 * time.Minute
	// noStableMaxAge is how long "no stable release" is remembered, which saves a request per check
	// for repositories that only publish prereleases.
	noStableMaxAge = 24 * time.Hour
)

// OpenCache reads the cache file at path. A missing or broken file is an empty cache: it's only a
// shortcut, so losing it just means asking GitHub again.
func OpenCache(path string) *Cache {
	c := &Cache{MaxAge: DefaultMaxAge, path: path, releases: map[string]cacheEntry{}, now: time.Now}
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &c.releases)
	}
	return c
}

func key(repo, kind string) string { return strings.ToLower(repo) + " " + kind }

// recent returns the answer for repo's latest release (stable or with prereleases) if it was checked
// recently enough to reuse.
func (c *Cache) recent(repo string, prerelease bool) (*Release, bool) {
	e, ok := c.releases[key(repo, kind(prerelease))]
	if !ok {
		return nil, false
	}
	maxAge := c.MaxAge
	if e.Release != nil && e.Checked.Sub(e.Release.Published) < newRelease {
		maxAge = min(maxAge, newReleaseMaxAge)
	}
	return e.Release, c.now().Sub(e.Checked) < maxAge
}

func (c *Cache) noStable(repo string) bool {
	e, ok := c.releases[key(repo, "no stable")]
	return ok && c.now().Sub(e.Checked) < noStableMaxAge
}

func (c *Cache) put(repo string, prerelease bool, r *Release) {
	c.releases[key(repo, kind(prerelease))] = cacheEntry{r, c.now()}
	c.save()
}

func (c *Cache) putNoStable(repo string) {
	c.releases[key(repo, "no stable")] = cacheEntry{nil, c.now()}
	c.save()
}

// save writes the file through a temp file, so a crash never leaves it half-written.
func (c *Cache) save() {
	data, err := json.Marshal(c.releases)
	if err != nil || os.MkdirAll(filepath.Dir(c.path), 0o755) != nil {
		return
	}
	if os.WriteFile(c.path+".tmp", data, 0o644) == nil {
		os.Rename(c.path+".tmp", c.path)
	}
}

func kind(prerelease bool) string {
	if prerelease {
		return "prerelease"
	}
	return "stable"
}
