package recent

import "testing"

func TestClassifyPath(t *testing.T) {
	home := "/home/test"
	cases := []struct {
		raw       string
		wantKind  Kind
		wantCanon string
	}{
		{"$USER_HOME$/IdeaProjects/alpha", KindHomeMacro, "/home/test/IdeaProjects/alpha"},
		{"$USER_HOME$/a/../b", KindHomeMacro, "/home/test/b"},
		{"/Users/dave/proj", KindLocal, "/Users/dave/proj"},
		{"/$devcontainer.ij/abc@x/IdeaProjects/p", KindRemote, ""},
		{"$APPLICATION_HOME_DIR$/bin", KindUnknown, ""},
	}
	for _, c := range cases {
		got := ClassifyPath(home, c.raw)
		if got.Kind != c.wantKind {
			t.Errorf("%q kind: want %v, got %v", c.raw, c.wantKind, got.Kind)
		}
		if got.Canonical != c.wantCanon {
			t.Errorf("%q canon: want %q, got %q", c.raw, c.wantCanon, got.Canonical)
		}
	}
}

func TestOpenable(t *testing.T) {
	if !(PathRef{Kind: KindLocal}).Openable() {
		t.Error("local should be openable")
	}
	if (PathRef{Kind: KindRemote}).Openable() {
		t.Error("remote should not be openable")
	}
}
