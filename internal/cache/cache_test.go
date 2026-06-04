package cache

import (
	"os"
	"testing"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
)

func TestFingerprintReflectsProjectRootMtime(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{ProjectRoots: []string{root}}

	roots := []string{root}
	old := time.Unix(1000, 0)
	if err := os.Chtimes(root, old, old); err != nil {
		t.Fatal(err)
	}
	fp1 := Fingerprint(cfg, nil, nil, roots)

	// Simulate a new subdir landing in the root (its mtime advances).
	newer := time.Unix(2000, 0)
	if err := os.Chtimes(root, newer, newer); err != nil {
		t.Fatal(err)
	}
	fp2 := Fingerprint(cfg, nil, nil, roots)

	if fp1 == fp2 {
		t.Fatal("fingerprint must change when a project-root mtime changes")
	}

	// With no project roots, the root mtime is irrelevant to the key.
	if Fingerprint(config.Config{}, nil, nil, nil) == fp2 {
		t.Fatal("project-root component should not appear when no roots are configured")
	}
}
