// Package ide knows the catalogue of JetBrains products, detects which are
// installed, and resolves which installed IDE should open a given project.
package ide

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
