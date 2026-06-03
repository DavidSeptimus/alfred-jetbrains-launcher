package ide

import "testing"

func TestPreferRunning(t *testing.T) {
	installed := []Installed{
		{Code: "IU", Version: "2026.1.2", AppPath: "/A/IU261.app"},
		{Code: "IU", Version: "2025.1.7", AppPath: "/A/IU251.app"},
		{Code: "AI", Version: "AI-261", AppPath: "/A/AS.app"},
	}
	target := installed[0] // resolved to IU 2026.1

	orig := psCommand
	defer func() { psCommand = orig }()
	stub := func(s string) { psCommand = func() ([]byte, error) { return []byte(s), nil } }

	// A different version of the same product is running -> swap to it.
	stub("/A/IU251.app/Contents/MacOS/idea\n/usr/bin/other\n")
	if got := PreferRunning(installed, target); got.AppPath != "/A/IU251.app" {
		t.Errorf("should reuse running IU 2025.1, got %s", got.AppPath)
	}

	// The resolved target itself is running -> unchanged.
	stub("/A/IU261.app/Contents/MacOS/idea\n")
	if got := PreferRunning(installed, target); got.AppPath != "/A/IU261.app" {
		t.Errorf("running target should be kept, got %s", got.AppPath)
	}

	// Nothing of that product running -> unchanged.
	stub("/Applications/Safari.app/Contents/MacOS/Safari\n")
	if got := PreferRunning(installed, target); got.AppPath != "/A/IU261.app" {
		t.Errorf("no running same-product should keep target, got %s", got.AppPath)
	}

	// A different product running must not hijack.
	stub("/A/AS.app/Contents/MacOS/studio\n")
	if got := PreferRunning(installed, target); got.AppPath != "/A/IU261.app" {
		t.Errorf("different product running should not swap, got %s", got.AppPath)
	}
}
