// Package ide knows the catalogue of JetBrains products, detects which are
// installed, and resolves which installed IDE should open a given project.
package ide

import (
	"sort"
	"strings"
)

// Product is a static definition of a JetBrains IDE product.
type Product struct {
	Code       string // production code, e.g. "IU"
	Family     string // launcher keyword / family, e.g. "idea"
	Display    string // human name
	DataPrefix string // config-dir prefix, e.g. "IntelliJIdea"
}

// products is keyed by production code. IU/IC and PY/PC are distinct products
// with distinct config prefixes — conflating them launches the wrong edition.
var products = map[string]Product{
	"IU":  {"IU", "idea", "IntelliJ IDEA", "IntelliJIdea"},
	"IC":  {"IC", "idea", "IntelliJ IDEA CE", "IdeaIC"},
	"AI":  {"AI", "studio", "Android Studio", "AndroidStudio"},
	"RR":  {"RR", "rustrover", "RustRover", "RustRover"},
	"PY":  {"PY", "pycharm", "PyCharm Professional", "PyCharm"},
	"PC":  {"PC", "pycharm", "PyCharm CE", "PyCharmCE"},
	"WS":  {"WS", "webstorm", "WebStorm", "WebStorm"},
	"GO":  {"GO", "goland", "GoLand", "GoLand"},
	"CL":  {"CL", "clion", "CLion", "CLion"},
	"RM":  {"RM", "rubymine", "RubyMine", "RubyMine"},
	"DB":  {"DB", "datagrip", "DataGrip", "DataGrip"},
	"PS":  {"PS", "phpstorm", "PhpStorm", "PhpStorm"},
	"RD":  {"RD", "rider", "Rider", "Rider"},
	"DS":  {"DS", "dataspell", "DataSpell", "DataSpell"},
	"QA":  {"QA", "aqua", "Aqua", "Aqua"},
	"WRS": {"WRS", "writerside", "Writerside", "Writerside"},
	// Fleet and Air keep recents in a JSON store (localStorage/recent_ships.*.json),
	// not recentProjects.xml; their codes are synthesised by the ship parser. Air's
	// "AIR" also matches its product-info.json so an installed Air resolves directly.
	"FL":  {"FL", "fleet", "Fleet", "Fleet"},
	"AIR": {"AIR", "air", "Air", "Air"},
}

// ProductByCode returns the product definition for a production code.
func ProductByCode(code string) (Product, bool) {
	p, ok := products[code]
	return p, ok
}

// FamilyOf returns the launcher family for a production code ("" if unknown).
func FamilyOf(code string) string {
	if p, ok := products[code]; ok {
		return p.Family
	}
	return ""
}

// ideaFirstClass lists families IntelliJ IDEA Ultimate can open first-class
// (via bundled or readily available plugins). Used as a fallback when neither
// the recorded IDE nor a same-family IDE is installed.
var ideaFirstClass = map[string]bool{
	"idea":     true,
	"webstorm": true,
	"goland":   true,
	"pycharm":  true,
	"phpstorm": true,
	"datagrip": true,
	"rubymine": true,
}

// IsIDEAFirstClass reports whether IDEA Ultimate is a reasonable host for a
// project whose family is given.
func IsIDEAFirstClass(family string) bool {
	return ideaFirstClass[family]
}

// familyDisplay gives a stable human name per family (deterministic, unlike
// iterating the products map which has two entries for some families).
var familyDisplay = map[string]string{
	"idea": "IntelliJ IDEA", "pycharm": "PyCharm", "webstorm": "WebStorm",
	"goland": "GoLand", "clion": "CLion", "rubymine": "RubyMine",
	"datagrip": "DataGrip", "phpstorm": "PhpStorm", "rider": "Rider",
	"rustrover": "RustRover", "studio": "Android Studio",
	"dataspell": "DataSpell", "aqua": "Aqua", "writerside": "Writerside",
	"fleet": "Fleet", "air": "Air",
}

// FamilyDisplay returns a human name for a launcher family.
func FamilyDisplay(family string) string {
	if d, ok := familyDisplay[family]; ok {
		return d
	}
	return family
}

// DefaultProjectDir is a conventional new-project directory plus the IDE it
// implies — so an un-opened project found under an auto-detected root can still
// resolve to the right IDE (a folder in GolandProjects implies GoLand).
type DefaultProjectDir struct {
	Name   string // directory name under $HOME, e.g. "GolandProjects"
	Family string // launcher family, e.g. "goland"
	Code   string // representative production code, e.g. "GO"
}

// familyPrimaryCode names the production code that represents a family when only
// the family is known (the IDE implied by an auto-detected project root).
// Dual-edition families name the paid edition; Resolve still falls back to
// whichever edition of the family is actually installed.
var familyPrimaryCode = map[string]string{
	"idea": "IU", "pycharm": "PY", "webstorm": "WS", "goland": "GO",
	"clion": "CL", "rubymine": "RM", "datagrip": "DB", "phpstorm": "PS",
	"rider": "RD", "rustrover": "RR", "studio": "AI", "dataspell": "DS",
	"aqua": "QA", "writerside": "WRS", "fleet": "FL", "air": "AIR",
}

// DefaultProjectDirs returns the conventional new-project directories JetBrains
// IDEs create under $HOME — "<Name>Projects" for the classic IDEs and Android
// Studio, "<Name>Workspaces" for Fleet and Air — each tagged with the IDE it
// implies. Names are title-cased from the launcher family (Android Studio uses
// its full product name, not the keyword); match them against the filesystem
// case-insensitively, so "GolandProjects" still matches an on-disk
// "GoLandProjects". Sorted by family for determinism.
func DefaultProjectDirs() []DefaultProjectDir {
	families := make([]string, 0, len(familyDisplay))
	for f := range familyDisplay {
		families = append(families, f)
	}
	sort.Strings(families)

	out := make([]DefaultProjectDir, 0, len(families))
	for _, f := range families {
		base := strings.ToUpper(f[:1]) + f[1:] // title-case the single-token family
		if f == "studio" {
			base = "AndroidStudio" // folder is AndroidStudioProjects, not StudioProjects
		}
		suffix := "Projects"
		if f == "fleet" || f == "air" {
			suffix = "Workspaces"
		}
		out = append(out, DefaultProjectDir{Name: base + suffix, Family: f, Code: familyPrimaryCode[f]})
	}
	return out
}
