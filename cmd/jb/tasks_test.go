package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
	taskrunner "github.com/davidseptimus/alfred-taskrunner"
)

func TestShellJoinArgv(t *testing.T) {
	cases := map[string][]string{
		"npm run dev":         {"npm", "run", "dev"},
		"./gradlew runIde":    {"./gradlew", "runIde"},
		"mvn -DskipTests pkg": {"mvn", "-DskipTests", "pkg"}, // '-' is not special; only metachars quoted
		"task 'a b'":          {"task", "a b"},
		`sh -c 'echo $HOME'`:  {"sh", "-c", "echo $HOME"},
	}
	for want, argv := range cases {
		if got := shellJoinArgv(argv); got != want {
			t.Errorf("shellJoinArgv(%v) = %q, want %q", argv, got, want)
		}
	}
}

func TestDisabledRunners(t *testing.T) {
	got := disabledRunners([]string{"gradle", "bogus", "NPM", " maven "})
	want := map[taskrunner.Runner]bool{taskrunner.RunnerGradle: true, taskrunner.RunnerNpm: true, taskrunner.RunnerMaven: true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, r := range got {
		if !want[r] {
			t.Errorf("unexpected runner %q", r)
		}
	}
}

func TestReplaceRunner(t *testing.T) {
	in := []taskrunner.Task{
		{Name: "dev", Runner: taskrunner.RunnerNpm},
		{Name: "build", Runner: taskrunner.RunnerGradle},
		{Name: "assemble", Runner: taskrunner.RunnerGradle},
	}
	repl := []taskrunner.Task{{Name: "runIde", Runner: taskrunner.RunnerGradle}}
	out := replaceRunner(in, taskrunner.RunnerGradle, repl)
	if len(out) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(out))
	}
	names := out[0].Name + "," + out[1].Name
	if names != "dev,runIde" {
		t.Errorf("expected dev,runIde, got %s", names)
	}
}

