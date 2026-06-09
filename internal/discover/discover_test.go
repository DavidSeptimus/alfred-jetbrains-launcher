package discover

import (
	"os"
	"os/exec"
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

// git runs a git command in dir, failing the test on error.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestWorktreesOf(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "init")

	// No worktrees yet -> nil (and no git call, since .git/worktrees is absent).
	if got := WorktreesOf(repo); got != nil {
		t.Fatalf("a repo with no linked worktrees should return nil, got %v", got)
	}

	// Add a linked worktree inside a dot-dir, mirroring the common ".worktrees"
	// layout the one-level root scan can't reach.
	wt := filepath.Join(repo, ".worktrees", "feature")
	git(t, repo, "worktree", "add", "-q", "-b", "feature", wt)

	got := WorktreesOf(repo)
	if len(got) != 1 {
		t.Fatalf("want exactly the one linked worktree (main excluded), got %v", got)
	}
	// macOS temp dirs are under /var -> /private/var symlink; compare resolved paths.
	wantResolved, _ := filepath.EvalSymlinks(wt)
	gotResolved, _ := filepath.EvalSymlinks(got[0])
	if gotResolved != wantResolved {
		t.Errorf("worktree path = %q, want %q", got[0], wt)
	}

	// A plain directory that isn't a git repo -> nil.
	if got := WorktreesOf(t.TempDir()); got != nil {
		t.Errorf("a non-repo dir should return nil, got %v", got)
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
