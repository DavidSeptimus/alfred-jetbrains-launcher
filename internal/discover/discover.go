// Package discover enumerates the per-version IDE config directories and locates
// their recent-project files across ALL versions of ALL classic IDEs. Reading
// every version directory (not just the newest) is the core fix over the prior
// workflow, which only ever read the highest-numbered version per product.
package discover

import (
	"os"
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

func splitProductVersion(name string) (product, version string) {
	m := productRe.FindStringSubmatch(name)
	if m == nil {
		return name, ""
	}
	return m[1], m[2]
}
