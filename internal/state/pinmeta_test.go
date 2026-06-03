package state

import "testing"

func TestPinMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()

	s := Load(dir)
	if now := s.TogglePinned("/p/proj"); !now || !s.IsPinned("/p/proj") {
		t.Fatalf("TogglePinned: now=%v pinned=%v", now, s.IsPinned("/p/proj"))
	}
	s.SetPinMeta("/p/proj", "GO", "GoLand2026.1")
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}

	// PinMeta persists across save/load.
	s2 := Load(dir)
	pi, ok := s2.PinInfoFor("/p/proj")
	if !ok || pi.Code != "GO" || pi.DataDir != "GoLand2026.1" {
		t.Fatalf("pin meta not persisted: %+v ok=%v", pi, ok)
	}

	// Unpin + ClearPinMeta drops the association.
	s2.TogglePinned("/p/proj")
	s2.ClearPinMeta("/p/proj")
	if _, ok := s2.PinInfoFor("/p/proj"); ok {
		t.Error("pin meta not cleared on unpin")
	}

	// An empty association is a no-op (no stray entry).
	s2.SetPinMeta("/x", "", "")
	if _, ok := s2.PinInfoFor("/x"); ok {
		t.Error("empty SetPinMeta should be a no-op")
	}
}

// Older state files (no pinMeta key) load without error and report no meta.
func TestPinMetaAbsentIsFine(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, State{Pinned: []string{"/a"}}); err != nil {
		t.Fatal(err)
	}
	s := Load(dir)
	if !s.IsPinned("/a") {
		t.Error("pinned path lost")
	}
	if _, ok := s.PinInfoFor("/a"); ok {
		t.Error("expected no pin meta")
	}
}
