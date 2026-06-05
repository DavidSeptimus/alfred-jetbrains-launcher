package taskrunner

import (
	"bufio"
	"bytes"
	"regexp"
)

// rakeDetector enumerates Rake tasks by invoking `rake -T` (described tasks) and
// parsing its output. It requires `rake` on PATH; without it the runner
// contributes nothing.
type rakeDetector struct{}

var rakefileNames = []string{"Rakefile", "rakefile", "Rakefile.rb", "rakefile.rb"}

func (rakeDetector) Runner() Runner { return RunnerRake }

func (rakeDetector) Available(dir string) bool {
	return anyFileExists(dir, rakefileNames...)
}

// rakeListLine matches a `rake -T` row: "rake name  # description".
var rakeListLine = regexp.MustCompile(`^rake\s+(\S+)\s*(?:#\s*(.*))?$`)

func (rakeDetector) Tasks(dir string) ([]Task, error) {
	if !onPath("rake") {
		return nil, nil
	}
	out, err := captureInDir(dir, "rake", "-T")
	if err != nil {
		return nil, err
	}
	source := firstExisting(dir, rakefileNames)

	var tasks []Task
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		m := rakeListLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		tasks = append(tasks, Task{
			Name:     m[1],
			Runner:   RunnerRake,
			Command:  []string{"rake", m[1]},
			Cwd:      dir,
			Source:   source,
			Desc:     m[2],
			Runnable: true,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sortByName(tasks)
	return tasks, nil
}
