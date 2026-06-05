package taskrunner

import "path/filepath"

// goDetector maps a Go module's common commands to fixed verbs.
type goDetector struct{}

func (goDetector) Runner() Runner { return RunnerGo }

func (goDetector) Available(dir string) bool {
	return fileExists(filepath.Join(dir, "go.mod"))
}

func (goDetector) Tasks(dir string) ([]Task, error) {
	verbs := []verb{
		{"build", []string{"build", "./..."}, "Compile all packages"},
		{"test", []string{"test", "./..."}, "Run all tests"},
		{"vet", []string{"vet", "./..."}, "Report suspicious constructs"},
		{"tidy", []string{"mod", "tidy"}, "Tidy go.mod / go.sum"},
	}
	// `go run .` only makes sense from a main package; offer it when this dir is one.
	if fileExists(filepath.Join(dir, "main.go")) {
		verbs = append(verbs, verb{"run", []string{"run", "."}, "Run the main package"})
	}
	return fixedVerbTasks(dir, "go.mod", "go", onPath("go"), RunnerGo, verbs), nil
}
