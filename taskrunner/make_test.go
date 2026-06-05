package taskrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMakeTargetsParsed(t *testing.T) {
	dir := t.TempDir()
	makefile := `# a comment
.PHONY: build test

CC := gcc
CFLAGS = -O2

build: deps
	$(CC) $(CFLAGS) -o app

test:
	go test ./...

deps:
	echo deps

clean lint:
	echo multi

%.o: %.c
	$(CC) -c $<

build: ## duplicate target, should not double-count
`
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(makefile), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, tk := range tasks {
		if tk.Runner != RunnerMake {
			continue
		}
		got[tk.Name] = true
	}

	for _, want := range []string{"build", "test", "deps", "clean", "lint"} {
		if !got[want] {
			t.Errorf("expected make target %q to be detected; got %v", want, keys(got))
		}
	}
	// Variable assignments, pattern rules, and special targets must not appear.
	for _, bad := range []string{"CC", "CFLAGS", "%.o", ".PHONY"} {
		if got[bad] {
			t.Errorf("did not expect %q to be detected as a target", bad)
		}
	}
	// Duplicate "build" rule should yield a single task.
	count := 0
	for _, tk := range tasks {
		if tk.Name == "build" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected build once, got %d", count)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
