package state

import "testing"

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()

	if s := Load(dir); s.IsPinned("/a") || s.IsHidden("/a") {
		t.Fatal("a fresh state should be empty")
	}

	s := Load(dir)
	if !s.TogglePinned("/a") {
		t.Error("toggling an unpinned path should pin it")
	}
	s.Hide("/b")
	s.Hide("/b") // idempotent
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}

	reloaded := Load(dir)
	if !reloaded.IsPinned("/a") {
		t.Error("pin should persist")
	}
	if !reloaded.IsHidden("/b") {
		t.Error("hide should persist")
	}
	if len(reloaded.Hidden) != 1 {
		t.Errorf("hide should be idempotent, got %v", reloaded.Hidden)
	}
	if reloaded.TogglePinned("/a") {
		t.Error("toggling a pinned path should unpin it")
	}
	if reloaded.IsPinned("/a") {
		t.Error("path should be unpinned after second toggle")
	}
	reloaded.ClearHidden()
	if reloaded.IsHidden("/b") {
		t.Error("ClearHidden should empty the hidden set")
	}
}
