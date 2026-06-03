package ide

import "testing"

func testInstalled() []Installed {
	return []Installed{
		{Code: "IU", Family: "idea", Version: "2026.1.2", Display: "IntelliJ IDEA", AppPath: "/A/IU261.app", DataDir: "IntelliJIdea2026.1"},
		{Code: "IU", Family: "idea", Version: "2025.1.7", Display: "IntelliJ IDEA", AppPath: "/A/IU251.app", DataDir: "IntelliJIdea2025.1"},
		{Code: "AI", Family: "studio", Version: "AI-261.1", Display: "Android Studio", AppPath: "/A/AS.app", DataDir: "AndroidStudio2026.1.1"},
	}
}

func TestResolveExactVersion(t *testing.T) {
	got, ok := Resolve(testInstalled(), "IU", "IntelliJIdea2025.1", "")
	if !ok || got.DataDir != "IntelliJIdea2025.1" {
		t.Errorf("should open in the recorded version 2025.1, got %q (ok=%v)", got.DataDir, ok)
	}
}

func TestResolveNewestSameCodeWhenRecordedVersionGone(t *testing.T) {
	// Recorded in 2026.2, which isn't installed -> newest IU (2026.1).
	got, ok := Resolve(testInstalled(), "IU", "IntelliJIdea2026.2", "")
	if !ok || got.DataDir != "IntelliJIdea2026.1" {
		t.Errorf("should fall back to newest IU 2026.1, got %q (ok=%v)", got.DataDir, ok)
	}
}

func TestResolveIDEAFirstClassFallback(t *testing.T) {
	// PyCharm community recorded, no pycharm installed -> IDEA Ultimate (python is first-class).
	got, ok := Resolve(testInstalled(), "PC", "PyCharmCE2025.1", "")
	if !ok || got.Code != "IU" {
		t.Errorf("pycharm project should fall back to IDEA Ultimate, got %q (ok=%v)", got.Code, ok)
	}
}

func TestResolveLastResort(t *testing.T) {
	// Rust isn't first-class in IDEA, RustRover not installed -> last resort newest IU.
	got, ok := Resolve(testInstalled(), "RR", "RustRover2025.3", "")
	if !ok || got.Code != "IU" {
		t.Errorf("rust project last-resort should be IDEA, got %q (ok=%v)", got.Code, ok)
	}
}

func TestResolveKeywordHardLimit(t *testing.T) {
	// Keyword studio must win even though the recorded code is IU.
	got, ok := Resolve(testInstalled(), "IU", "IntelliJIdea2026.1", "studio")
	if !ok || got.Family != "studio" {
		t.Errorf("keyword should hard-limit to studio, got family %q (ok=%v)", got.Family, ok)
	}
}

func TestResolveKeywordNotInstalledFallsBack(t *testing.T) {
	// goland keyword, no GoLand installed -> relax hard-limit; Go is IDEA
	// first-class, so fall back to IntelliJ IDEA Ultimate.
	got, ok := Resolve(testInstalled(), "GO", "GoLand2025.1", "goland")
	if !ok || got.Code != "IU" {
		t.Errorf("goland keyword w/o GoLand should fall back to IDEA, got %q (ok=%v)", got.Code, ok)
	}
}

func TestResolveNoneInstalled(t *testing.T) {
	if _, ok := Resolve(nil, "IU", "IntelliJIdea2026.1", ""); ok {
		t.Error("no IDEs installed should return ok=false")
	}
}
