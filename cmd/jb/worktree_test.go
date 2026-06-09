package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
)

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
