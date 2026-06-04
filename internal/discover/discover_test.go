package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
)

func writeRecent(t *testing.T, root, dataDir, file string) {
	t.Helper()
	dir := filepath.Join(root, dataDir, "options")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte("<application/>"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFind(t *testing.T) {
	jb := t.TempDir()
	google := t.TempDir()

	writeRecent(t, jb, "IntelliJIdea2026.2", "recentProjects.xml")
	writeRecent(t, jb, "IntelliJIdea2025.1", "recentProjects.xml")
	writeRecent(t, jb, "Rider2026.1", "recentSolutions.xml")
	writeRecent(t, jb, "AndroidStudio2026.1.1-backup/2026-05-04", "recentProjects.xml") // skipped (-backup)
	// Toolbox has no options/recentProjects.xml -> falls away.
	if err := os.MkdirAll(filepath.Join(jb, "Toolbox", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRecent(t, google, "AndroidStudio2026.1.1", "recentProjects.xml")
	writeRecent(t, google, "SomethingElse", "recentProjects.xml") // skipped (Google requires AndroidStudio prefix)

	cfg := config.Config{ConfigRoots: []config.Root{
		{Dir: jb, Vendor: "JetBrains"},
		{Dir: google, Vendor: "Google"},
	}}

	files := Find(cfg)
	got := map[string]RecentFile{}
	for _, f := range files {
		got[f.DataDir] = f
	}

	wantDirs := []string{"IntelliJIdea2026.2", "IntelliJIdea2025.1", "Rider2026.1", "AndroidStudio2026.1.1"}
	if len(files) != len(wantDirs) {
		t.Fatalf("want %d files, got %d: %+v", len(wantDirs), len(files), files)
	}
	for _, d := range wantDirs {
		if _, ok := got[d]; !ok {
			t.Errorf("missing expected file for %s", d)
		}
	}
	if got["IntelliJIdea2026.2"].Product != "IntelliJIdea" || got["IntelliJIdea2026.2"].Version != "2026.2" {
		t.Errorf("bad product/version split: %+v", got["IntelliJIdea2026.2"])
	}
	if !got["Rider2026.1"].IsSolutions {
		t.Error("Rider recentSolutions.xml should set IsSolutions")
	}
}

func TestFindProjectDirs(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"alpha", "beta", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A plain file at the root must be skipped (only dirs are projects).
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	roots := []string{
		root,
		filepath.Join(root, "does-not-exist"), // missing root -> skipped silently
	}

	got := map[string]bool{}
	for _, d := range FindProjectDirs(roots) {
		got[filepath.Base(d)] = true
	}
	if len(got) != 2 || !got["alpha"] || !got["beta"] {
		t.Fatalf("want exactly alpha+beta (dotdir, file, missing root excluded), got %v", got)
	}
}

func TestSplitProductVersion(t *testing.T) {
	cases := map[string][2]string{
		"IntelliJIdea2026.2":    {"IntelliJIdea", "2026.2"},
		"RustRover2025.3":       {"RustRover", "2025.3"},
		"AndroidStudio2026.1.1": {"AndroidStudio", "2026.1.1"},
		"PyCharmCE2024.1":       {"PyCharmCE", "2024.1"},
		"Fleet":                 {"Fleet", ""},
	}
	for in, want := range cases {
		p, v := splitProductVersion(in)
		if p != want[0] || v != want[1] {
			t.Errorf("%s -> (%q,%q), want (%q,%q)", in, p, v, want[0], want[1])
		}
	}
}
