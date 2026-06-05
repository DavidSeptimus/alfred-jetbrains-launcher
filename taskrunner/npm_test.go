package taskrunner

import (
	"os"
	"path/filepath"
	"testing"
)

// writePackageJSON writes a package.json into a fresh temp dir and returns it.
func writePackageJSON(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func taskByName(tasks []Task, name string) (Task, bool) {
	for _, tk := range tasks {
		if tk.Name == name {
			return tk, true
		}
	}
	return Task{}, false
}

func TestNpmScriptsParsed(t *testing.T) {
	dir := writePackageJSON(t, `{
		"scripts": {
			"dev": "vite",
			"build": "vite build",
			"prebuild": "rimraf dist",
			"preview": "vite preview"
		}
	}`)

	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}

	// prebuild is a hook for build → dropped; preview has no "view" sibling → kept.
	if _, ok := taskByName(tasks, "prebuild"); ok {
		t.Error("prebuild lifecycle hook should be omitted")
	}
	if _, ok := taskByName(tasks, "preview"); !ok {
		t.Error("preview is a real script and should be kept")
	}

	dev, ok := taskByName(tasks, "dev")
	if !ok {
		t.Fatal("dev script not detected")
	}
	if got, want := dev.Command, []string{"npm", "run", "dev"}; !equalStrings(got, want) {
		t.Errorf("dev command = %v, want %v", got, want)
	}
	if dev.Runner != RunnerNpm || dev.Source != "package.json" {
		t.Errorf("unexpected runner/source: %q / %q", dev.Runner, dev.Source)
	}
}

func TestNpmPackageManagerDetection(t *testing.T) {
	t.Run("packageManager field wins", func(t *testing.T) {
		dir := writePackageJSON(t, `{"packageManager":"pnpm@9.0.0","scripts":{"x":"y"}}`)
		tasks, _ := Detect(dir, Options{})
		x, _ := taskByName(tasks, "x")
		if x.Command[0] != "pnpm" {
			t.Errorf("command[0] = %q, want pnpm", x.Command[0])
		}
	})

	t.Run("lockfile decides", func(t *testing.T) {
		dir := writePackageJSON(t, `{"scripts":{"x":"y"}}`)
		if err := os.WriteFile(filepath.Join(dir, "yarn.lock"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		tasks, _ := Detect(dir, Options{})
		x, _ := taskByName(tasks, "x")
		if x.Command[0] != "yarn" {
			t.Errorf("command[0] = %q, want yarn", x.Command[0])
		}
	})
}

func TestDisabledRunnerSkipped(t *testing.T) {
	dir := writePackageJSON(t, `{"scripts":{"x":"y"}}`)
	tasks, err := Detect(dir, Options{Disabled: []Runner{RunnerNpm}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected no tasks when npm disabled, got %d", len(tasks))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
