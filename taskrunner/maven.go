package taskrunner

import (
	"os"
	"path/filepath"
	"strings"
)

// mavenDetector maps a fixed set of canonical verbs to Maven command lines.
// Maven goals are plugin-defined with no clean way to enumerate "runnable"
// goals, so — unlike Gradle — a fixed verb map is the pragmatic choice. The
// `run` verb is specialised by sniffing the pom for a known framework.
type mavenDetector struct{}

func (mavenDetector) Runner() Runner { return RunnerMaven }

func (mavenDetector) Available(dir string) bool {
	return fileExists(filepath.Join(dir, "pom.xml"))
}

func (mavenDetector) Tasks(dir string) ([]Task, error) {
	cmd := mvnCommand(dir)
	runnable := mvnRunnable(dir)
	mk := func(name, desc string, args ...string) Task {
		return Task{
			Name: name, Runner: RunnerMaven,
			Command: append([]string{cmd}, args...), Cwd: dir,
			Source: "pom.xml", Desc: desc, Runnable: runnable,
		}
	}
	tasks := []Task{
		mk("clean", "Delete target/", "clean"),
		mk("compile", "Compile sources", "compile", "test-compile"),
		mk("build", "Package (skip tests)", "-DskipTests", "package"),
		mk("test", "Run tests", "test"),
		mk("verify", "Run checks and integration tests", "verify"),
		mk("deps", "Print dependency tree", "dependency:tree"),
		mk("run", "Run the application", mvnRunArgs(dir)...),
	}
	sortByName(tasks)
	return tasks, nil
}

// mvnCommand prefers the project's wrapper over a system mvn.
func mvnCommand(dir string) string {
	if fileExists(filepath.Join(dir, "mvnw")) {
		return "./mvnw"
	}
	return "mvn"
}

func mvnRunnable(dir string) bool {
	return fileExists(filepath.Join(dir, "mvnw")) || onPath("mvn")
}

// mvnRunArgs picks the run goal from the pom: Spring Boot and Quarkus have
// dedicated dev goals; otherwise fall back to exec:java.
func mvnRunArgs(dir string) []string {
	pom, _ := os.ReadFile(filepath.Join(dir, "pom.xml"))
	content := string(pom)
	switch {
	case strings.Contains(content, "spring-boot-starter-web"),
		strings.Contains(content, "spring-boot-starter-webflux"):
		return []string{"spring-boot:run"}
	case strings.Contains(content, "quarkus-maven-plugin"):
		return []string{"quarkus:dev"}
	default:
		return []string{"exec:java"}
	}
}
