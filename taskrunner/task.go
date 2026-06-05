// Package taskrunner detects runnable build-system tasks in a project directory
// — npm scripts, Make targets, justfile recipes, Taskfile tasks, and Gradle /
// Maven verbs — and resolves each to a runnable command.
//
// It is deliberately Alfred-agnostic: it knows nothing about the workflow, the
// terminal, or Script Filter JSON. Callers translate a [Task] into their own UI
// and decide how to launch its [Task.Command]. That boundary is what lets the
// package be developed inside the jb repo today and extracted to its own module
// later without change.
package taskrunner

import "slices"

// Runner identifies the build system or task runner a [Task] originates from.
// It is the unit of enabling/disabling (see [Options.Disabled]); the concrete
// executable (e.g. npm vs pnpm) lives in [Task.Command].
type Runner string

const (
	RunnerNpm      Runner = "npm"
	RunnerMake     Runner = "make"
	RunnerJust     Runner = "just"
	RunnerTask     Runner = "task"
	RunnerComposer Runner = "composer"
	RunnerDeno     Runner = "deno"
	RunnerRake     Runner = "rake"
	RunnerGradle   Runner = "gradle"
	RunnerMaven    Runner = "maven"
	RunnerCargo    Runner = "cargo"
	RunnerGo       Runner = "go"
	RunnerDotnet   Runner = "dotnet"
)

// Task is a single runnable task detected in a project directory.
type Task struct {
	Name     string   // user-facing task name, e.g. "dev", "runIde"
	Runner   Runner   // the system that defines and runs it
	Command  []string // resolved argv to run it, e.g. {"npm", "run", "dev"}
	Cwd      string   // working directory the command should run in
	Source   string   // file it was found in, e.g. "package.json" (UI subtitle)
	Desc     string   // optional human description, when the source supplies one
	Runnable bool     // whether the underlying tool is available to run it
}

// Options configures a [Detect] call.
type Options struct {
	// Disabled lists runners to skip entirely. A disabled runner's detector is
	// never invoked, so disabling is also a performance lever — notably for
	// Gradle (whose richer task list comes from the separate, slow
	// [EnumerateGradle]).
	Disabled []Runner
}

func (o Options) isDisabled(r Runner) bool {
	return slices.Contains(o.Disabled, r)
}
