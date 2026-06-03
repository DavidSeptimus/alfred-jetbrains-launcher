// Package state persists the user's pinned and hidden (forgotten) projects in a
// durable JSON file under the workflow's data directory. This is deliberately
// workflow-local: "forget" hides a project from this launcher without editing
// JetBrains' own recentProjects.xml (which a running IDE would rewrite, and
// which we should not risk corrupting).
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PinInfo records the IDE association captured when a project was pinned, so a
// pin that later ages out of the recents window still resolves to the right IDE
// and shows its real icon. It is a snapshot from pin time.
type PinInfo struct {
	Code    string `json:"code,omitempty"`    // productionCode, e.g. "GO"
	DataDir string `json:"dataDir,omitempty"` // source version dir, e.g. "GoLand2026.1"
}

// State is the persisted set of pinned and hidden project paths (canonical).
// PinMeta is keyed by pinned path; absent keys (e.g. older state files) simply
// fall back to IDE-agnostic resolution.
type State struct {
	Pinned  []string           `json:"pinned"`
	Hidden  []string           `json:"hidden"`
	PinMeta map[string]PinInfo `json:"pinMeta,omitempty"`
}

func file(dataDir string) string { return filepath.Join(dataDir, "state.json") }

// Load reads the state; a missing or unreadable file yields empty state.
func Load(dataDir string) State {
	var s State
	if data, err := os.ReadFile(file(dataDir)); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

// Save atomically writes the state, creating the data dir if needed.
func Save(dataDir string, s State) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := file(dataDir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file(dataDir))
}

func (s State) IsPinned(p string) bool { return contains(s.Pinned, p) }
func (s State) IsHidden(p string) bool { return contains(s.Hidden, p) }

// TogglePinned flips a path's pinned state and reports whether it is now pinned.
func (s *State) TogglePinned(p string) bool {
	if contains(s.Pinned, p) {
		s.Pinned = remove(s.Pinned, p)
		return false
	}
	s.Pinned = append(s.Pinned, p)
	return true
}

// SetPinMeta records the IDE association for a pinned path. An empty code and
// dir is a no-op, so a pin with no known IDE doesn't create an empty entry.
func (s *State) SetPinMeta(p, code, dataDir string) {
	if code == "" && dataDir == "" {
		return
	}
	if s.PinMeta == nil {
		s.PinMeta = map[string]PinInfo{}
	}
	s.PinMeta[p] = PinInfo{Code: code, DataDir: dataDir}
}

// ClearPinMeta drops any stored IDE association for a path (e.g. on unpin).
func (s *State) ClearPinMeta(p string) { delete(s.PinMeta, p) }

// PinInfoFor returns the stored IDE association for a path, if any.
func (s State) PinInfoFor(p string) (PinInfo, bool) {
	pi, ok := s.PinMeta[p]
	return pi, ok
}

// Hide adds a path to the hidden set (no-op if already hidden).
func (s *State) Hide(p string) {
	if !contains(s.Hidden, p) {
		s.Hidden = append(s.Hidden, p)
	}
}

// ClearHidden empties the hidden set.
func (s *State) ClearHidden() { s.Hidden = nil }

func contains(list []string, p string) bool {
	for _, v := range list {
		if v == p {
			return true
		}
	}
	return false
}

func remove(list []string, p string) []string {
	out := list[:0]
	for _, v := range list {
		if v != p {
			out = append(out, v)
		}
	}
	return out
}
