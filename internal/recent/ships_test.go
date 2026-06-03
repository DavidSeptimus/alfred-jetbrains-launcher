package recent

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/discover"
)

// shipStoreJSON mirrors the real recent_ships.*.json shape: a "ships" map whose
// shipState.locations is itself a JSON-encoded string array.
const shipStoreJSON = `{"ships":{
  "a":{"shipType":"LOCAL","shipState":{"locations":"[\"/Users/x/proj-one\"]","lastLoadedTime":"1700000002000"}},
  "b":{"shipType":"LOCAL","shipState":{"locations":"[\"/Users/x/multi-a\",\"/Users/x/multi-b\"]","lastLoadedTime":"1700000001000"}},
  "scratch":{"shipType":"LOCAL","shipState":{"locations":"[]","lastLoadedTime":"1700000009000"}},
  "remote":{"shipType":"AIR_REMOTE","shipState":{"locations":"[\"/Users/x/should-not-appear\"]","lastLoadedTime":"1700000099000"}},
  "noLoaded":{"shipType":"LOCAL","shipState":{"locations":"[\"/Users/x/uses-creation\"]","creationTime":"1700000003000"}}
}}`

func writeShipStore(t *testing.T, body string) discover.ShipFile {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "recent_ships.2.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(p)
	return discover.ShipFile{
		Path: p, Product: "Fleet", Code: "FL", LocalShipType: "LOCAL",
		DataDir: "Fleet", ModTime: info.ModTime(),
	}
}

func TestParseShips(t *testing.T) {
	entries := ParseShips("/Users/x", writeShipStore(t, shipStoreJSON))

	got := map[string]RawEntry{}
	for _, e := range entries {
		got[e.Ref.Canonical] = e
	}

	// Scratch (empty locations) and remote (wrong shipType) are excluded.
	if _, ok := got["/Users/x/should-not-appear"]; ok {
		t.Error("AIR_REMOTE ship should be excluded")
	}
	want := []string{
		"/Users/x/proj-one",
		"/Users/x/multi-a", "/Users/x/multi-b", // multi-root expanded
		"/Users/x/uses-creation",
	}
	if len(entries) != len(want) {
		var paths []string
		for _, e := range entries {
			paths = append(paths, e.Ref.Canonical)
		}
		sort.Strings(paths)
		t.Fatalf("got %d entries %v, want %d", len(entries), paths, len(want))
	}
	for _, w := range want {
		e, ok := got[w]
		if !ok {
			t.Errorf("missing entry for %s", w)
			continue
		}
		if e.ProductionCode != "FL" {
			t.Errorf("%s: code = %q, want FL", w, e.ProductionCode)
		}
		if e.SourceDataDir != "Fleet" {
			t.Errorf("%s: dataDir = %q, want Fleet", w, e.SourceDataDir)
		}
	}

	// lastLoadedTime is parsed as epoch-ms.
	if ts := got["/Users/x/proj-one"].Timestamp; !ts.Equal(time.UnixMilli(1700000002000)) {
		t.Errorf("proj-one timestamp = %v, want %v", ts, time.UnixMilli(1700000002000))
	}
	// creationTime is used when lastLoadedTime is absent.
	if ts := got["/Users/x/uses-creation"].Timestamp; !ts.Equal(time.UnixMilli(1700000003000)) {
		t.Errorf("uses-creation timestamp = %v, want %v", ts, time.UnixMilli(1700000003000))
	}
}

func TestParseShipsMalformed(t *testing.T) {
	if got := ParseShips("/Users/x", writeShipStore(t, "{not json")); got != nil {
		t.Errorf("malformed store: got %v, want nil", got)
	}
}

func TestProductTokenCoversNewIDEs(t *testing.T) {
	// DataSpell/Aqua/Writerside config-dir tokens must map to their codes so
	// entries that omit productionCode still resolve correctly.
	for tok, code := range map[string]string{"DataSpell": "DS", "Aqua": "QA", "Writerside": "WRS"} {
		if got := productTokenToCode[tok]; got != code {
			t.Errorf("productTokenToCode[%q] = %q, want %q", tok, got, code)
		}
	}
}
