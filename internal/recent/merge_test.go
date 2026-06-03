package recent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mkdir(p string) error { return os.MkdirAll(p, 0o755) }

func entry(canon, code, datadir string, tsMillis int64) RawEntry {
	return RawEntry{
		Ref:            PathRef{Raw: canon, Canonical: canon, Kind: KindLocal},
		ProductionCode: code,
		SourceDataDir:  datadir,
		Timestamp:      time.UnixMilli(tsMillis),
	}
}

func TestMergeDedupeAndCodes(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "alpha")
	if err := mkdir(existing); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "ghost")

	entries := []RawEntry{
		entry(existing, "GO", "GoLand2025.1", 100),
		entry(existing, "IU", "IntelliJIdea2026.1", 200), // newer -> wins
		entry(missing, "AI", "AndroidStudio2026.1.1", 300),
		{Ref: PathRef{Raw: "/$devcontainer.ij/x", Kind: KindRemote}, Timestamp: time.UnixMilli(999)},
	}

	projects := Merge(entries, nil)
	if len(projects) != 2 {
		t.Fatalf("want 2 projects (remote excluded), got %d", len(projects))
	}

	// existing should sort before missing despite missing having a newer ts.
	if projects[0].Path != existing {
		t.Errorf("existing project should sort first, got %q", projects[0].Path)
	}
	if !projects[0].Exists || projects[1].Exists {
		t.Errorf("Exists flags wrong: %v / %v", projects[0].Exists, projects[1].Exists)
	}

	winner := projects[0]
	if winner.ProductionCode != "IU" || winner.SourceDataDir != "IntelliJIdea2026.1" {
		t.Errorf("newest entry should win: got code=%q datadir=%q", winner.ProductionCode, winner.SourceDataDir)
	}
	if len(winner.AllCodes) != 2 || winner.AllCodes[0] != "GO" || winner.AllCodes[1] != "IU" {
		t.Errorf("AllCodes should union both: got %v", winner.AllCodes)
	}
	if winner.DisplayName != "alpha" {
		t.Errorf("DisplayName should be folder base, got %q", winner.DisplayName)
	}
}

func TestMergeWorktreeDetection(t *testing.T) {
	dir := t.TempDir()
	normal := filepath.Join(dir, "normal")
	wt := filepath.Join(dir, "wt")
	sub := filepath.Join(dir, "sub")
	for _, d := range []string{normal, wt, sub} {
		if err := mkdir(d); err != nil {
			t.Fatal(err)
		}
	}
	// normal repo: .git is a directory
	if err := mkdir(filepath.Join(normal, ".git")); err != nil {
		t.Fatal(err)
	}
	// worktree: .git is a file pointing at .../worktrees/<name>
	os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /repo/.git/worktrees/feature\n"), 0o644)
	// submodule: .git is a file pointing at .../modules/<name> (NOT a worktree)
	os.WriteFile(filepath.Join(sub, ".git"), []byte("gitdir: /repo/.git/modules/lib\n"), 0o644)

	byPath := map[string]Project{}
	for _, p := range Merge([]RawEntry{
		entry(normal, "IU", "x", 300),
		entry(wt, "IU", "x", 200),
		entry(sub, "IU", "x", 100),
	}, nil) {
		byPath[p.Path] = p
	}
	if byPath[normal].IsWorktree {
		t.Error("normal repo (.git dir) should not be a worktree")
	}
	if !byPath[wt].IsWorktree {
		t.Error("linked worktree (.git file -> worktrees/) should be detected")
	}
	if byPath[sub].IsWorktree {
		t.Error("submodule (.git file -> modules/) should not be flagged as a worktree")
	}
}

func TestMergeStubDetection(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub")   // only hidden: .idea + .git + .DS_Store
	leftover := filepath.Join(dir, "lo") // only ignored content: build + node_modules + .idea
	real := filepath.Join(dir, "real")   // .idea + a visible source file
	empty := filepath.Join(dir, "empty") // nothing
	for _, d := range []string{stub, leftover, real, empty} {
		if err := mkdir(d); err != nil {
			t.Fatal(err)
		}
	}
	for _, h := range []string{".idea", ".git"} {
		if err := mkdir(filepath.Join(stub, h)); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(stub, ".DS_Store"), []byte("x"), 0o644)
	for _, h := range []string{".idea", "build", "node_modules"} {
		if err := mkdir(filepath.Join(leftover, h)); err != nil {
			t.Fatal(err)
		}
	}
	if err := mkdir(filepath.Join(real, ".idea")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(real, "main.go"), []byte("package main"), 0o644)

	ignore := []string{"build", "dist", "node_modules"}
	byPath := map[string]Project{}
	for _, p := range Merge([]RawEntry{
		entry(stub, "IU", "x", 400),
		entry(leftover, "IU", "x", 300),
		entry(real, "IU", "x", 200),
		entry(empty, "IU", "x", 100),
	}, ignore) {
		byPath[p.Path] = p
	}
	if !byPath[stub].Stub {
		t.Error("a dir of only hidden entries should be a Stub")
	}
	if !byPath[leftover].Stub {
		t.Error("a dir with only ignored content (build/node_modules) should be a Stub")
	}
	if byPath[real].Stub {
		t.Error("a dir with a visible source file should not be a Stub")
	}
	if !byPath[empty].Stub {
		t.Error("an empty dir should be a Stub")
	}
}

func TestMergeSortRecencyAmongExisting(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "older")
	newer := filepath.Join(dir, "newer")
	for _, d := range []string{older, newer} {
		if err := mkdir(d); err != nil {
			t.Fatal(err)
		}
	}
	projects := Merge([]RawEntry{
		entry(older, "IU", "x", 100),
		entry(newer, "IU", "x", 500),
	}, nil)
	if projects[0].Path != newer {
		t.Errorf("newer project should sort first, got %q", projects[0].Path)
	}
}
