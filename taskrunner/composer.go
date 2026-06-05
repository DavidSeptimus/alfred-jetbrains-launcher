package taskrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// composerDetector reads the `scripts` map from composer.json, much like npm.
// Composer event hooks (pre-*/post-*) run automatically around their event and
// aren't independently interesting, so they're omitted.
type composerDetector struct{}

func (composerDetector) Runner() Runner { return RunnerComposer }

func (composerDetector) Available(dir string) bool {
	return fileExists(filepath.Join(dir, "composer.json"))
}

func (composerDetector) Tasks(dir string) ([]Task, error) {
	data, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		return nil, err
	}
	// A composer script value is either a string or an array of strings.
	var doc struct {
		Scripts map[string]json.RawMessage `json:"scripts"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	runnable := onPath("composer")

	tasks := make([]Task, 0, len(doc.Scripts))
	for name, raw := range doc.Scripts {
		if strings.HasPrefix(name, "pre-") || strings.HasPrefix(name, "post-") {
			continue
		}
		tasks = append(tasks, Task{
			Name:     name,
			Runner:   RunnerComposer,
			Command:  []string{"composer", "run-script", name},
			Cwd:      dir,
			Source:   "composer.json",
			Desc:     scriptDesc(raw),
			Runnable: runnable,
		})
	}
	sortByName(tasks)
	return tasks, nil
}

// scriptDesc renders a string or string-array script value as a one-line
// description.
func scriptDesc(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return strings.Join(list, " && ")
	}
	return ""
}
