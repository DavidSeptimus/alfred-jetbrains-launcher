package taskrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMavenFixedVerbs(t *testing.T) {
	dir := t.TempDir()
	pom := `<project><dependencies>
		<dependency><artifactId>spring-boot-starter-web</artifactId></dependency>
	</dependencies></project>`
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0o644); err != nil {
		t.Fatal(err)
	}
	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"clean", "build", "test", "run"} {
		if _, ok := taskByName(tasks, want); !ok {
			t.Errorf("expected maven verb %q", want)
		}
	}
	run, _ := taskByName(tasks, "run")
	if !equalStrings(run.Command, []string{"mvn", "spring-boot:run"}) {
		t.Errorf("spring-boot run command = %v", run.Command)
	}
}

func TestMavenWrapperPreferred(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "mvnw"), []byte("#!/bin/sh\n"), 0o755))
	tasks, _ := Detect(dir, Options{})
	build, _ := taskByName(tasks, "build")
	if build.Command[0] != "./mvnw" {
		t.Errorf("expected ./mvnw, got %q", build.Command[0])
	}
}

func TestGradleFixedAndRunSniff(t *testing.T) {
	dir := t.TempDir()
	build := `plugins { id("org.jetbrains.intellij.platform") version "2.0.0" }`
	must(t, os.WriteFile(filepath.Join(dir, "build.gradle.kts"), []byte(build), 0o644))

	// Default (no enumeration): fixed verbs + sniffed run task, no gradle call.
	tasks, err := Detect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := taskByName(tasks, "build"); !ok {
		t.Error("expected fixed gradle build verb")
	}
	if _, ok := taskByName(tasks, "runIde"); !ok {
		t.Error("expected runIde to be inferred for an IntelliJ Platform project")
	}
	for _, tk := range tasks {
		if tk.Runner == RunnerGradle && tk.Source != "build.gradle.kts" {
			t.Errorf("unexpected source %q", tk.Source)
		}
	}
}

func TestGradleFingerprintChangesWithBuildFile(t *testing.T) {
	dir := t.TempDir()
	bf := filepath.Join(dir, "build.gradle")
	must(t, os.WriteFile(bf, []byte("// v1"), 0o644))
	fp1 := GradleFingerprint(dir)
	if fp1 == "" {
		t.Fatal("expected non-empty fingerprint for a gradle project")
	}
	// A different mtime must change the fingerprint.
	future := mustStat(t, bf).ModTime().Add(2 * 1e9)
	must(t, os.Chtimes(bf, future, future))
	if GradleFingerprint(dir) == fp1 {
		t.Error("fingerprint should change when a build input changes")
	}
	if GradleFingerprint(t.TempDir()) != "" {
		t.Error("non-gradle dir should yield empty fingerprint")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustStat(t *testing.T, p string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
