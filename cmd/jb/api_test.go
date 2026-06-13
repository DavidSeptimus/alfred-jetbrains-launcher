package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/state"
	taskrunner "github.com/davidseptimus/alfred-taskrunner"
)

func TestAPIVariantFlags(t *testing.T) {
	cases := []struct {
		in        string
		wantWT    bool
		wantRoots bool
	}{
		{"recent", false, false},
		{"", false, false},
		{"worktrees", true, false},
		{"~", true, false},
		{"roots", false, true},
		{"unopened", false, true},
		{"+", false, true},
	}
	for _, c := range cases {
		gotWT, gotRoots := apiVariantFlags(c.in)
		if gotWT != c.wantWT || gotRoots != c.wantRoots {
			t.Errorf("apiVariantFlags(%q) = (%v,%v), want (%v,%v)", c.in, gotWT, gotRoots, c.wantWT, c.wantRoots)
		}
	}
}

func TestAPITaskItemLaunchMatrix(t *testing.T) {
	item := apiTaskItem(config.Config{}, taskrunner.Task{
		Name:     "runIde",
		Runner:   taskrunner.RunnerGradle,
		Command:  []string{"./gradlew", "runIde"},
		Cwd:      "/p",
		Source:   "build.gradle.kts",
		Runnable: true,
	})

	us := "\x1f"
	if item.Spec != "tab"+us+"/p"+us+"./gradlew runIde" {
		t.Errorf("default spec = %q", item.Spec)
	}
	if item.WindowSpec != "window"+us+"/p"+us+"./gradlew runIde" {
		t.Errorf("window spec = %q", item.WindowSpec)
	}
	if item.BGSpec != "bg"+us+"/p"+us+"./gradlew runIde" {
		t.Errorf("background spec = %q", item.BGSpec)
	}
	if item.CopySpec != "copy"+us+"/p"+us+"./gradlew runIde" {
		t.Errorf("copy spec = %q", item.CopySpec)
	}
	if item.ResetSpec != "tabreset"+us+"/p"+us+"./gradlew runIde" {
		t.Errorf("reset spec = %q", item.ResetSpec)
	}
}

func TestAPITaskItemHonorsWindowDefault(t *testing.T) {
	item := apiTaskItem(config.Config{TaskNewWindow: true}, taskrunner.Task{
		Name:     "test",
		Runner:   taskrunner.RunnerNpm,
		Command:  []string{"npm", "test"},
		Cwd:      "/p",
		Source:   "package.json",
		Runnable: true,
	})
	if !strings.HasPrefix(item.Spec, "window\x1f") {
		t.Errorf("default spec = %q, want window launch", item.Spec)
	}
	if !strings.HasPrefix(item.ResetSpec, "windowreset\x1f") {
		t.Errorf("reset spec = %q, want window reset launch", item.ResetSpec)
	}
}

// TestAPITaskItemParityWithAlfred guards against the API and Alfred task builders
// drifting: they now share taskSubtitle and the same spec(kind) wire format, so
// every field the two frontends both carry must match for the same task.
func TestAPITaskItemParityWithAlfred(t *testing.T) {
	task := taskrunner.Task{
		Name:     "build",
		Runner:   taskrunner.RunnerGradle,
		Command:  []string{"./gradlew", "build"},
		Cwd:      "/p",
		Source:   "build.gradle.kts",
		Desc:     "Assembles the outputs",
		Runnable: true,
	}
	for _, newWindow := range []bool{false, true} {
		cfg := config.Config{TaskNewWindow: newWindow}
		api := apiTaskItem(cfg, task)
		alf := taskItem(cfg, task)

		if api.Subtitle != alf.Subtitle {
			t.Errorf("newWindow=%v subtitle: api %q != alfred %q", newWindow, api.Subtitle, alf.Subtitle)
		}
		if api.Match != alf.Match {
			t.Errorf("newWindow=%v match: api %q != alfred %q", newWindow, api.Match, alf.Match)
		}
		if api.Spec != alf.Arg {
			t.Errorf("newWindow=%v default spec: api %q != alfred arg %q", newWindow, api.Spec, alf.Arg)
		}
		// Alfred's ⌘ runs in the non-default view, which is exactly the API's
		// window or tab spec depending on the configured default.
		otherSpec := api.WindowSpec
		if newWindow {
			otherSpec = api.TabSpec
		}
		if otherSpec != alf.Mods["cmd"].Arg {
			t.Errorf("newWindow=%v other-view spec: api %q != alfred cmd %q", newWindow, otherSpec, alf.Mods["cmd"].Arg)
		}
		if api.BGSpec != alf.Mods["alt"].Arg {
			t.Errorf("newWindow=%v bg spec: api %q != alfred alt %q", newWindow, api.BGSpec, alf.Mods["alt"].Arg)
		}
		if api.CopySpec != alf.Mods["ctrl"].Arg {
			t.Errorf("newWindow=%v copy spec: api %q != alfred ctrl %q", newWindow, api.CopySpec, alf.Mods["ctrl"].Arg)
		}
		if api.ResetSpec != alf.Mods["shift"].Arg {
			t.Errorf("newWindow=%v reset spec: api %q != alfred shift %q", newWindow, api.ResetSpec, alf.Mods["shift"].Arg)
		}
		if alf.Icon == nil || api.Icon.Path != alf.Icon.Path {
			t.Errorf("newWindow=%v icon: api %q != alfred %v", newWindow, api.Icon.Path, alf.Icon)
		}
	}
}

