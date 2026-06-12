// Package alfred renders the Script Filter JSON that Alfred consumes.
package alfred

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Icon points at an image. With Type left empty, Path is an image file
// (bundle-relative or absolute). With Type "fileicon", Path is a file/app whose
// macOS icon Alfred renders — so an installed IDE shows its own live icon
// without us bundling or extracting anything.
type Icon struct {
	Type string `json:"type,omitempty"`
	Path string `json:"path,omitempty"`
}

// Mod customises a result for a held modifier key (cmd/alt/ctrl/shift).
type Mod struct {
	Subtitle  string            `json:"subtitle,omitempty"`
	Arg       string            `json:"arg,omitempty"`
	Valid     *bool             `json:"valid,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

// Item is one Script Filter result row.
type Item struct {
	UID       string            `json:"uid,omitempty"`
	Title     string            `json:"title"`
	Subtitle  string            `json:"subtitle,omitempty"`
	Arg       string            `json:"arg,omitempty"`
	Match     string            `json:"match,omitempty"`
	Icon      *Icon             `json:"icon,omitempty"`
	Valid     *bool             `json:"valid,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	Mods      map[string]Mod    `json:"mods,omitempty"`
}

// Output is the top-level Script Filter document.
type Output struct {
	Items []Item `json:"items"`
	// Rerun asks Alfred to re-run the Script Filter automatically after this many
	// seconds (valid range 0.1–5.0). Used to poll a background job (e.g. a Gradle
	// enumeration) and refresh the list in place. Omitted when zero.
	Rerun float64 `json:"rerun,omitempty"`
}

// Render marshals items into the Script Filter JSON Alfred expects. An empty
// list still produces a valid `{"items":[]}` document.
func Render(items []Item) ([]byte, error) {
	return RenderWithRerun(items, 0)
}

// RenderWithRerun is Render with an Alfred `rerun` interval, so the Script Filter
// re-runs itself after rerun seconds (0 omits the field — no auto-rerun).
func RenderWithRerun(items []Item, rerun float64) ([]byte, error) {
	if items == nil {
		items = []Item{}
	}
	return json.Marshal(Output{Items: items, Rerun: rerun})
}

// Info builds a single non-actionable informational row (e.g. empty state or a
// surfaced error) so the user never sees a blank Alfred box.
func Info(title, subtitle string) Item {
	no := false
	return Item{Title: title, Subtitle: subtitle, Valid: &no, Icon: &Icon{Path: IconPath("")}}
}

// IconPath returns the bundle-relative icon file for a family ("" -> default).
func IconPath(family string) string {
	if family == "" {
		return "icons/default.png"
	}
	return "icons/" + family + ".png"
}

// AbbreviateHome replaces a leading home dir with "~" for compact subtitles.
func AbbreviateHome(home, path string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// BoolPtr is a helper for the optional Valid field.
func BoolPtr(b bool) *bool { return &b }
