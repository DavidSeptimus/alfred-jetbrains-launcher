package state

import (
	"testing"
	"time"
)

func TestLaunchedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 6, 7, 15, 0, 0, 0, time.UTC)

	s := Load(dir)
	s.SetLaunched("/p/proj", "IU", "IntelliJIdea2026.1", at)
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}

	// The launched IDE (and its time) persists across save/load.
	s2 := Load(dir)
	li, ok := s2.LaunchedFor("/p/proj")
	if !ok || li.Code != "IU" || li.DataDir != "IntelliJIdea2026.1" || !li.At.Equal(at) {
		t.Fatalf("launched not persisted: %+v ok=%v", li, ok)
	}

	// Re-launching with a different IDE overwrites the association.
	s2.SetLaunched("/p/proj", "GO", "GoLand2026.1", at.Add(time.Hour))
	if li, _ := s2.LaunchedFor("/p/proj"); li.Code != "GO" || li.DataDir != "GoLand2026.1" {
		t.Fatalf("launched not overwritten: %+v", li)
	}

	// An empty association is a no-op (no stray entry).
	s2.SetLaunched("/x", "", "", at)
	if _, ok := s2.LaunchedFor("/x"); ok {
		t.Error("empty SetLaunched should be a no-op")
	}
}

// Older state files (no launched key) load without error and report nothing.
func TestLaunchedAbsentIsFine(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, State{Pinned: []string{"/a"}}); err != nil {
		t.Fatal(err)
	}
	s := Load(dir)
	if _, ok := s.LaunchedFor("/a"); ok {
		t.Error("expected no launched entry")
	}
}