// TestAPIProjectItemSubtitle locks the project subtitle to the shared
// projectSubtitle helper (the Alfred search uses the same one) and guards against
// the ASCII-separator regression the review found.
func TestAPIProjectItemSubtitle(t *testing.T) {
	cfg := config.Config{Home: "/Users/me"}
	p := recent.Project{Path: "/Users/me/code/demo", DisplayName: "demo", Exists: true}
	item := apiProjectItem(cfg, p, nil, false, "", true, false, false)

	want := projectSubtitle(cfg, p, false, true, "", "")
	if item.Subtitle != want {
		t.Errorf("subtitle = %q, want %q (shared helper)", item.Subtitle, want)
	}
	if !strings.Contains(item.Subtitle, "  —  no IDE installed") {
		t.Errorf("subtitle %q lost the em-dash no-IDE warning (separator regression)", item.Subtitle)
	}
}

// TestAPIRerunResetSpec covers apiRerun's launch matrix (previously untested) and
// locks in the fix that the reset spec runs in the DEFAULT view, like task rows —
// not the alternate view it used before.
func TestAPIRerunResetSpec(t *testing.T) {
	us := "\x1f"
	cases := []struct {
		newWindow     bool
		wantDefault   string
		wantResetKind string
	}{
		{false, "tab", "tabreset"},
		{true, "window", "windowreset"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		t.Setenv("JB_DATA_DIR", dir)
		t.Setenv("JB_CACHE_DIR", t.TempDir())
		if c.newWindow {
			t.Setenv("JB_TASK_WINDOW", "1")
		} else {
			t.Setenv("JB_TASK_WINDOW", "0")
		}
		saveLastRun(config.Load(), "/work/proj", "npm test")

		out := captureStdout(t, func() { apiRerun(nil) })
		var decoded apiOutput[apiTask]
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			t.Fatalf("newWindow=%v: not JSON: %v\n%s", c.newWindow, err, out)
		}
		if len(decoded.Items) != 1 {
			t.Fatalf("newWindow=%v: items = %d, want 1", c.newWindow, len(decoded.Items))
		}
		it := decoded.Items[0]
		if it.Spec != c.wantDefault+us+"/work/proj"+us+"npm test" {
			t.Errorf("newWindow=%v default spec = %q", c.newWindow, it.Spec)
		}
		wantReset := c.wantResetKind + us + "/work/proj" + us + "npm test"
		if it.ResetSpec != wantReset {
			t.Errorf("newWindow=%v reset spec = %q, want %q", c.newWindow, it.ResetSpec, wantReset)
		}
	}
}

// TestProjectInVariant pins the single visibility gate both frontends share, so
// the cached-loader switch can't change which projects a variant surfaces. It
// uses real dirs because the gate reconciles with the filesystem (.git presence)
// to drop leftover husks of removed worktrees.
func TestProjectInVariant(t *testing.T) {
	cfg := config.Config{}
	st := state.State{}

	mkdir := func(p string) string {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}

	root := t.TempDir()
	repo := mkdir(filepath.Join(root, "app")) // git repo, immediate child of a root
	mkdir(filepath.Join(repo, ".git"))
	plain := mkdir(filepath.Join(root, "plain"))      // non-git, but an immediate root child
	husk := mkdir(filepath.Join(root, "sub", "dead")) // non-git, nested → removed-worktree leftover
	wt := mkdir(filepath.Join(t.TempDir(), "feat"))   // worktree living outside any root
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /x/.git/worktrees/feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	roots := map[string]bool{root: true}

	gitRepo := recent.Project{Path: repo, Exists: true}
	rootChild := recent.Project{Path: plain, Exists: true}                                   // non-git folder opened from a root
	diskWorktree := recent.Project{Path: wt, Exists: true, IsWorktree: true, Unopened: true} // worktree anywhere
	rootEntry := recent.Project{Path: plain, Exists: true, Unopened: true}                   // un-opened `+` entry
	leftover := recent.Project{Path: husk, Exists: true}                                     // husk: no .git, nested
	pinnedLoose := recent.Project{Path: husk, Exists: true}                                  // same shape, but pinned
	stub := recent.Project{Path: repo, Exists: true, Stub: true}

	pinnedSt := state.State{Pinned: []string{husk}}

	cases := []struct {
		name             string
		p                recent.Project
		st               state.State
		wtFlag, scanRoot bool
		want             bool
	}{
		{"git repo in recents", gitRepo, st, false, false, true},
		{"git repo not in ~", gitRepo, st, true, false, false},
		{"git repo in +", gitRepo, st, false, true, true},
		{"non-git root child shown in recents", rootChild, st, false, false, true},
		{"disk worktree only in ~", diskWorktree, st, true, false, true},
		{"disk worktree hidden in recents", diskWorktree, st, false, false, false},
		{"root entry only in +", rootEntry, st, false, true, true},
		{"leftover husk hidden in recents", leftover, st, false, false, false},
		{"leftover husk hidden in ~", leftover, st, true, false, false},
		{"leftover husk hidden in +", leftover, st, false, true, false},
		{"pinned loose dir shown despite no git / not a root child", pinnedLoose, pinnedSt, false, false, true},
		{"stub never shown", stub, st, false, false, false},
	}
	for _, c := range cases {
		if got := projectInVariant(c.p, c.st, cfg, roots, c.wtFlag, c.scanRoot); got != c.want {
			t.Errorf("%s: projectInVariant = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEmitAPIShape(t *testing.T) {
	out := captureStdout(t, func() {
		emitAPI(apiOutput[apiProject]{Items: []apiProject{{Title: "Demo", Path: "/p/demo"}}})
	})
	var decoded apiOutput[apiProject]
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("api output is not JSON: %v\n%s", err, out)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].Title != "Demo" || decoded.Items[0].Path != "/p/demo" {
		t.Fatalf("decoded output = %+v", decoded)
	}
}
