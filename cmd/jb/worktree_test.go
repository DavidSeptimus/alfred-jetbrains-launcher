package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
)

// TestEmitSearchWorktreeOnly verifies the `~` variant is a worktree-only list:
// a normal recent project is dropped, while a discovered worktree of it shows.
func TestEmitSearchWorktreeOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	root := filepath.Join(tmp, "roots")
	repo := filepath.Join(root, "demo-repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-qm", "init")
	gitIn(t, repo, "worktree", "add", "-q", "-b", "wt-branch", filepath.Join(repo, ".worktrees", "wt-branch"))

	cfg := config.Config{
		Home:         tmp,
		CacheDir:     filepath.Join(tmp, "cache"),
		DataDir:      filepath.Join(tmp, "data"),
		ProjectRoots: []string{root},
	}
	for _, d := range []string{cfg.CacheDir, cfg.DataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Assert on the title field, not raw substrings: the worktree's own path
	// contains "demo-repo", so a naive Contains would collide.
	const repoTitle = `"title":"demo-repo"`
	const wtTitle = `"title":"⑂ wt-branch"`

	// `+` lists the un-opened repo (a non-worktree) but not the worktree.
	roots := captureStdout(t, func() { emitSearch(cfg, "", "", false, true, "jb+") })
	if !strings.Contains(roots, repoTitle) {
		t.Errorf("`+` should list the un-opened repo, got:\n%s", roots)
	}
	if strings.Contains(roots, wtTitle) {
		t.Errorf("`+` must not list worktrees, got:\n%s", roots)
	}

	// `~` lists the worktree (⑂-marked) but drops the non-worktree repo.
	wt := captureStdout(t, func() { emitSearch(cfg, "", "", true, false, "jb~") })
	if !strings.Contains(wt, wtTitle) {
		t.Errorf("`~` should list the discovered worktree, got:\n%s", wt)
	}
	if strings.Contains(wt, repoTitle) {
		t.Errorf("`~` must drop non-worktree projects, got:\n%s", wt)
	}
}

// gitIn runs a git command in dir, failing the test on error.
func gitIn(t *testing.T, dir string, args ...string) {
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

// TestScanWorktreesDropsPrunable is the regression guard for deleted worktrees:
// `git worktree list` keeps reporting a worktree whose working dir was removed
// (marked "prunable") until `git worktree prune` runs, so discovery must not
// surface it. scanWorktrees collects whatever git reports; the existence guard in
// AppendUnopened/ProjectFromPath drops the dead one, leaving only the live tree.
func TestScanWorktreesDropsPrunable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-qm", "init")

	// Two linked worktrees in the common in-repo dot-dir layout.
	live := filepath.Join(repo, ".worktrees", "live")
	gone := filepath.Join(repo, ".worktrees", "gone")
	gitIn(t, repo, "worktree", "add", "-q", "-b", "live", live)
	gitIn(t, repo, "worktree", "add", "-q", "-b", "gone", gone)

	// Delete one on disk WITHOUT pruning — git keeps listing it as prunable.
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	// Sanity: git (and thus WorktreesOf) still reports the prunable worktree.
	scans := scanWorktrees([]recent.Project{{Path: repo, Exists: true}})
	if len(scans) != 2 {
		t.Fatalf("WorktreesOf should still report both worktrees (one prunable), got %d: %+v", len(scans), scans)
	}

	// The build folds them through AppendUnopened, whose existence guard drops the
	// deleted one. Only the live worktree must survive.
	got := recent.AppendUnopened(nil, scans, nil)
	if len(got) != 1 {
		t.Fatalf("want exactly the live worktree (prunable dropped), got %d: %+v", len(got), got)
	}
	wantResolved, _ := filepath.EvalSymlinks(live)
	gotResolved, _ := filepath.EvalSymlinks(got[0].Path)
	if gotResolved != wantResolved {
		t.Errorf("surviving worktree = %q, want %q", got[0].Path, live)
	}
	if !got[0].IsWorktree {
		t.Error("a discovered worktree should be flagged IsWorktree")
	}
}
