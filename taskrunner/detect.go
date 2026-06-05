package taskrunner

// Detector finds the tasks for a single runner within a directory.
type Detector interface {
	// Runner reports which runner this detector handles (for enable/disable).
	Runner() Runner
	// Available reports whether the runner's source file(s) exist in dir. It must
	// be cheap (file-existence checks only) — it gates the more expensive Tasks.
	Available(dir string) bool
	// Tasks returns the tasks detected in dir. It is only called when Available
	// returned true.
	Tasks(dir string) ([]Task, error)
}

// detectors is the ordered registry. Order defines the precedence used when the
// same task name appears in more than one runner: lower-index runners list
// first. Runners (project-defined task names) come before managers (canonical
// build verbs).
func detectors() []Detector {
	return []Detector{
		npmDetector{},
		makeDetector{},
		justDetector{},
		taskfileDetector{},
		composerDetector{},
		denoDetector{},
		rakeDetector{},
		gradleDetector{},
		mavenDetector{},
		cargoDetector{},
		goDetector{},
		dotnetDetector{},
	}
}

// Detect returns every task found in dir across all enabled runners.
//
// Runners listed in opts.Disabled are skipped before their detector runs. Task
// names that collide across runners are intentionally NOT deduplicated — a
// `build` from a Makefile and a `build` npm script are different commands;
// callers disambiguate them by [Task.Source].
//
// A failing detector does not abort the others: its error is recorded (the
// first such error is returned) while the tasks from the remaining runners are
// still returned. Callers that only want a best-effort list may ignore the
// error.
func Detect(dir string, opts Options) ([]Task, error) {
	var out []Task
	var firstErr error
	for _, d := range detectors() {
		if opts.isDisabled(d.Runner()) || !d.Available(dir) {
			continue
		}
		tasks, err := d.Tasks(dir)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, tasks...)
	}
	return out, firstErr
}
