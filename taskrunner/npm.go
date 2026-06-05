package taskrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// npmDetector reads the `scripts` map from package.json. It resolves the package
// manager (npm / pnpm / yarn / bun) so the run command matches the project's
// lockfile, and omits npm's pre*/post* lifecycle hooks, which run automatically
// around their base script and aren't independently interesting to launch.
type npmDetector struct{}

func (npmDetector) Runner() Runner { return RunnerNpm }

func (npmDetector) Available(dir string) bool {
	return fileExists(filepath.Join(dir, "package.json"))
}

type packageJSON struct {
	Scripts        map[string]string `json:"scripts"`
	PackageManager string            `json:"packageManager"`
}

func (npmDetector) Tasks(dir string) ([]Task, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	var pj packageJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return nil, err
	}
	pm := npmCommand(dir, pj)
	runnable := onPath(pm)

	tasks := make([]Task, 0, len(pj.Scripts))
	for name, script := range pj.Scripts {
		if isLifecycleHook(name, pj.Scripts) {
			continue
		}
		tasks = append(tasks, Task{
			Name:     name,
			Runner:   RunnerNpm,
			Command:  []string{pm, "run", name},
			Cwd:      dir,
			Source:   "package.json",
			Desc:     script,
			Runnable: runnable,
		})
	}
	sortByName(tasks)
	return tasks, nil
}

// npmCommand picks the package manager: the explicit `packageManager` field
// wins, otherwise the lockfile present on disk decides, defaulting to npm.
func npmCommand(dir string, pj packageJSON) string {
	if pm := pj.PackageManager; pm != "" {
		switch {
		case strings.HasPrefix(pm, "yarn"):
			return "yarn"
		case strings.HasPrefix(pm, "pnpm"):
			return "pnpm"
		case strings.HasPrefix(pm, "bun"):
			return "bun"
		default:
			return "npm"
		}
	}
	switch {
	case anyFileExists(dir, "bun.lockb", "bun.lock"):
		return "bun"
	case anyFileExists(dir, "pnpm-lock.yaml"):
		return "pnpm"
	case anyFileExists(dir, "yarn.lock"):
		return "yarn"
	}
	return "npm"
}

// isLifecycleHook reports whether name is a pre*/post* hook for another script.
// Unlike a blanket "starts with pre/post" rule, it only treats name as a hook
// when the base script actually exists, so genuine scripts like "preview" or a
// standalone "postcss" survive.
func isLifecycleHook(name string, scripts map[string]string) bool {
	for _, prefix := range []string{"pre", "post"} {
		if base := strings.TrimPrefix(name, prefix); base != name && base != "" {
			if _, ok := scripts[base]; ok {
				return true
			}
		}
	}
	return false
}
