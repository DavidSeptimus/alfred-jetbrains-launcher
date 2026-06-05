package taskrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// captureInDir runs name+args with its working directory set to dir and returns
// stdout. Detectors that enumerate by invoking the runner's own tooling (just,
// go-task, gradle) use it. stderr is not captured; a non-zero exit returns an
// error.
func captureInDir(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Output()
}

// fileExists reports whether p exists (file or directory).
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// anyFileExists reports whether any of names exists directly under dir.
func anyFileExists(dir string, names ...string) bool {
	for _, n := range names {
		if fileExists(filepath.Join(dir, n)) {
			return true
		}
	}
	return false
}

// firstExisting returns the first name in names that exists under dir, or the
// first element as a fallback when none do.
func firstExisting(dir string, names []string) string {
	for _, n := range names {
		if fileExists(filepath.Join(dir, n)) {
			return n
		}
	}
	return names[0]
}

// onPath reports whether cmd resolves on the current PATH. Detectors use it to
// set [Task.Runnable]; for a project-local wrapper (e.g. ./gradlew) check file
// existence instead.
func onPath(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// sortByName orders tasks by name, case-sensitively and stably, so a runner's
// output is deterministic.
func sortByName(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Name < tasks[j].Name })
}

// verb is one canonical task of a fixed-verb manager (Go, Cargo, dotnet, Maven):
// a display name, the argv tail appended to the base command, and a description.
type verb struct {
	name string
	args []string
	desc string
}

// fixedVerbTasks materialises a fixed-verb manager's tasks. cmd is the base
// executable (e.g. "go", "cargo"); each verb's args are appended to it.
func fixedVerbTasks(dir, source, cmd string, runnable bool, runner Runner, verbs []verb) []Task {
	tasks := make([]Task, 0, len(verbs))
	for _, v := range verbs {
		tasks = append(tasks, Task{
			Name:     v.name,
			Runner:   runner,
			Command:  append([]string{cmd}, v.args...),
			Cwd:      dir,
			Source:   source,
			Desc:     v.desc,
			Runnable: runnable,
		})
	}
	sortByName(tasks)
	return tasks
}

// newestModTime returns the most recent modification time under root (walking
// recursively), skipping any directory whose base name is in skipDirs, or the
// zero time if root does not exist. Walk errors are ignored so a
// partially-readable tree still yields a best-effort value.
//
// skipDirs matters for fingerprinting: build-output directories (build,
// .gradle) are rewritten by the very command whose output we cache, so
// including them would make the fingerprint change every run and the cache never
// match. Pruning them keeps the fingerprint to source inputs.
func newestModTime(root string, skipDirs ...string) time.Time {
	skip := make(map[string]bool, len(skipDirs))
	for _, s := range skipDirs {
		skip[s] = true
	}
	var newest time.Time
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && p != root && skip[d.Name()] {
			return filepath.SkipDir
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}
