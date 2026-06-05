package taskrunner

import (
	"os"
	"path/filepath"
	"testing"
)

// These exercise the detectors that enumerate by invoking the runner's own
// tooling. They skip when the tool isn't installed, so they add coverage on
// machines/CI that have just / go-task without failing where they don't.

func TestJustRecipesWhenInstalled(t *testing.T) {
	if !onPath("just") {
		t.Skip("just not installed")
	}
	dir := t.TempDir()
	body := "# greet someone\nhello:\n    echo hi\n\nbuild:\n    echo build\n"
	if err := os.WriteFile(filepath.Join(dir, "justfile"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	hello, ok := taskByName(tasks, "hello")
	if !ok {
		t.Fatalf("hello recipe not detected; got %v", tasks)
	}
	if hello.Runner != RunnerJust || !equalStrings(hello.Command, []string{"just", "hello"}) {
		t.Errorf("unexpected hello task: %+v", hello)
	}
}

func TestTaskfileTasksWhenInstalled(t *testing.T) {
	bin := goTaskCommand()
	if bin == "" {
		t.Skip("task/go-task not installed")
	}
	dir := t.TempDir()
	body := "version: '3'\ntasks:\n  hello:\n    desc: Greet\n    cmds:\n      - echo hi\n"
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	hello, ok := taskByName(tasks, "hello")
	if !ok {
		t.Fatalf("hello task not detected; got %v", tasks)
	}
	if hello.Runner != RunnerTask || hello.Desc != "Greet" {
		t.Errorf("unexpected hello task: %+v", hello)
	}
}
