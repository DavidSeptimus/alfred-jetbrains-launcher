package main

import (
	"strings"
	"testing"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
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
