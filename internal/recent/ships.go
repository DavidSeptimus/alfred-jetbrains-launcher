package recent

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/discover"
)

// Fleet and Air don't use recentProjects.xml. They store recent workspaces
// ("ships") in localStorage/recent_ships.*.json. The real project paths live in
// shipState.locations (a JSON-encoded string array, parallel to namedRoots);
// most ships are throwaway --scratch sessions with empty locations, and remote
// ships (Air's AIR_REMOTE / WORKSPACE_SERVER) point into agent sandbox caches —
// both are skipped by requiring the store's local shipType and a non-empty path.

type shipsDoc struct {
	Ships map[string]shipRecord `json:"ships"`
}

type shipRecord struct {
	ShipType  string    `json:"shipType"`
	ShipState shipState `json:"shipState"`
}

type shipState struct {
	Locations      string `json:"locations"`      // JSON-encoded "[\"/path\", …]"
	LastLoadedTime string `json:"lastLoadedTime"` // epoch-ms, as a string
	CreationTime   string `json:"creationTime"`
}

// ParseShips reads one Fleet/Air recent_ships file and returns an entry per
// real project path. A malformed file yields a warning and zero entries.
func ParseShips(home string, f discover.ShipFile) []RawEntry {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jb: cannot read %s: %v\n", f.Path, err)
		return nil
	}
	var doc shipsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		fmt.Fprintf(os.Stderr, "jb: malformed ship store %s: %v\n", f.Path, err)
		return nil
	}

	var entries []RawEntry
	for _, s := range doc.Ships {
		if s.ShipType != f.LocalShipType {
			continue // skip remote / server ships (agent sandboxes, etc.)
		}
		ts := shipTimestamp(s.ShipState, f.ModTime)
		for _, loc := range decodeStringArray(s.ShipState.Locations) {
			if loc == "" {
				continue
			}
			entries = append(entries, RawEntry{
				Ref:            ClassifyPath(home, loc),
				ProductionCode: f.Code,
				SourceDataDir:  f.DataDir,
				Timestamp:      ts,
			})
		}
	}
	return entries
}

// decodeStringArray parses a JSON-encoded string holding an array of strings,
// e.g. `["/Users/dave/p"]`. Returns nil on empty/invalid input.
func decodeStringArray(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// shipTimestamp prefers lastLoadedTime (then creationTime), falling back to the
// file's mtime, mirroring bestTimestamp for the XML path.
func shipTimestamp(st shipState, fallback time.Time) time.Time {
	for _, v := range []string{st.LastLoadedTime, st.CreationTime} {
		if v == "" {
			continue
		}
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms > 0 {
			return time.UnixMilli(ms)
		}
	}
	return fallback
}
