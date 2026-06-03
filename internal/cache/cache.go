// Package cache memoises the merged project list between Script Filter
// invocations. Alfred re-runs the binary on every keystroke, so the cache keeps
// cold-start fast while staying correct: the key includes the config-root
// mtimes (so a newly-created version dir invalidates it — preventing the very
// "only newest version" bug this workflow fixes) plus every recent file's mtime.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/discover"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
)

const cacheFile = "jb-projects-cache.json"

type payload struct {
	Fingerprint string           `json:"fingerprint"`
	Projects    []recent.Project `json:"projects"`
}

// Fingerprint builds the cache key from config-root mtimes, recent-file mtimes,
// and Fleet/Air ship-store mtimes.
func Fingerprint(cfg config.Config, files []discover.RecentFile, ships []discover.ShipFile) string {
	var parts []string
	for _, r := range cfg.ConfigRoots {
		if info, err := os.Stat(r.Dir); err == nil {
			parts = append(parts, fmt.Sprintf("root:%s=%d", r.Dir, info.ModTime().UnixNano()))
		}
	}
	for _, f := range files {
		parts = append(parts, fmt.Sprintf("file:%s=%d", f.Path, f.ModTime.UnixNano()))
	}
	for _, s := range ships {
		parts = append(parts, fmt.Sprintf("ship:%s=%d", s.Path, s.ModTime.UnixNano()))
	}
	sort.Strings(parts)
	// Stub detection depends on the content-ignore globs, so a change to them
	// must invalidate the cached project list.
	return strings.Join(parts, "\n") + "\nignore:" + strings.Join(cfg.IgnoreContent, ",")
}

func path(cfg config.Config) string {
	return filepath.Join(cfg.CacheDir, cacheFile)
}

// Load returns the cached projects when the fingerprint matches.
func Load(cfg config.Config, fingerprint string) ([]recent.Project, bool) {
	data, err := os.ReadFile(path(cfg))
	if err != nil {
		return nil, false
	}
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, false
	}
	if p.Fingerprint != fingerprint {
		return nil, false
	}
	return p.Projects, true
}

// Save writes the merged projects with their fingerprint. Errors are non-fatal.
func Save(cfg config.Config, fingerprint string, projects []recent.Project) {
	data, err := json.Marshal(payload{Fingerprint: fingerprint, Projects: projects})
	if err != nil {
		return
	}
	tmp := path(cfg) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path(cfg))
}
