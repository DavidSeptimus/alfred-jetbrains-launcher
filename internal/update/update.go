// Package update checks GitHub Releases for a newer version of the workflow and
// downloads the packaged .alfredworkflow. Downloading via Go's HTTP client (not
// a browser) means the file carries no com.apple.quarantine xattr, so importing
// it does not trip Gatekeeper — the free path to seamless updates.
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repo coordinates the releases this workflow updates from.
const (
	Owner = "davidseptimus"
	Repo  = "alfred-jetbrains-launcher"
)

// Release is the subset of the GitHub release payload we use.
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func get(url string, timeout time.Duration) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", Repo)
	return (&http.Client{Timeout: timeout}).Do(req)
}

// Latest fetches the most recent published release.
func Latest() (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", Owner, Repo)
	resp, err := get(url, 15*time.Second)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// WorkflowAsset returns the .alfredworkflow asset's download URL, if present.
func (r *Release) WorkflowAsset() (string, bool) {
	for _, a := range r.Assets {
		if strings.HasSuffix(a.Name, ".alfredworkflow") {
			return a.BrowserDownloadURL, true
		}
	}
	return "", false
}

// Download saves url to a temp .alfredworkflow file and returns its path.
func Download(url string) (string, error) {
	resp, err := get(url, 60*time.Second)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %s", resp.Status)
	}
	f, err := os.CreateTemp("", "jb-update-*.alfredworkflow")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// Cache is the locally-stored result of the last release check, so `jb search`
// can show an "update available" banner without a network call on every run.
type Cache struct {
	CheckedAt int64  `json:"checkedAt"` // unix ms of the last check attempt
	LatestTag string `json:"latestTag"`
	AssetURL  string `json:"assetURL"`
	HTMLURL   string `json:"htmlURL"`
}

func cachePath(dataDir string) string { return filepath.Join(dataDir, "update-cache.json") }

// LoadCache reads the cached check result (empty if absent/unreadable).
func LoadCache(dataDir string) Cache {
	var c Cache
	if data, err := os.ReadFile(cachePath(dataDir)); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

func saveCache(dataDir string, c Cache) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	tmp := cachePath(dataDir) + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, cachePath(dataDir))
	}
}

// Stale reports whether the last check was longer than maxAge ago.
func (c Cache) Stale(maxAge time.Duration) bool {
	return time.Since(time.UnixMilli(c.CheckedAt)) > maxAge
}

// TouchChecked records "checked just now" without changing the result, used to
// debounce background refreshes (so a per-keystroke search spawns at most one).
func TouchChecked(dataDir string) {
	c := LoadCache(dataDir)
	c.CheckedAt = time.Now().UnixMilli()
	saveCache(dataDir, c)
}

// RefreshCache fetches the latest release and records it (and the check time,
// even on failure, so we don't retry constantly).
func RefreshCache(dataDir string) {
	c := LoadCache(dataDir)
	c.CheckedAt = time.Now().UnixMilli()
	if rel, err := Latest(); err == nil {
		c.LatestTag = rel.TagName
		c.HTMLURL = rel.HTMLURL
		if url, ok := rel.WorkflowAsset(); ok {
			c.AssetURL = url
		}
	}
	saveCache(dataDir, c)
}

// IsNewer reports whether release tag is a newer semver than current.
func IsNewer(tag, current string) bool {
	return cmpSemver(tag, current) > 0
}

func cmpSemver(a, b string) int {
	af, aPre := semverParse(a)
	bf, bPre := semverParse(b)
	for i := 0; i < len(af) || i < len(bf); i++ {
		var x, y int
		if i < len(af) {
			x = af[i]
		}
		if i < len(bf) {
			y = bf[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	// Equal numeric versions: a release outranks its pre-release (1.0.0 > 1.0.0-rc).
	if aPre == bPre {
		return 0
	}
	if aPre {
		return -1
	}
	return 1
}

// semverParse extracts numeric MAJOR.MINOR.PATCH fields and whether the version
// carries a pre-release suffix. A leading "v" and any "+build" metadata are
// ignored; an unparseable version yields no fields (treated as oldest).
func semverParse(v string) (fields []int, prerelease bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		prerelease = true
		v = v[:i]
	}
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		fields = append(fields, n)
	}
	return fields, prerelease
}
