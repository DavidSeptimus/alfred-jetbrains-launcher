package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/state"
)

// A workflow launch re-stamps a project's IDE association as a loose override:
// it wins while no merged recents/ship entry is newer than the launch, and
// yields once one is.
func TestApplyLaunchOverrides(t *testing.T) {
	cfg := config.Config{DataDir: filepath.Join(t.TempDir(), "data")}
	launchAt := time.Date(2026, 6, 7, 15, 0, 0, 0, time.UTC)
	older := launchAt.Add(-time.Hour) // recents entry predates the launch
	newer := launchAt.Add(time.Hour)  // recents entry postdates the launch

	st := state.Load(cfg.DataDir)
	st.SetLaunched("/p/launch-wins", "IU", "IntelliJIdea2026.1", launchAt)
	st.SetLaunched("/p/recents-wins", "IU", "IntelliJIdea2026.1", launchAt)
	if err := state.Save(cfg.DataDir, st); err != nil {
		t.Fatal(err)
	}

	projects := []recent.Project{
		// Launch is newer than the merged (Air) entry -> override applies.
		{Path: "/p/launch-wins", ProductionCode: "AIR", SourceDataDir: "Air", Timestamp: older, AllCodes: []string{"AIR"}},
		// A genuine entry is newer than the launch -> override yields to it.
		{Path: "/p/recents-wins", ProductionCode: "AIR", SourceDataDir: "Air", Timestamp: newer, AllCodes: []string{"AIR"}},
		// No launch override at all -> untouched.
		{Path: "/p/untouched", ProductionCode: "GO", SourceDataDir: "GoLand2026.1", AllCodes: []string{"GO"}},
	}
	got := applyLaunchOverrides(cfg, projects)

	// launch-wins flips to the launched IDE; AllCodes keeps it matchable under the
	// IDEA keyword (front) without dropping its history.
	if p := got[0]; p.ProductionCode != "IU" || p.SourceDataDir != "IntelliJIdea2026.1" {
		t.Errorf("override should apply (launch newer): code=%q dir=%q", p.ProductionCode, p.SourceDataDir)
	}
	if p := got[0]; len(p.AllCodes) == 0 || p.AllCodes[0] != "IU" {
		t.Errorf("launched code should lead AllCodes, got %v", p.AllCodes)
	}

	// recents-wins keeps its merged association because a newer entry supersedes.
	if p := got[1]; p.ProductionCode != "AIR" || p.SourceDataDir != "Air" {
		t.Errorf("newer recents entry should win, got code=%q dir=%q", p.ProductionCode, p.SourceDataDir)
	}

	// A project with no launch override is left exactly as-is.
	if p := got[2]; p.ProductionCode != "GO" || p.SourceDataDir != "GoLand2026.1" {
		t.Errorf("non-launched project mutated: %+v", p)
	}
}

// On an exact timestamp tie the launch override applies: a merged entry must be
// strictly newer (`Timestamp.After(At)`) to supersede it.
func TestApplyLaunchOverridesTie(t *testing.T) {
	cfg := config.Config{DataDir: filepath.Join(t.TempDir(), "data")}
	at := time.Date(2026, 6, 7, 15, 0, 0, 0, time.UTC)

	st := state.Load(cfg.DataDir)
	st.SetLaunched("/p/tie", "IU", "IntelliJIdea2026.1", at)
	if err := state.Save(cfg.DataDir, st); err != nil {
		t.Fatal(err)
	}
	in := []recent.Project{{Path: "/p/tie", ProductionCode: "AIR", Timestamp: at}}
	if got := applyLaunchOverrides(cfg, in); got[0].ProductionCode != "IU" {
		t.Errorf("override should apply on an exact tie, got %q", got[0].ProductionCode)
	}
}

// A zero launch time (pre-dating the At field) is treated as always-applicable.
func TestApplyLaunchOverridesZeroTime(t *testing.T) {
	cfg := config.Config{DataDir: filepath.Join(t.TempDir(), "data")}
	st := state.Load(cfg.DataDir)
	st.Launched = map[string]state.LaunchInfo{
		"/p/a": {Code: "IU", DataDir: "IntelliJIdea2026.1"}, // no At
	}
	if err := state.Save(cfg.DataDir, st); err != nil {
		t.Fatal(err)
	}
	in := []recent.Project{{Path: "/p/a", ProductionCode: "AIR", Timestamp: time.Now()}}
	if got := applyLaunchOverrides(cfg, in); got[0].ProductionCode != "IU" {
		t.Errorf("zero-time override should apply, got %q", got[0].ProductionCode)
	}
}

// With no launch history the project list passes through untouched.
func TestApplyLaunchOverridesEmpty(t *testing.T) {
	cfg := config.Config{DataDir: filepath.Join(t.TempDir(), "data")}
	in := []recent.Project{{Path: "/p/a", ProductionCode: "AIR"}}
	got := applyLaunchOverrides(cfg, in)
	if got[0].ProductionCode != "AIR" {
		t.Errorf("empty history should not mutate, got %q", got[0].ProductionCode)
	}
}
