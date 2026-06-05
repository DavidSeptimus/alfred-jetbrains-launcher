package taskrunner

import (
	"os"
	"path/filepath"
	"strings"
)

// dotnetDetector maps a .NET project/solution's common commands to fixed verbs.
type dotnetDetector struct{}

func (dotnetDetector) Runner() Runner { return RunnerDotnet }

func (dotnetDetector) Available(dir string) bool {
	return dotnetProjectFile(dir) != ""
}

func (dotnetDetector) Tasks(dir string) ([]Task, error) {
	src := dotnetProjectFile(dir)
	verbs := []verb{
		{"build", []string{"build"}, "Build the project"},
		{"run", []string{"run"}, "Run the project"},
		{"test", []string{"test"}, "Run the tests"},
		{"restore", []string{"restore"}, "Restore dependencies"},
		{"publish", []string{"publish"}, "Publish the application"},
		{"clean", []string{"clean"}, "Clean build outputs"},
	}
	return fixedVerbTasks(dir, src, "dotnet", onPath("dotnet"), RunnerDotnet, verbs), nil
}

// dotnetProjectFile returns the name of a .sln or .csproj/.fsproj/.vbproj in
// dir (preferring a solution), or "" when none exists.
func dotnetProjectFile(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var proj string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch ext := strings.ToLower(filepath.Ext(e.Name())); ext {
		case ".sln":
			return e.Name() // a solution wins outright
		case ".csproj", ".fsproj", ".vbproj":
			if proj == "" {
				proj = e.Name()
			}
		}
	}
	return proj
}
