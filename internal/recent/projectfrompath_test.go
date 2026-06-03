package recent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectFromPath(t *testing.T) {
	dir := t.TempDir()

	// A real directory with content.
	real := filepath.Join(dir, "real-proj")
	if err := os.MkdirAll(filepath.Join(real, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, ok := ProjectFromPath(real, nil)
	if !ok {
		t.Fatalf("existing dir: ok=false, want true")
	}
	if p.Path != real || p.DisplayName != "real-proj" || !p.Exists || p.Stub {
		t.Errorf("unexpected project: %+v", p)
	}

	// A stub directory (only ignored content) is flagged so the caller can hide it.
	stub := filepath.Join(dir, "stub-proj")
	if err := os.MkdirAll(filepath.Join(stub, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if p, ok := ProjectFromPath(stub, []string{"node_modules"}); !ok || !p.Stub {
		t.Errorf("stub: ok=%v stub=%v, want ok=true stub=true", ok, p.Stub)
	}

	// A pin whose folder is gone returns ok=false.
	if _, ok := ProjectFromPath(filepath.Join(dir, "missing"), nil); ok {
		t.Errorf("missing dir: ok=true, want false")
	}
}
