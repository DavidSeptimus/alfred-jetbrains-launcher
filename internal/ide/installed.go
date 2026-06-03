package ide

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
)

// Installed is a JetBrains IDE found on disk.
type Installed struct {
	Code    string // production code from product-info.json
	Family  string
	Version string // e.g. "2026.1.2"
	Display string
	AppPath string // absolute path to the .app bundle (used to launch)
	DataDir string // dataDirectoryName, e.g. "IntelliJIdea2026.1"
}

// productInfo is the subset of <App>/Contents/Resources/product-info.json we use.
type productInfo struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	ProductCode       string `json:"productCode"`
	DataDirectoryName string `json:"dataDirectoryName"`
}

// toolboxAppRe extracts the .app bundle path a Toolbox launcher script points at,
// from its trailing `open -na "<...>.app/Contents/MacOS/<exe>" ...` line.
var toolboxAppRe = regexp.MustCompile(`open -na "(.+?\.app)/Contents/`)

// Detect returns every installed IDE we recognise, discovered from .app bundles
// in the configured app roots plus Toolbox launcher scripts (which may point at
// bundles outside those roots).
func Detect(cfg config.Config) []Installed {
	byKey := map[string]Installed{} // key: AppPath

	add := func(appPath string) {
		if appPath == "" {
			return
		}
		if _, ok := byKey[appPath]; ok {
			return
		}
		pi, ok := readProductInfo(appPath)
		if !ok {
			// Fleet ships no product-info.json; recognise it by its app bundle so
			// Fleet workspaces still resolve to a launcher.
			if in, ok := fleetFallback(appPath); ok {
				byKey[appPath] = in
			}
			return
		}
		fam := FamilyOf(pi.ProductCode)
		if fam == "" {
			return // not a classic IDE we support in v1 (e.g. GW)
		}
		display := pi.Name
		if p, ok := ProductByCode(pi.ProductCode); ok {
			display = p.Display
		}
		byKey[appPath] = Installed{
			Code:    pi.ProductCode,
			Family:  fam,
			Version: pi.Version,
			Display: display,
			AppPath: appPath,
			DataDir: pi.DataDirectoryName,
		}
	}

	for _, root := range cfg.AppRoots {
		matches, _ := filepath.Glob(filepath.Join(root, "*.app"))
		for _, m := range matches {
			add(m)
		}
	}

	for _, dir := range cfg.ToolboxDirs {
		for _, app := range toolboxApps(dir) {
			add(app)
		}
	}

	out := make([]Installed, 0, len(byKey))
	for _, v := range byKey {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return cmpVersion(out[i].Version, out[j].Version) > 0 // newest first within a code
	})
	return out
}

// fleetFallback recognises a Fleet.app bundle — which ships no
// product-info.json — by its bundle identifier in Contents/Info.plist (the
// literal id is present whether the plist is XML or binary), so Fleet
// workspaces resolve to a launcher. Version is left empty: there is only one
// Fleet, so version-ranking never matters.
func fleetFallback(appPath string) (Installed, bool) {
	data, err := os.ReadFile(filepath.Join(appPath, "Contents", "Info.plist"))
	if err != nil || !bytes.Contains(data, []byte("com.jetbrains.fleet")) {
		return Installed{}, false
	}
	p, _ := ProductByCode("FL")
	return Installed{
		Code:    "FL",
		Family:  "fleet",
		Display: p.Display,
		AppPath: appPath,
		DataDir: "Fleet",
	}, true
}

func readProductInfo(appPath string) (productInfo, bool) {
	data, err := os.ReadFile(filepath.Join(appPath, "Contents", "Resources", "product-info.json"))
	if err != nil {
		return productInfo{}, false
	}
	var pi productInfo
	if err := json.Unmarshal(data, &pi); err != nil || pi.ProductCode == "" {
		return productInfo{}, false
	}
	return pi, true
}

func toolboxApps(scriptsDir string) []string {
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return nil
	}
	var apps []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(scriptsDir, e.Name()))
		if err != nil {
			continue
		}
		if m := toolboxAppRe.FindSubmatch(data); m != nil {
			apps = append(apps, string(m[1]))
		}
	}
	return apps
}

// NewestByCode returns the newest installed IDE with the given production code.
func NewestByCode(installed []Installed, code string) (Installed, bool) {
	return newestMatch(installed, func(i Installed) bool { return i.Code == code })
}

// NewestByFamily returns the newest installed IDE in the given family.
func NewestByFamily(installed []Installed, family string) (Installed, bool) {
	return newestMatch(installed, func(i Installed) bool { return i.Family == family })
}

func newestMatch(installed []Installed, pred func(Installed) bool) (Installed, bool) {
	var best Installed
	found := false
	for _, i := range installed {
		if !pred(i) {
			continue
		}
		if !found || cmpVersion(i.Version, best.Version) > 0 {
			best, found = i, true
		}
	}
	return best, found
}

// cmpVersion compares two JetBrains version strings numerically.
// It tolerates Android Studio's "AI-261.x..." form by stripping a leading
// non-numeric, dash-terminated prefix before parsing dotted integer fields.
func cmpVersion(a, b string) int {
	av := versionFields(a)
	bv := versionFields(b)
	for i := 0; i < len(av) || i < len(bv); i++ {
		var x, y int
		if i < len(av) {
			x = av[i]
		}
		if i < len(bv) {
			y = bv[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionFields(v string) []int {
	if i := strings.Index(v, "-"); i >= 0 {
		// e.g. "AI-261.23567..." -> "261.23567..."
		if _, err := strconv.Atoi(strings.SplitN(v, ".", 2)[0]); err != nil {
			v = v[i+1:]
		}
	}
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == '.' })
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}
