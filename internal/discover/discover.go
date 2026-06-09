// Package discover enumerates the per-version IDE config directories and locates
// their recent-project files across ALL versions of ALL classic IDEs. Reading
// every version directory (not just the newest) is the core fix over the prior
// workflow, which only ever read the highest-numbered version per product.
package discover

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
)

// RecentFile is one located recentProjects.xml (or Rider recentSolutions.xml).
type RecentFile struct {
	Path        string // absolute path to the XML file
	Vendor      string // "JetBrains" or "Google"
	DataDir     string // the version config dir name, e.g. "IntelliJIdea2026.1" (== dataDirectoryName)
	Product     string // the product token, e.g. "IntelliJIdea"
	Version     string // the version tail, e.g. "2026.1" (may be empty)
	IsSolutions bool   // true for Rider's recentSolutions.xml
	ModTime     time.Time
}

// ShipFile is one located Fleet/Air recent_ships.*.json workspace store.
type ShipFile struct {
	Path          string // absolute path to the JSON file
	Product       string // "Fleet" or "Air"
	Code          string // synthesised production code, "FL" or "AIR"
	LocalShipType string // shipType that denotes a local workspace ("LOCAL"/"AIR")
	DataDir       string // config dir name (== dataDirectoryName), "Fleet"/"Air"
	ModTime       time.Time
}

// shipStores describes the JSON-store IDEs scanned by FindShips. They live in
// unversioned config dirs directly under the JetBrains root.
var shipStores = []struct{ Dir, Product, Code, LocalShipType string }{
	{"Fleet", "Fleet", "FL", "LOCAL"},
	{"Air", "Air", "AIR", "AIR"},
}

// productRe splits a config dir name into a product token and an optional
// version tail. The product token is non-greedy so e.g. "IntelliJIdea2026.2"
// splits into ("IntelliJIdea", "2026.2"). The real filter is the file-existence
// probe below; this regex only extracts metadata for display and version sort.
var productRe = regexp.MustCompile(`^([A-Za-z]+?)(\d{4}\.\d+(?:\.\d+)*.*)?$`)

// Find scans every config root one level deep and returns each recent-project
// file present. Backup directories and non-IDE directories fall away naturally.
func Find(cfg config.Config) []RecentFile {
	var files []RecentFile

	for _, root := range cfg.ConfigRoots {
		entries, err := os.ReadDir(root.Dir)
		if err != nil {
			continue // root absent (e.g. no Google dir) — skip silently
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, "-backup") {
				continue // backup trees nest their file deeper anyway
			}
			if name == "Fleet" || name == "Air" || strings.HasPrefix(name, "FleetBackend") {
				continue // JSON-store IDEs handled by FindShips, not the XML scan
			}
			// The Google root holds non-IDE dirs too; only Android Studio there.
			if root.Vendor == "Google" && !strings.HasPrefix(name, "AndroidStudio") {
				continue
			}

			product, version := splitProductVersion(name)
			optionsDir := filepath.Join(root.Dir, name, "options")

			candidates := []struct {
				file        string
				isSolutions bool
			}{
				{"recentProjects.xml", false},
			}
			if root.Vendor == "JetBrains" {
				// Rider records solutions separately; probe defensively.
				candidates = append(candidates, struct {
					file        string
					isSolutions bool
				}{"recentSolutions.xml", true})
			}

			for _, c := range candidates {
				p := filepath.Join(optionsDir, c.file)
				info, err := os.Stat(p)
				if err != nil || info.IsDir() {
					continue
				}
				files = append(files, RecentFile{
					Path:        p,
					Vendor:      root.Vendor,
					DataDir:     name,
					Product:     product,
					Version:     version,
					IsSolutions: c.isSolutions,
					ModTime:     info.ModTime(),
				})
			}
		}
	}

	return files
}

// FindShips locates the Fleet/Air recent_ships.*.json workspace stores. Each
// store lives in an unversioned dir directly under a JetBrains config root; when
// several recent_ships.<n>.json files exist, the most recently modified wins.
func FindShips(cfg config.Config) []ShipFile {
	var out []ShipFile
	for _, root := range cfg.ConfigRoots {
		if root.Vendor != "JetBrains" {
			continue
		}
		for _, s := range shipStores {
			glob := filepath.Join(root.Dir, s.Dir, "localStorage", "recent_ships.*.json")
			matches, _ := filepath.Glob(glob)
			var best string
			var bestMod time.Time
			for _, m := range matches {
				info, err := os.Stat(m)
				if err != nil || info.IsDir() {
					continue
				}
				if best == "" || info.ModTime().After(bestMod) {
					best, bestMod = m, info.ModTime()
				}
			}
			if best == "" {
				continue
			}
			out = append(out, ShipFile{
				Path:          best,
				Product:       s.Product,
				Code:          s.Code,
				LocalShipType: s.LocalShipType,
				DataDir:       s.Dir,
				ModTime:       bestMod,
			})
		}
	}
	return out
}

// FindProjectDirs scans each project root one level deep and returns every
// immediate subdirectory (cleaned absolute path). Files and dot-directories are
// skipped; a missing root is skipped silently, mirroring Find's tolerance. These
// are candidate "un-opened" projects surfaced by the `+` keyword variant;
// stub/worktree/ignore filtering happens downstream, not here. The roots are the
// effective ones (configured JB_PROJECT_ROOTS, or the auto-detected defaults).
func FindProjectDirs(roots []string) []string {
	var dirs []string
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue // root absent or unreadable — skip silently
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dirs = append(dirs, filepath.Clean(filepath.Join(root, e.Name())))
		}
	}
	return dirs
}

// WorktreesOf returns the working-tree paths of every *linked* git worktree of
// the repository at repoDir, excluding repoDir's own (main) worktree. It returns
// nil when repoDir is not a repo, has no linked worktrees, or git can't be run.
// A repo only gains linked worktrees once it has a ".git/worktrees" registry dir
// (created by `git worktree add`), so the common no-worktree case is rejected by
// a single stat without spawning git. Enumerating via git is necessary because a
// linked worktree's working dir lives wherever its gitdir pointer says — commonly
// a dot-dir like ".worktrees/<branch>" inside the repo — which neither the
// one-level root scan (FindProjectDirs) nor a shallow walk would ever reach.
// These are candidate worktrees surfaced by the `~` keyword variant; existence,
// stub, and dedup filtering happen downstream.
func WorktreesOf(repoDir string) []string {
	if info, err := os.Stat(filepath.Join(repoDir, ".git", "worktrees")); err != nil || !info.IsDir() {
		return nil // no linked worktrees registered — skip the git call
	}
	out, err := exec.Command("git", "-C", repoDir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	// git reports paths with symlinks resolved (e.g. /var -> /private/var on
	// macOS), so resolve repoDir the same way before comparing to exclude the main
	// worktree reliably.
	main := resolvePath(repoDir)
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		p, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue // porcelain has HEAD/branch/bare lines too — only "worktree <path>" matters
		}
		if p = strings.TrimSpace(p); p != "" && resolvePath(p) != main {
			paths = append(paths, filepath.Clean(p)) // skip the main worktree; keep the linked ones
		}
	}
	return paths
}

// resolvePath returns the symlink-resolved, cleaned form of p, falling back to a
// plain Clean when the path can't be resolved (e.g. it no longer exists).
func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func splitProductVersion(name string) (product, version string) {
	m := productRe.FindStringSubmatch(name)
	if m == nil {
		return name, ""
	}
	return m[1], m[2]
}
