package taskrunner

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// gradleDetector surfaces a project's Gradle tasks as an instant fixed-verb set
// (common verbs plus the run task inferred from the build file). This is the
// keystroke-safe path used for first paint.
//
// The richer list — real Gradle projects carry tasks a fixed list can't know
// about (runIde, buildPlugin, code-gen, release pipelines) — comes from
// [EnumerateGradle], which is slow (daemon + configuration phase) and so is
// produced in the background and cached by the caller.
type gradleDetector struct{}

var gradleMarkers = []string{
	"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts",
}

func (gradleDetector) Runner() Runner { return RunnerGradle }

func (gradleDetector) Available(dir string) bool {
	return anyFileExists(dir, gradleMarkers...)
}

func (gradleDetector) Tasks(dir string) ([]Task, error) {
	return gradleFixed(dir), nil
}

// EnumerateGradle runs `./gradlew tasks` and returns the project's real grouped
// tasks. It is slow and intended to be called in the background and cached. It
// returns an error when the Gradle invocation fails *or* when no tasks could be
// parsed — so a transient failure or unexpected output is never mistaken for an
// authoritative empty list (which would otherwise be cached, masking the real
// tasks).
func EnumerateGradle(dir string) ([]Task, error) {
	return gradleEnumerate(dir)
}

// gradleCommand prefers the project's wrapper over a system gradle.
func gradleCommand(dir string) string {
	if fileExists(filepath.Join(dir, "gradlew")) {
		return "./gradlew"
	}
	return "gradle"
}

func gradleRunnable(dir string) bool {
	return fileExists(filepath.Join(dir, "gradlew")) || onPath("gradle")
}

// gradleBuildFile is the build script used for the Source label.
func gradleBuildFile(dir string) string {
	if fileExists(filepath.Join(dir, "build.gradle.kts")) {
		return "build.gradle.kts"
	}
	return "build.gradle"
}

// gradleFixed is the instant set: common verbs plus the inferred run task.
func gradleFixed(dir string) []Task {
	cmd := gradleCommand(dir)
	runnable := gradleRunnable(dir)
	src := gradleBuildFile(dir)
	mk := func(name, task, desc string) Task {
		return Task{
			Name: name, Runner: RunnerGradle,
			Command: []string{cmd, task}, Cwd: dir,
			Source: src, Desc: desc, Runnable: runnable,
		}
	}
	tasks := []Task{
		mk("build", "build", "Assemble and test"),
		mk("assemble", "assemble", "Assemble outputs"),
		mk("test", "test", "Run tests"),
		mk("clean", "clean", "Delete build outputs"),
	}
	if run := gradleRunTaskName(dir); run != "" {
		tasks = append(tasks, mk(run, run, "Run the application"))
	}
	sortByName(tasks)
	return tasks
}

// gradleRunTaskName infers the project's "run" task from build-script contents,
// used to highlight it. The real task also appears via enumeration; this just
// lets the wrapper pin it (and gives the fixed set a run entry).
func gradleRunTaskName(dir string) string {
	content := readGradleBuildScripts(dir)
	switch {
	case strings.Contains(content, "org.jetbrains.intellij"):
		return "runIde"
	case strings.Contains(content, "org.springframework.boot"):
		return "bootRun"
	case strings.Contains(content, "io.quarkus"):
		return "quarkusDev"
	case strings.Contains(content, `id 'application'`),
		strings.Contains(content, `id("application")`),
		strings.Contains(content, "apply plugin: 'application'"):
		return "run"
	}
	return ""
}

func readGradleBuildScripts(dir string) string {
	var b strings.Builder
	for _, n := range []string{"build.gradle", "build.gradle.kts"} {
		if data, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
			b.Write(data)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// gradleTaskLine matches a single grouped-task row: a single-token task name
// optionally followed by " - description". Multi-word lines (titles, "BUILD
// SUCCESSFUL", group headers like "Build tasks") never match.
var gradleTaskLine = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9:_-]*)(?: - (.*))?$`)
var gradleUnderline = regexp.MustCompile(`^-{3,}$`)

// gradleEnumerate runs `./gradlew tasks` and parses the grouped task list.
// Tasks live in sections introduced by a "Group title" line followed by a
// dashed underline; rows run until the next blank line. The default `tasks`
// (not `--all`) lists only grouped/public tasks — where authored tasks like
// runIde live, without the per-subproject noise of `--all`.
func gradleEnumerate(dir string) ([]Task, error) {
	out, err := captureInDir(dir, gradleCommand(dir), "tasks", "--console=plain")
	if err != nil {
		return nil, err
	}
	cmd := gradleCommand(dir)
	src := gradleBuildFile(dir)

	var tasks []Task
	seen := map[string]bool{}
	inGroup := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t")
		switch {
		case gradleUnderline.MatchString(line):
			inGroup = true
			continue
		case line == "":
			inGroup = false
			continue
		}
		if !inGroup {
			continue
		}
		m := gradleTaskLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		tasks = append(tasks, Task{
			Name: name, Runner: RunnerGradle,
			Command: []string{cmd, name}, Cwd: dir,
			Source: src, Desc: strings.TrimSpace(m[2]), Runnable: true,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		// `gradle tasks` always lists at least the built-in help/build tasks, so an
		// empty parse means the invocation didn't really succeed (broken build,
		// unexpected output). Signal that rather than return an authoritative empty.
		return nil, fmt.Errorf("gradle: no tasks parsed from `gradle tasks` output")
	}
	sortByName(tasks)
	return tasks, nil
}

// GradleFingerprint hashes the Gradle build inputs whose change should
// invalidate a cached enumeration. It lets a caching caller decide freshness
// without duplicating knowledge of which files matter. Returns "" when dir is
// not a Gradle project.
func GradleFingerprint(dir string) string {
	if !anyFileExists(dir, gradleMarkers...) {
		return ""
	}
	inputs := []string{
		"settings.gradle", "settings.gradle.kts",
		"build.gradle", "build.gradle.kts",
		"gradle.properties", "gradle/libs.versions.toml",
		"gradle/wrapper/gradle-wrapper.properties",
	}
	h := sha256.New()
	for _, rel := range inputs {
		if info, err := os.Stat(filepath.Join(dir, rel)); err == nil {
			h.Write([]byte(rel))
			h.Write([]byte(info.ModTime().UTC().Format("20060102150405.000000000")))
		}
	}
	// buildSrc and included convention plugins can change tasks too; fold in the
	// buildSrc tree's newest source mtime — pruning build/.gradle outputs, which
	// `gradlew tasks` itself rewrites (otherwise the fingerprint would change on
	// every enumeration and the cache would never match).
	if newest := newestModTime(filepath.Join(dir, "buildSrc"), "build", ".gradle", ".idea"); !newest.IsZero() {
		h.Write([]byte("buildSrc"))
		h.Write([]byte(newest.UTC().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(h.Sum(nil))
}
