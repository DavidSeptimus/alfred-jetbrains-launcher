package taskrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGoFixedVerbs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.23\n")
	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"build", "test", "vet", "tidy"} {
		tk, ok := taskByName(tasks, want)
		if !ok || tk.Runner != RunnerGo {
			t.Errorf("expected go verb %q", want)
		}
	}
	if _, ok := taskByName(tasks, "run"); ok {
		t.Error("run should be absent without main.go")
	}
	writeFile(t, dir, "main.go", "package main\nfunc main(){}\n")
	tasks, _ = Detect(dir, Options{})
	run, ok := taskByName(tasks, "run")
	if !ok || !equalStrings(run.Command, []string{"go", "run", "."}) {
		t.Errorf("expected `go run .` once main.go exists; got %+v", run)
	}
}

func TestCargoVerbsAndRunGate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"x\"\n")
	tasks, _ := Detect(dir, Options{})
	for _, want := range []string{"build", "test", "clippy", "fmt"} {
		if _, ok := taskByName(tasks, want); !ok {
			t.Errorf("expected cargo verb %q", want)
		}
	}
	if _, ok := taskByName(tasks, "run"); ok {
		t.Error("cargo run should be gated on src/main.rs")
	}
	must(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	writeFile(t, dir, filepath.Join("src", "main.rs"), "fn main(){}")
	tasks, _ = Detect(dir, Options{})
	if _, ok := taskByName(tasks, "run"); !ok {
		t.Error("cargo run should appear with src/main.rs")
	}
}

func TestComposerScripts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{"scripts":{"test":"phpunit","pre-install-cmd":"x","lint":["a","b"]}}`)
	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	test, ok := taskByName(tasks, "test")
	if !ok || !equalStrings(test.Command, []string{"composer", "run-script", "test"}) {
		t.Errorf("composer test wrong: %+v", test)
	}
	if _, ok := taskByName(tasks, "pre-install-cmd"); ok {
		t.Error("composer pre-* hooks should be omitted")
	}
	lint, _ := taskByName(tasks, "lint")
	if lint.Desc != "a && b" {
		t.Errorf("array script desc = %q", lint.Desc)
	}
}

func TestDenoTasksJsonc(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "deno.jsonc", "{\n  // comment\n  \"tasks\": {\n    \"dev\": \"deno run -A main.ts\",\n    \"start\": {\"command\": \"deno run main.ts\"}\n  }\n}")
	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	dev, ok := taskByName(tasks, "dev")
	if !ok || !equalStrings(dev.Command, []string{"deno", "task", "dev"}) {
		t.Errorf("deno dev wrong: %+v", dev)
	}
	start, _ := taskByName(tasks, "start")
	if start.Desc != "deno run main.ts" {
		t.Errorf("object task command desc = %q", start.Desc)
	}
}

func TestDenoPreservesCommentMarkersInStrings(t *testing.T) {
	dir := t.TempDir()
	// A task command containing "//" (a URL) and "/*" must survive comment stripping.
	writeFile(t, dir, "deno.json", `{"tasks":{"fetch":"deno run -A https://deno.land/x/foo/mod.ts /* not a comment */"}}`)
	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	fetch, ok := taskByName(tasks, "fetch")
	if !ok {
		t.Fatal("fetch task lost — comment stripper corrupted the JSON")
	}
	if want := "deno run -A https://deno.land/x/foo/mod.ts /* not a comment */"; fetch.Desc != want {
		t.Errorf("desc = %q, want %q", fetch.Desc, want)
	}
}

func TestDotnetDetection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "App.csproj", "<Project/>")
	tasks, _ := Detect(dir, Options{})
	for _, want := range []string{"build", "run", "test", "publish"} {
		if _, ok := taskByName(tasks, want); !ok {
			t.Errorf("expected dotnet verb %q", want)
		}
	}
	b, _ := taskByName(tasks, "build")
	if b.Runner != RunnerDotnet || b.Source != "App.csproj" {
		t.Errorf("unexpected dotnet task: %+v", b)
	}
}

func TestRakeSkippedWithoutTool(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Rakefile", "task :build\n")
	// rakeDetector.Available is true, but Tasks returns nil when rake isn't installed.
	if !onPath("rake") {
		tasks, err := Detect(dir, Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, tk := range tasks {
			if tk.Runner == RunnerRake {
				t.Error("no rake tasks expected without the tool")
			}
		}
	}
}

func TestDisableNewRunner(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module x\n")
	tasks, _ := Detect(dir, Options{Disabled: []Runner{RunnerGo}})
	for _, tk := range tasks {
		if tk.Runner == RunnerGo {
			t.Error("go runner should be disabled")
		}
	}
}
