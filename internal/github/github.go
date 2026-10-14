// Package github reads releases from GitHub and downloads their files.
package github

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Release is a GitHub release.
type Release struct {
	Tag    string  `json:"tag_name"`
	Draft  bool    `json:"draft"`
	Assets []Asset `json:"assets"`
}

// Asset is a file attached to a release. Digest is GitHub's checksum, like "sha256:ab12...".
type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// Client talks to the GitHub API. Token is optional; without it GitHub allows about 60 requests an hour.
type Client struct {
	HTTP  *http.Client
	API   string
	Token string
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
func (c *Client) Latest(repo string, prerelease bool) (*Release, error) {
	if !prerelease {
		var r Release
		err := c.get("/repos/"+repo+"/releases/latest", &r)
		var status *StatusError
		if err == nil {
			return &r, nil
		} else if !errors.As(err, &status) || status.Status != http.StatusNotFound {
			return nil, err
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
		return &StatusError{resp.StatusCode, "GitHub's limit of about 60 checks an hour was reached; set GITHUB_TOKEN to a GitHub token for more"}
	case resp.StatusCode >= 300:
		return &StatusError{resp.StatusCode, fmt.Sprintf("GitHub replied %s", resp.Status)}
	}
	return json.NewDecoder(resp.Body).Decode(v)
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
	_, err = io.Copy(io.MultiWriter(out, hash), resp.Body)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(file)
		return "", fmt.Errorf("downloading %s: %w", a.Name, err)
	}
	if a.Digest != "" && !strings.EqualFold(a.Digest, "sha256:"+hex.EncodeToString(hash.Sum(nil))) {
		os.Remove(file)
		return "", fmt.Errorf("checksum mismatch for %s. Nothing was installed", a.Name)
	}
	return file, nil
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
