// Package github reads releases from GitHub and downloads their files.
package github

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Release is a GitHub release.
type Release struct {
	Tag       string    `json:"tag_name"`
	Draft     bool      `json:"draft"`
	Published time.Time `json:"published_at"`
	Assets    []Asset   `json:"assets"`
}

// Asset is a file attached to a release. Digest is GitHub's checksum, like "sha256:ab12...".
type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// Client talks to the GitHub API. Token is optional; without it GitHub allows 60 requests an hour.
type Client struct {
	HTTP  *http.Client
	API   string
	Token string
	Cache *Cache // optional: recent answers, reused instead of asking again
	now   func() time.Time
}

// New returns a client for api.github.com.
func New(token string) *Client {
	return &Client{HTTP: http.DefaultClient, API: "https://api.github.com", Token: token}
}

// StatusError is a GitHub reply that isn't a success.
type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string { return e.Message }

// Latest returns the newest release: the latest stable one, or with prerelease (or when there's no
// stable release) the newest one that isn't a draft. It returns nil when there are no releases.
// With a Cache, a recent answer is reused instead of asking GitHub again.
func (c *Client) Latest(repo string, prerelease bool) (*Release, error) {
	if c.Cache != nil {
		if r, ok := c.Cache.recent(repo, prerelease); ok {
			return r, nil
		}
	}
	r, err := c.latest(repo, prerelease)
	if err == nil && c.Cache != nil {
		c.Cache.put(repo, prerelease, r)
	}
	return r, err
}

func (c *Client) latest(repo string, prerelease bool) (*Release, error) {
	// A repository known to have no stable release goes straight to the list, saving a request
	if !prerelease && (c.Cache == nil || !c.Cache.noStable(repo)) {
		var r Release
		err := c.get("/repos/"+repo+"/releases/latest", &r)
		var status *StatusError
		if err == nil {
			return &r, nil
		} else if !errors.As(err, &status) || status.Status != http.StatusNotFound {
			return nil, err
		}
		if c.Cache != nil {
			c.Cache.putNoStable(repo)
		}
	}
	var all []Release
	if err := c.get("/repos/"+repo+"/releases?per_page=10", &all); err != nil {
		return nil, err
	}
	for i := range all {
		if !all[i].Draft {
			return &all[i], nil
		}
	}
	return nil, nil
}

func (c *Client) get(path string, v any) error {
	req, err := http.NewRequest(http.MethodGet, c.API+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Reforge")
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return &StatusError{resp.StatusCode, "repository not found on GitHub (or it's private)"}
	case (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return &StatusError{resp.StatusCode, c.limitMessage(resp.Header)}
	case resp.StatusCode >= 300:
		return &StatusError{resp.StatusCode, fmt.Sprintf("GitHub replied %s", resp.Status)}
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// limitMessage says GitHub's hourly limit was reached, when it resets, and how to get more.
func (c *Client) limitMessage(h http.Header) string {
	limit, when := h.Get("X-RateLimit-Limit"), "soon"
	if limit == "" {
		limit = "60"
	}
	if reset, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		now := time.Now()
		if c.now != nil {
			now = c.now()
		}
		minutes := max(int(math.Ceil(time.Unix(reset, 0).Sub(now).Minutes())), 1)
		when = fmt.Sprintf("in %d minute", minutes)
		if minutes > 1 {
			when += "s"
		}
	}
	msg := fmt.Sprintf("GitHub's limit of %s checks an hour was reached; it resets %s", limit, when)
	if c.Token == "" {
		msg += ". For 5,000 an hour, sign in with the GitHub CLI (gh auth login) or set GITHUB_TOKEN to a GitHub token"
	}
	return msg
}

// Download saves the asset into dir and checks it against GitHub's checksum, deleting it if it doesn't
// match. It doesn't send the token: release files are public, and the download host isn't GitHub's API.
func (c *Client) Download(a Asset, dir string) (string, error) {
	file := filepath.Join(dir, filepath.Base(a.Name))
	req, err := http.NewRequest(http.MethodGet, a.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Reforge")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("downloading %s: %s", a.Name, resp.Status)
	}
	out, err := os.Create(file)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, hash), resp.Body)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(file)
		return "", fmt.Errorf("downloading %s: %w", a.Name, err)
	}
	// The size GitHub lists catches a cut-off or wrong download, even for files without a checksum
	if a.Size > 0 && n != a.Size {
		os.Remove(file)
		return "", fmt.Errorf("%s is %d bytes, but GitHub lists %d; the download is broken. Nothing was installed", a.Name, n, a.Size)
	}
	if a.Digest != "" && !strings.EqualFold(a.Digest, "sha256:"+hex.EncodeToString(hash.Sum(nil))) {
		os.Remove(file)
		return "", fmt.Errorf("checksum mismatch for %s. Nothing was installed", a.Name)
	}
	return file, nil
}

// Repo is a repository found by Search.
type Repo struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Stars       int    `json:"stargazers_count"`
	Archived    bool   `json:"archived"`
}

// Search finds repositories with the words in their name, in GitHub's order of relevance (forks are
// left out). GitHub allows about 10 searches a minute without a token.
func (c *Client) Search(words string) ([]Repo, error) {
	var found struct {
		Items []Repo `json:"items"`
	}
	err := c.get("/search/repositories?per_page=20&q="+url.QueryEscape(words+" in:name"), &found)
	return found.Items, err
}

var (
	repoLink = regexp.MustCompile(`^(?i)https?://(www\.)?github\.com/`)
	repoName = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)
)

// ParseRepo turns "owner/repo" or a GitHub link into "owner/repo".
func ParseRepo(s string) (string, bool) {
	repo := repoLink.ReplaceAllString(strings.TrimSpace(s), "")
	repo = strings.TrimSuffix(strings.TrimRight(repo, "/"), ".git")
	return repo, repoName.MatchString(repo)
}
