package recent

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/discover"
)

const testHome = "/home/test"

func fixtureFile(name, product string) discover.RecentFile {
	return discover.RecentFile{
		Path:    filepath.Join("testdata", name),
		Product: product,
		DataDir: product + "2026.1",
		ModTime: time.UnixMilli(1111111111111),
	}
}

func findEntry(entries []RawEntry, canonical string) (RawEntry, bool) {
	for _, e := range entries {
		if e.Ref.Canonical == canonical {
			return e, true
		}
	}
	return RawEntry{}, false
}

func TestParseIntelliJ(t *testing.T) {
	entries := Parse(testHome, fixtureFile("intellij.xml", "IntelliJIdea"))
	if len(entries) != 4 {
		t.Fatalf("want 4 entries (alpha, beta, remote, gamma), got %d", len(entries))
	}

	alpha, ok := findEntry(entries, "/home/test/IdeaProjects/alpha")
	if !ok {
		t.Fatal("alpha not found")
	}
	if alpha.ProductionCode != "IU" {
		t.Errorf("alpha code: want IU, got %q", alpha.ProductionCode)
	}
	if got := alpha.Timestamp.UnixMilli(); got != 1779308608268 {
		t.Errorf("alpha timestamp: want max(activation,open)=1779308608268, got %d", got)
	}
	if alpha.SourceDataDir != "IntelliJIdea2026.1" {
		t.Errorf("alpha SourceDataDir: got %q", alpha.SourceDataDir)
	}

	// beta has no productionCode -> derived from product token.
	beta, ok := findEntry(entries, "/home/test/IdeaProjects/beta")
	if !ok {
		t.Fatal("beta not found")
	}
	if beta.ProductionCode != "IU" {
		t.Errorf("beta derived code: want IU, got %q", beta.ProductionCode)
	}

	// devcontainer entry must be classified remote (merge will drop it).
	remote, ok := findEntryByRaw(entries, "/$devcontainer.ij/abc123@u~var~run~docker.sock/IdeaProjects/remote-proj")
	if !ok {
		t.Fatal("remote entry not parsed")
	}
	if remote.Ref.Kind != KindRemote {
		t.Errorf("remote entry kind: want KindRemote, got %v", remote.Ref.Kind)
	}

	// lastOpenedProject (gamma) becomes a synthetic entry with file mtime.
	gamma, ok := findEntry(entries, "/home/test/IdeaProjects/gamma")
	if !ok {
		t.Fatal("gamma (lastOpenedProject) not found")
	}
	if !gamma.FromLastOpened {
		t.Error("gamma should be FromLastOpened")
	}
	if got := gamma.Timestamp.UnixMilli(); got != 1111111111111 {
		t.Errorf("gamma timestamp should fall back to file mtime, got %d", got)
	}
}

func TestParseDropsRedundantLastOpened(t *testing.T) {
	// MyApp appears both in additionalInfo and lastOpenedProject -> one entry.
	entries := Parse(testHome, fixtureFile("androidstudio.xml", "AndroidStudio"))
	if len(entries) != 1 {
		t.Fatalf("want 1 entry (lastOpenedProject dedup), got %d", len(entries))
	}
	if entries[0].ProductionCode != "AI" {
		t.Errorf("want AI, got %q", entries[0].ProductionCode)
	}
}

func TestParseRiderComponent(t *testing.T) {
	entries := Parse(testHome, fixtureFile("rider.xml", "Rider"))
	if len(entries) != 1 {
		t.Fatalf("want 1 Rider entry, got %d", len(entries))
	}
	if entries[0].ProductionCode != "RD" {
		t.Errorf("want RD, got %q", entries[0].ProductionCode)
	}
}

func TestParseLegacyAndMalformed(t *testing.T) {
	if n := len(Parse(testHome, fixtureFile("legacy.xml", "IntelliJIdea"))); n != 0 {
		t.Errorf("legacy recentPaths schema should yield 0 entries, got %d", n)
	}
	if n := len(Parse(testHome, fixtureFile("malformed.xml", "IntelliJIdea"))); n != 0 {
		t.Errorf("malformed XML should yield 0 entries, got %d", n)
	}
	if n := len(Parse(testHome, fixtureFile("does-not-exist.xml", "IntelliJIdea"))); n != 0 {
		t.Errorf("missing file should yield 0 entries, got %d", n)
	}
}

func findEntryByRaw(entries []RawEntry, raw string) (RawEntry, bool) {
	for _, e := range entries {
		if e.Ref.Raw == raw {
			return e, true
		}
	}
	return RawEntry{}, false
}
