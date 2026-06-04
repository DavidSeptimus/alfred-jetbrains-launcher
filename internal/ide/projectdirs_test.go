package ide

import "testing"

func TestDefaultProjectDirs(t *testing.T) {
	byName := map[string]DefaultProjectDir{}
	for _, d := range DefaultProjectDirs() {
		byName[d.Name] = d
	}

	// Spot-check the cases that don't fall out of a naive title-case, and that
	// each carries the production code of the IDE it implies:
	//  - IntelliJ uses "Idea" (code IU), not "IntelliJ"
	//  - Android Studio uses its full product name (code AI), not the keyword
	//  - Fleet/Air use the "Workspaces" suffix
	want := map[string]string{
		"IdeaProjects":          "IU",
		"AndroidStudioProjects": "AI",
		"GolandProjects":        "GO",
		"PycharmProjects":       "PY",
		"FleetWorkspaces":       "FL",
		"AirWorkspaces":         "AIR",
	}
	for name, code := range want {
		d, ok := byName[name]
		if !ok {
			t.Errorf("expected %q among default dirs, got %v", name, DefaultProjectDirs())
			continue
		}
		if d.Code != code {
			t.Errorf("%s: implied code = %q, want %q", name, d.Code, code)
		}
	}

	// Fleet/Air must not get a "Projects" folder.
	for _, bad := range []string{"FleetProjects", "AirProjects", "StudioProjects"} {
		if _, ok := byName[bad]; ok {
			t.Errorf("did not expect %q among default dirs", bad)
		}
	}

	// One entry per family — no duplicates from the two-edition products (IU/IC, PY/PC).
	if len(DefaultProjectDirs()) != len(familyDisplay) {
		t.Errorf("want one dir per family (%d), got %d", len(familyDisplay), len(DefaultProjectDirs()))
	}
}

// TestFamilyPrimaryCodeConsistency guards against drift: every family has a
// primary code, and each names a real product in that same family.
func TestFamilyPrimaryCodeConsistency(t *testing.T) {
	for family := range familyDisplay {
		code, ok := familyPrimaryCode[family]
		if !ok {
			t.Errorf("family %q has no primary code", family)
			continue
		}
		p, ok := products[code]
		if !ok {
			t.Errorf("family %q primary code %q is not a known product", family, code)
			continue
		}
		if p.Family != family {
			t.Errorf("family %q primary code %q belongs to family %q", family, code, p.Family)
		}
	}
}
