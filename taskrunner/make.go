package taskrunner

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// makeDetector parses targets directly from a Makefile. It is a pure line parse
// (no `make` invocation), so it doesn't resolve `include`d makefiles or targets
// synthesised by functions — the common, explicitly-written targets are what a
// launcher wants anyway.
type makeDetector struct{}

var makefileNames = []string{"Makefile", "makefile", "GNUmakefile"}

func (makeDetector) Runner() Runner { return RunnerMake }

func (makeDetector) Available(dir string) bool {
	return anyFileExists(dir, makefileNames...)
}

// makeTargetLine matches a rule line: one or more target names followed by a
// single ':' that is not part of a ':='/'::=' assignment. The first character
// must be alphanumeric, which skips indented recipe lines, comments, and
// '.'-prefixed special targets (.PHONY, .SUFFIXES, …).
var makeTargetLine = regexp.MustCompile(`^([A-Za-z0-9][^:=#]*):(?:[^=]|$)`)

func (makeDetector) Tasks(dir string) ([]Task, error) {
	name := firstExisting(dir, makefileNames)
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	runnable := onPath("make")
	seen := map[string]bool{}
	var tasks []Task
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := makeTargetLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		for _, target := range strings.Fields(m[1]) {
			// Pattern rules (%), variable refs ($), special/file targets.
			if strings.ContainsAny(target, "%$") || target == name || seen[target] {
				continue
			}
			seen[target] = true
			tasks = append(tasks, Task{
				Name:     target,
				Runner:   RunnerMake,
				Command:  []string{"make", target},
				Cwd:      dir,
				Source:   name,
				Runnable: runnable,
			})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sortByName(tasks)
	return tasks, nil
}
