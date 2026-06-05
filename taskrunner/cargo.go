package taskrunner

import "path/filepath"

// cargoDetector maps a Cargo workspace's common commands to fixed verbs.
type cargoDetector struct{}

func (cargoDetector) Runner() Runner { return RunnerCargo }

func (cargoDetector) Available(dir string) bool {
	return fileExists(filepath.Join(dir, "Cargo.toml"))
}

func (cargoDetector) Tasks(dir string) ([]Task, error) {
	verbs := []verb{
		{"build", []string{"build"}, "Compile the project"},
		{"release", []string{"build", "--release"}, "Compile with optimizations"},
		{"test", []string{"test"}, "Run the tests"},
		{"check", []string{"check"}, "Type-check without building"},
		{"clippy", []string{"clippy"}, "Lint with Clippy"},
		{"fmt", []string{"fmt"}, "Format the source"},
		{"doc", []string{"doc"}, "Build the documentation"},
		{"clean", []string{"clean"}, "Remove build artifacts"},
	}
	// `cargo run` needs a binary target; offer it when this crate has one.
	if fileExists(filepath.Join(dir, "src", "main.rs")) {
		verbs = append(verbs, verb{"run", []string{"run"}, "Run the binary"})
	}
	return fixedVerbTasks(dir, "Cargo.toml", "cargo", onPath("cargo"), RunnerCargo, verbs), nil
}
