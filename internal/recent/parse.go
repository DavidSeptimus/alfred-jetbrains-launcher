package recent

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/discover"
)

// RawEntry is one recent-project record extracted from a single XML file,
// before cross-file merging.
type RawEntry struct {
	Ref            PathRef
	ProductionCode string
	FrameTitle     string
	SourceDataDir  string // the version config dir this entry came from
	Timestamp      time.Time
	FromLastOpened bool // synthesised from <option name="lastOpenedProject">
}

// --- XML schema (recentProjects.xml / recentSolutions.xml) ---

type xmlApplication struct {
	XMLName    xml.Name       `xml:"application"`
	Components []xmlComponent `xml:"component"`
}

type xmlComponent struct {
	Name    string      `xml:"name,attr"`
	Options []xmlOption `xml:"option"`
}

type xmlOption struct {
	Name  string  `xml:"name,attr"`
	Value string  `xml:"value,attr"`
	Map   *xmlMap `xml:"map"`
}

type xmlMap struct {
	Entries []xmlEntry `xml:"entry"`
}

type xmlEntry struct {
	Key  string  `xml:"key,attr"`
	Meta xmlMeta `xml:"value>RecentProjectMetaInfo"`
}

type xmlMeta struct {
	FrameTitle string       `xml:"frameTitle,attr"`
	Opened     string       `xml:"opened,attr"`
	Options    []xmlMetaOpt `xml:"option"`
}

type xmlMetaOpt struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// productTokenToCode maps a config-dir product token to a production code, used
// when an entry has no explicit productionCode (e.g. lastOpenedProject, or some
// entries that omit it).
var productTokenToCode = map[string]string{
	"IntelliJIdea":  "IU",
	"IdeaIC":        "IC",
	"PyCharm":       "PY",
	"PyCharmCE":     "PC",
	"WebStorm":      "WS",
	"WebIde":        "WS",
	"GoLand":        "GO",
	"CLion":         "CL",
	"RubyMine":      "RM",
	"DataGrip":      "DB",
	"PhpStorm":      "PS",
	"Rider":         "RD",
	"RustRover":     "RR",
	"AndroidStudio": "AI",
	"DataSpell":     "DS",
	"Aqua":          "QA",
	"Writerside":    "WRS",
}

// Parse reads one recent file and returns its entries. A malformed file, or one
// using an unrecognised/legacy schema, yields a warning and zero entries rather
// than aborting the whole run.
func Parse(home string, f discover.RecentFile) []RawEntry {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jb: cannot read %s: %v\n", f.Path, err)
		return nil
	}

	var app xmlApplication
	if err := xml.Unmarshal(data, &app); err != nil {
		fmt.Fprintf(os.Stderr, "jb: malformed XML %s: %v\n", f.Path, err)
		return nil
	}

	comp := selectComponent(app.Components)
	if comp == nil {
		// No recognised component (possibly legacy recentPaths/<list> schema).
		fmt.Fprintf(os.Stderr, "jb: no RecentProjectsManager in %s (unsupported schema)\n", f.Path)
		return nil
	}

	fallbackCode := productTokenToCode[f.Product]
	var entries []RawEntry
	seen := map[string]bool{}

	for _, opt := range comp.Options {
		switch {
		case opt.Name == "additionalInfo" && opt.Map != nil:
			for _, e := range opt.Map.Entries {
				ref := ClassifyPath(home, e.Key)
				code := metaValue(e.Meta.Options, "productionCode")
				if code == "" {
					code = fallbackCode
				}
				entries = append(entries, RawEntry{
					Ref:            ref,
					ProductionCode: code,
					FrameTitle:     e.Meta.FrameTitle,
					SourceDataDir:  f.DataDir,
					Timestamp:      bestTimestamp(e.Meta.Options, f.ModTime),
				})
				if ref.Canonical != "" {
					seen[ref.Canonical] = true
				}
			}
		case opt.Name == "lastOpenedProject" && opt.Value != "":
			// Defer: only add if not already present as a full entry (checked
			// after the loop, since additionalInfo may appear after this option).
			entries = append(entries, RawEntry{
				Ref:            ClassifyPath(home, opt.Value),
				ProductionCode: fallbackCode,
				SourceDataDir:  f.DataDir,
				Timestamp:      f.ModTime,
				FromLastOpened: true,
			})
		}
	}

	// Drop lastOpenedProject synthetics whose path already has a real entry.
	out := entries[:0]
	for _, e := range entries {
		if e.FromLastOpened && e.Ref.Canonical != "" && seen[e.Ref.Canonical] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// selectComponent finds the recent-projects component leniently: by known name,
// else any component that carries an additionalInfo map.
func selectComponent(comps []xmlComponent) *xmlComponent {
	for i := range comps {
		switch comps[i].Name {
		case "RecentProjectsManager", "RiderRecentProjectsManager":
			return &comps[i]
		}
	}
	for i := range comps {
		for _, o := range comps[i].Options {
			if o.Name == "additionalInfo" && o.Map != nil {
				return &comps[i]
			}
		}
	}
	return nil
}

func metaValue(opts []xmlMetaOpt, name string) string {
	for _, o := range opts {
		if o.Name == name {
			return o.Value
		}
	}
	return ""
}

// bestTimestamp returns the most recent of activationTimestamp /
// projectOpenTimestamp (epoch-ms), falling back to the file's mtime.
func bestTimestamp(opts []xmlMetaOpt, fallback time.Time) time.Time {
	var best int64
	for _, name := range []string{"activationTimestamp", "projectOpenTimestamp"} {
		if v := metaValue(opts, name); v != "" {
			if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms > best {
				best = ms
			}
		}
	}
	if best == 0 {
		return fallback
	}
	return time.UnixMilli(best)
}