func TestGradleRefreshItem(t *testing.T) {
	gradleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(gradleDir, "build.gradle.kts"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("gradle project shows refresh row carrying its path", func(t *testing.T) {
		item, ok := gradleRefreshItem(config.Config{}, gradleDir)
		if !ok {
			t.Fatal("expected a refresh row for a Gradle project")
		}
		if item.Arg != "refresh"+specSep+gradleDir {
			t.Errorf("arg = %q, want refresh<US>%s", item.Arg, gradleDir)
		}
		if item.Valid == nil || !*item.Valid {
			t.Error("refresh row should be valid")
		}
	})

	t.Run("non-gradle project has no refresh row", func(t *testing.T) {
		if _, ok := gradleRefreshItem(config.Config{}, t.TempDir()); ok {
			t.Error("expected no refresh row for a non-Gradle project")
		}
	})

	t.Run("disabling gradle hides the refresh row", func(t *testing.T) {
		if _, ok := gradleRefreshItem(config.Config{TaskDisable: []string{"gradle"}}, gradleDir); ok {
			t.Error("expected no refresh row when Gradle is disabled")
		}
	})
}

func TestGradleEnumInFlight(t *testing.T) {
	cfg := config.Config{CacheDir: t.TempDir()}
	const project = "/some/gradle/project"
	marker := gradleCachePath(cfg, project) + ".spawning"

	t.Run("no marker is not in flight", func(t *testing.T) {
		if gradleEnumInFlight(cfg, project) {
			t.Error("expected not-in-flight with no marker")
		}
	})

	t.Run("fresh marker is in flight", func(t *testing.T) {
		if err := os.WriteFile(marker, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if !gradleEnumInFlight(cfg, project) {
			t.Error("expected in-flight with a fresh marker")
		}
	})

	t.Run("stale marker is not in flight", func(t *testing.T) {
		old := time.Now().Add(-2 * gradleEnumLease)
		if err := os.Chtimes(marker, old, old); err != nil {
			t.Fatal(err)
		}
		if gradleEnumInFlight(cfg, project) {
			t.Error("a marker past the lease should read as not-in-flight (crashed leftover)")
		}
	})
}

func TestGradleEnumErroredAndCooldown(t *testing.T) {
	cfg := config.Config{CacheDir: t.TempDir()}
	const project = "/some/gradle/project"
	errMarker := gradleErrorMarker(cfg, project)
	spawnMarker := gradleSpawnMarker(cfg, project)

	t.Run("no error marker is not errored", func(t *testing.T) {
		if gradleEnumErrored(cfg, project) {
			t.Error("expected not-errored with no marker")
		}
	})

	t.Run("fresh error marker reads as errored", func(t *testing.T) {
		if err := os.WriteFile(errMarker, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if !gradleEnumErrored(cfg, project) {
			t.Error("expected errored with a fresh error marker")
		}
	})

	t.Run("fresh error marker suppresses a re-spawn", func(t *testing.T) {
		// Error marker is present (prior subtest). spawnGradleEnumeration must back
		// off at the cooldown check and never create the spawn marker — so this also
		// never reaches cmd.Start() (no stray process in the test).
		_ = os.Remove(spawnMarker)
		spawnGradleEnumeration(cfg, project)
		if _, err := os.Stat(spawnMarker); !os.IsNotExist(err) {
			t.Error("spawn should be suppressed while the error cooldown is fresh")
		}
	})

	t.Run("stale error marker reads as cleared", func(t *testing.T) {
		old := time.Now().Add(-2 * gradleEnumLease)
		if err := os.Chtimes(errMarker, old, old); err != nil {
			t.Fatal(err)
		}
		if gradleEnumErrored(cfg, project) {
			t.Error("an error marker past the lease should read as cleared")
		}
	})
}

func TestRecordGradleEnumError(t *testing.T) {
	cfg := config.Config{CacheDir: t.TempDir()}
	const project = "/some/gradle/project"
	spawn := gradleSpawnMarker(cfg, project)
	errMark := gradleErrorMarker(cfg, project)

	if err := os.WriteFile(spawn, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	recordGradleEnumError(cfg, project)

	if _, err := os.Stat(errMark); err != nil {
		t.Errorf("expected an error marker to be written: %v", err)
	}
	if _, err := os.Stat(spawn); !os.IsNotExist(err) {
		t.Error("the spawn marker should be cleared once the error marker is in place")
	}
}

func TestRefreshGradleTaskCacheNonGradleClearsSpinner(t *testing.T) {
	cfg := config.Config{CacheDir: t.TempDir()}
	// A non-Gradle dir: GradleFingerprint == "" so the worker does no enumeration,
	// but it must still clear any in-flight marker so the spinner can't wedge.
	project := t.TempDir()
	spawn := gradleSpawnMarker(cfg, project)
	if err := os.WriteFile(spawn, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	refreshGradleTaskCache(cfg, project)

	if _, err := os.Stat(spawn); !os.IsNotExist(err) {
		t.Error("a non-Gradle refresh should clear the spawn marker")
	}
	if _, err := os.Stat(gradleErrorMarker(cfg, project)); !os.IsNotExist(err) {
		t.Error("a non-Gradle refresh should not write an error marker (no cooldown needed)")
	}
}

func TestTaskItemLaunchMatrix(t *testing.T) {
	item := taskItem(config.Config{}, taskrunner.Task{
		Name: "runIde", Runner: taskrunner.RunnerGradle,
		Command: []string{"./gradlew", "runIde"}, Cwd: "/p", Source: "build.gradle.kts", Runnable: true,
	})
	if item.Title != "runIde" {
		t.Errorf("title = %q", item.Title)
	}
	us := "\x1f"
	if item.Arg != "tab"+us+"/p"+us+"./gradlew runIde" {
		t.Errorf("Enter arg = %q", item.Arg)
	}
	if item.Mods["cmd"].Arg != "window"+us+"/p"+us+"./gradlew runIde" {
		t.Errorf("cmd(window) arg = %q", item.Mods["cmd"].Arg)
	}
	if item.Mods["alt"].Arg != "bg"+us+"/p"+us+"./gradlew runIde" {
		t.Errorf("alt(bg) arg = %q", item.Mods["alt"].Arg)
	}
	if !strings.HasPrefix(item.Mods["ctrl"].Arg, "copy"+us) {
		t.Errorf("ctrl(copy) arg = %q", item.Mods["ctrl"].Arg)
	}
}

// TestRuntaskStateVariantRoundTrip covers the persisted-variant state: a pick
// records both the project and the picker variant it came from; "back"
// (clearRuntaskTarget) drops the project but keeps the variant so it can reopen
// the same widened picker; and a plain-variant clear removes the file entirely.
func TestRuntaskStateVariantRoundTrip(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	setRuntaskTarget(cfg, "/p/proj", "~")
	if s := loadRuntaskState(cfg); s.Path != "/p/proj" || s.Variant != "~" {
		t.Fatalf("after pick, state = %+v, want {/p/proj ~}", s)
	}

	clearRuntaskTarget(cfg) // "back": keep the variant, drop the project
	if s := loadRuntaskState(cfg); s.Path != "" || s.Variant != "~" {
		t.Fatalf("after back, state = %+v, want {\"\" ~}", s)
	}
	if loadRuntaskTarget(cfg) != "" {
		t.Error("target should be empty after back")
	}

	// A plain-variant scope has nothing to preserve, so clearing removes the file.
	setRuntaskTarget(cfg, "/p/proj", "")
	clearRuntaskTarget(cfg)
	if _, err := os.Stat(runtaskStatePath(cfg)); !os.IsNotExist(err) {
		t.Error("clearing a plain-variant scope should remove the state file")
	}
}

// TestProjectPickItemCarriesVariant verifies a picker row encodes the active
// variant in its picktask spec (so "back" returns to the same picker) and marks
// worktrees with the same glyph the jb keyword uses.
func TestProjectPickItemCarriesVariant(t *testing.T) {
	cfg := config.Config{}

	wt := projectPickItem(cfg, recent.Project{Path: "/p/wt", DisplayName: "wt", IsWorktree: true}, nil, false, "~")
	if wt.Arg != "picktask"+specSep+"/p/wt"+specSep+"~" {
		t.Errorf("worktree pick arg = %q, want picktask<US>/p/wt<US>~", wt.Arg)
	}
	if wt.Title != worktreeGlyph+" wt" {
		t.Errorf("worktree row should be glyph-marked, got %q", wt.Title)
	}

	// Plain variant → empty trailing field; a pinned row gets the ★ marker.
	plain := projectPickItem(cfg, recent.Project{Path: "/p/a", DisplayName: "a"}, nil, true, "")
	if plain.Arg != "picktask"+specSep+"/p/a"+specSep {
		t.Errorf("plain pick arg = %q, want picktask<US>/p/a<US>", plain.Arg)
	}
	if plain.Title != "★ a" {
		t.Errorf("pinned title = %q, want ★ a", plain.Title)
	}
}

// TestEmitRuntaskVariantForcesPicker locks in the core reworked semantics: with
// a project saved, plain `runtask` resumes its task list, but `runtask~`/`+`
// always open the (widened) picker — without discarding the saved project.
func TestEmitRuntaskVariantForcesPicker(t *testing.T) {
	target := t.TempDir() // a real dir so plain runtask enters task mode
	cfg := config.Config{Home: t.TempDir(), DataDir: t.TempDir(), CacheDir: t.TempDir()}
	setRuntaskTarget(cfg, target, "")

	plain := captureStdout(t, func() { emitRuntask(cfg, "", false, false) })
	if !strings.Contains(plain, "Switch project") {
		t.Errorf("plain runtask with a saved target should show the task list (back row), got:\n%s", plain)
	}

	wt := captureStdout(t, func() { emitRuntask(cfg, "", true, false) })
	if strings.Contains(wt, "Switch project") {
		t.Errorf("runtask~ must force the picker, not the saved task list, got:\n%s", wt)
	}
	if !strings.Contains(wt, "No git worktrees") {
		t.Errorf("runtask~ picker should show the worktree empty hint, got:\n%s", wt)
	}

	if loadRuntaskTarget(cfg) != target {
		t.Error("opening a variant picker must not drop the saved project")
	}
}

func TestTaskItemNonRunnableStillCopyable(t *testing.T) {
	item := taskItem(config.Config{}, taskrunner.Task{
		Name: "dev", Runner: taskrunner.RunnerNpm,
		Command: []string{"pnpm", "run", "dev"}, Cwd: "/p", Source: "package.json", Runnable: false,
	})
	if item.Valid == nil || *item.Valid {
		t.Error("non-runnable task should be invalid for ↩")
	}
	if item.Mods["ctrl"].Valid == nil || !*item.Mods["ctrl"].Valid {
		t.Error("copy modifier should stay valid even when the tool is missing")
	}
	if !strings.Contains(item.Subtitle, "pnpm not found") {
		t.Errorf("subtitle should note the missing tool: %q", item.Subtitle)
	}
}
