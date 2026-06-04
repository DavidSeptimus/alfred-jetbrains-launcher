package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/state"
)

func TestKeywordFor(t *testing.T) {
	cases := []struct {
		product   string
		wt, roots bool
		want      string
	}{
		{"", false, false, "jb"},
		{"", true, false, "jb~"},
		{"", false, true, "jb+"},
		{"goland", false, false, "goland"},
		{"goland", false, true, "goland+"},
		{"idea", true, false, "idea~"},
	}
	for _, c := range cases {
		if got := keywordFor(c.product, c.wt, c.roots); got != c.want {
			t.Errorf("keywordFor(%q,%v,%v) = %q, want %q", c.product, c.wt, c.roots, got, c.want)
		}
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

func TestDefaultProjectRoots(t *testing.T) {
	home := t.TempDir()
	// On-disk casing differs from our title-cased candidate ("GolandProjects"):
	// the case-insensitive match must still find it. A non-convention dir and a
	// file are ignored.
	for _, name := range []string{"GoLandProjects", "IdeaProjects", "random-dir"} {
		if err := os.MkdirAll(filepath.Join(home, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "PyCharmProjects"), []byte("x"), 0o644); err != nil {
		t.Fatal(err) // a file, not a dir -> not a root
	}

	got := map[string]string{} // on-disk base name -> implied code
	for _, r := range defaultProjectRoots(home) {
		got[filepath.Base(r.Path)] = r.Code
	}
	// Real on-disk names are preserved (not our candidate casing), each tagged
	// with the IDE its folder implies.
	if got["GoLandProjects"] != "GO" {
		t.Errorf("want GoLandProjects implying GO, got %q", got["GoLandProjects"])
	}
	if got["IdeaProjects"] != "IU" {
		t.Errorf("want IdeaProjects implying IU, got %q", got["IdeaProjects"])
	}
	if _, ok := got["random-dir"]; ok {
		t.Error("non-convention dirs must not be seeded as roots")
	}
	if _, ok := got["PyCharmProjects"]; ok {
		t.Error("a file matching a convention name must not be seeded")
	}
}

func TestEmitSearchRootsGate(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "roots")
	proj := filepath.Join(root, "scan-demo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		Home:         tmp,
		CacheDir:     filepath.Join(tmp, "cache"),
		DataDir:      filepath.Join(tmp, "data"),
		ProjectRoots: []string{root},
	}
	for _, d := range []string{cfg.CacheDir, cfg.DataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	without := captureStdout(t, func() { emitSearch(cfg, "", "", false, false, "jb") })
	if strings.Contains(without, "scan-demo") {
		t.Errorf("plain search must hide un-opened dirs, but output mentions it:\n%s", without)
	}

	with := captureStdout(t, func() { emitSearch(cfg, "", "", false, true, "jb+") })
	if !strings.Contains(with, "scan-demo") {
		t.Errorf("--roots search must surface the un-opened dir, got:\n%s", with)
	}

	// A custom-root un-opened project (no implied IDE) is eligible under a per-IDE
	// `+` keyword too — the keyword drives which IDE opens it.
	perIDE := captureStdout(t, func() { emitSearch(cfg, "goland", "", false, true, "goland+") })
	if !strings.Contains(perIDE, "scan-demo") {
		t.Errorf("a custom-root un-opened project should appear under a per-IDE + keyword, got:\n%s", perIDE)
	}

	// Pinning an un-opened project promotes it into the plain (non-`+`) list,
	// ★-pinned, even though it's never been opened.
	st := state.Load(cfg.DataDir)
	st.TogglePinned(proj)
	if err := state.Save(cfg.DataDir, st); err != nil {
		t.Fatal(err)
	}
	pinned := captureStdout(t, func() { emitSearch(cfg, "", "", false, false, "jb") })
	if !strings.Contains(pinned, "★ scan-demo") {
		t.Errorf("a pinned un-opened project should show ★-pinned in plain jb, got:\n%s", pinned)
	}
}
