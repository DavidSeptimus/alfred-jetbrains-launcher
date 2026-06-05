package taskrunner

import "encoding/json"

// justDetector enumerates recipes by asking just itself for a structured dump
// (`just --dump --dump-format=json`), which yields recipe docs and submodules
// without us reimplementing justfile parsing. It requires `just` on PATH; with
// the tool absent we can't enumerate, so the runner contributes nothing (a
// best-effort line parse is possible future work).
type justDetector struct{}

var justfileNames = []string{"justfile", "Justfile", ".justfile"}

func (justDetector) Runner() Runner { return RunnerJust }

func (justDetector) Available(dir string) bool {
	return anyFileExists(dir, justfileNames...)
}

type justDump struct {
	Recipes map[string]justRecipe `json:"recipes"`
	Modules map[string]justDump   `json:"modules"`
}

type justRecipe struct {
	Doc string `json:"doc"`
}

func (justDetector) Tasks(dir string) ([]Task, error) {
	if !onPath("just") {
		return nil, nil
	}
	out, err := captureInDir(dir, "just", "--unstable", "--dump", "--dump-format=json")
	if err != nil {
		return nil, err
	}
	var dump justDump
	if err := json.Unmarshal(out, &dump); err != nil {
		return nil, err
	}

	source := firstExisting(dir, justfileNames)
	var tasks []Task
	add := func(name, doc string) {
		tasks = append(tasks, Task{
			Name:     name,
			Runner:   RunnerJust,
			Command:  []string{"just", name},
			Cwd:      dir,
			Source:   source,
			Desc:     doc,
			Runnable: true,
		})
	}
	for name, r := range dump.Recipes {
		add(name, r.Doc)
	}
	// Submodule recipes are addressable as `mod::recipe` (which `just` accepts as
	// the task argument), matching how task-keeper surfaces them.
	for mod, sub := range dump.Modules {
		for name, r := range sub.Recipes {
			add(mod+"::"+name, r.Doc)
		}
	}
	sortByName(tasks)
	return tasks, nil
}
