package taskrunner

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

// taskfileDetector enumerates tasks by invoking go-task's own listing
// (`task --list-all`) and parsing its `* name: description` lines, expanding any
// `(aliases: …)` into separate entries. It requires `task` (or `go-task`) on
// PATH; without it the runner contributes nothing.
type taskfileDetector struct{}

var taskfileNames = []string{"Taskfile.yml", "Taskfile.yaml", "taskfile.yml", "taskfile.yaml"}

func (taskfileDetector) Runner() Runner { return RunnerTask }

func (taskfileDetector) Available(dir string) bool {
	return anyFileExists(dir, taskfileNames...)
}

// taskListLine matches go-task's `--list-all` rows: "* name:   description".
var taskListLine = regexp.MustCompile(`^\*\s+([A-Za-z0-9._:-]+):\s*(.*)$`)

func (taskfileDetector) Tasks(dir string) ([]Task, error) {
	bin := goTaskCommand()
	if bin == "" {
		return nil, nil
	}
	out, err := captureInDir(dir, bin, "--list-all")
	if err != nil {
		return nil, err
	}

	source := firstExisting(dir, taskfileNames)
	mk := func(name, desc string) Task {
		return Task{
			Name:     name,
			Runner:   RunnerTask,
			Command:  []string{bin, name},
			Cwd:      dir,
			Source:   source,
			Desc:     desc,
			Runnable: true,
		}
	}

	var tasks []Task
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		m := taskListLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		name, desc := m[1], strings.TrimSpace(m[2])
		if i := strings.Index(desc, "(aliases:"); i >= 0 {
			aliases := strings.TrimSuffix(strings.TrimSpace(desc[i+len("(aliases:"):]), ")")
			desc = strings.TrimSpace(desc[:i])
			tasks = append(tasks, mk(name, desc))
			for _, alias := range strings.Split(aliases, ",") {
				if alias = strings.TrimSpace(alias); alias != "" {
					tasks = append(tasks, mk(alias, "Alias for "+name))
				}
			}
			continue
		}
		tasks = append(tasks, mk(name, desc))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sortByName(tasks)
	return tasks, nil
}

func goTaskCommand() string {
	for _, n := range []string{"go-task", "task"} {
		if onPath(n) {
			return n
		}
	}
	return ""
}
