package ide

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
)

func writeApp(t *testing.T, dir, appName, code, version, dataDir string) string {
	t.Helper()
	app := filepath.Join(dir, appName)
	res := filepath.Join(app, "Contents", "Resources")
	if err := os.MkdirAll(res, 0o755); err != nil {
		t.Fatal(err)
	}
	pi := fmt.Sprintf(`{"name":%q,"version":%q,"productCode":%q,"dataDirectoryName":%q}`,
		appName, version, code, dataDir)
	if err := os.WriteFile(filepath.Join(res, "product-info.json"), []byte(pi), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestDetect(t *testing.T) {
	apps := t.TempDir()
	elsewhere := t.TempDir()
	scripts := t.TempDir()

	writeApp(t, apps, "IntelliJ IDEA.app", "IU", "2026.1.2", "IntelliJIdea2026.1")
	writeApp(t, apps, "IntelliJ IDEA 2025.1.7.app", "IU", "2025.1.7", "IntelliJIdea2025.1")
	writeApp(t, apps, "Gateway.app", "GW", "2026.1", "JetBrainsGateway2026.1") // not a classic IDE -> excluded

	// A Toolbox script pointing at a CLion bundle outside the app roots.
	clion := writeApp(t, elsewhere, "CLion.app", "CL", "2026.1", "CLion2026.1")
	script := fmt.Sprintf("#!/bin/bash\nopen -na \"%s/Contents/MacOS/clion\" $wait --args \"$@\"\n", clion)
	if err := os.WriteFile(filepath.Join(scripts, "clion"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{AppRoots: []string{apps}, ToolboxDirs: []string{scripts}}
	installed := Detect(cfg)

	byApp := map[string]Installed{}
	for _, i := range installed {
		byApp[filepath.Base(i.AppPath)] = i
	}
	if _, ok := byApp["Gateway.app"]; ok {
		t.Error("Gateway (GW) should be excluded in v1")
	}
	if len(installed) != 3 {
		t.Fatalf("want 3 IDEs (2x IU + CLion via toolbox), got %d: %+v", len(installed), installed)
	}
	if byApp["CLion.app"].Code != "CL" {
		t.Error("CLion should be discovered via toolbox script")
	}

	newest, ok := NewestByCode(installed, "IU")
	if !ok || newest.Version != "2026.1.2" {
		t.Errorf("NewestByCode IU should be 2026.1.2, got %q (ok=%v)", newest.Version, ok)
	}
}

func TestCmpVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2026.2", "2026.1", 1},
		{"2026.1", "2026.2", -1},
		{"2025.1.7", "2025.1", 1},
		{"2026.1", "2026.1", 0},
		{"AI-261.23567.1", "AI-252.100.0", 1}, // Android Studio non-standard form
		{"AI-261.23567.1", "AI-261.23567.1", 0},
	}
	for _, c := range cases {
		if got := sign(cmpVersion(c.a, c.b)); got != c.want {
			t.Errorf("cmpVersion(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}
