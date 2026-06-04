package recent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// touch creates dir and stamps its mtime so AppendUnopened's recency interleave
// is deterministic.
func touchDir(t *testing.T, p string, ts time.Time) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	// Give it a visible file so it isn't a stub.
	if err := os.WriteFile(filepath.Join(p, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, ts, ts); err != nil {
		t.Fatal(err)
	}
}

func TestAppendUnopenedMarksAndInterleaves(t *testing.T) {
	dir := t.TempDir()

	// An already-opened recent project (kept, not re-flagged).
	opened := filepath.Join(dir, "opened")
	if err := mkdir(opened); err != nil {
		t.Fatal(err)
	}
	recents := Merge([]RawEntry{entry(opened, "IU", "IntelliJIdea2026.1", 300)}, nil)

	// Two scanned dirs: one newer than the recent, one older.
	fresh := filepath.Join(dir, "fresh")
	stale := filepath.Join(dir, "stale")
	touchDir(t, fresh, time.UnixMilli(500))
	touchDir(t, stale, time.UnixMilli(100))

	got := AppendUnopened(recents, []ScanDir{
		{Path: fresh, Code: "GO"}, // implied IDE from its root
		{Path: stale},
		{Path: opened, Code: "GO"},
	}, nil)
	if len(got) != 3 {
		t.Fatalf("want 3 projects (opened deduped), got %d", len(got))
	}

	byPath := map[string]Project{}
	for _, p := range got {
		byPath[p.Path] = p
	}
	if byPath[opened].Unopened {
		t.Error("an already-opened recent must not be flagged Unopened")
	}
	if byPath[opened].ProductionCode != "IU" {
		t.Errorf("opened project should keep its real IDE (not the scan's code), got %q", byPath[opened].ProductionCode)
	}
	if !byPath[fresh].Unopened || !byPath[stale].Unopened {
		t.Error("scanned dirs should be flagged Unopened")
	}
	if byPath[fresh].ProductionCode != "GO" {
		t.Errorf("scanned dir should take its root's implied code, got %q", byPath[fresh].ProductionCode)
	}
	if byPath[stale].ProductionCode != "" {
		t.Errorf("a codeless root should leave ProductionCode empty, got %q", byPath[stale].ProductionCode)
	}

	// Interleave by mtime: fresh (500) > opened (300) > stale (100).
	order := []string{got[0].Path, got[1].Path, got[2].Path}
	want := []string{fresh, opened, stale}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("recency interleave wrong: got %v, want %v", order, want)
		}
	}
}

func TestAppendUnopenedSkipsMissingAndDedupesRoots(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	touchDir(t, real, time.UnixMilli(200))
	missing := filepath.Join(dir, "ghost") // never created

	// real listed twice (e.g. a root configured twice) -> one entry.
	got := AppendUnopened(nil, []ScanDir{{Path: real}, {Path: missing}, {Path: real}}, nil)
	if len(got) != 1 {
		t.Fatalf("want 1 project (missing skipped, dup collapsed), got %d", len(got))
	}
	if got[0].Path != real || !got[0].Unopened {
		t.Errorf("unexpected project: %+v", got[0])
	}
}

func TestAppendUnopenedNoDirs(t *testing.T) {
	in := Merge([]RawEntry{entry("/x", "IU", "y", 1)}, nil)
	out := AppendUnopened(in, nil, nil)
	if len(out) != len(in) {
		t.Fatalf("no dirs should be a no-op: got %d, want %d", len(out), len(in))
	}
}
