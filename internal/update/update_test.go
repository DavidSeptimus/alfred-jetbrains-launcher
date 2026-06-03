package update

import (
	"testing"
	"time"
)

func TestCacheStale(t *testing.T) {
	if !(Cache{}).Stale(24 * time.Hour) {
		t.Error("a never-checked cache should be stale")
	}
	old := Cache{CheckedAt: time.Now().Add(-48 * time.Hour).UnixMilli()}
	if !old.Stale(24 * time.Hour) {
		t.Error("a 48h-old check should be stale at 24h")
	}
	fresh := Cache{CheckedAt: time.Now().UnixMilli()}
	if fresh.Stale(24 * time.Hour) {
		t.Error("a just-now check should not be stale")
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"v0.2.0", "0.1.0", true},
		{"0.1.1", "0.1.0", true},
		{"v1.0.0", "0.9.9", true},
		{"v0.1.0", "0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.1.0", "0.1.0-rc1", true}, // release > its own pre-release
		{"v0.1.0", "dev", true},       // unparseable current treated as older
	}
	for _, c := range cases {
		if got := IsNewer(c.tag, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.tag, c.current, got, c.want)
		}
	}
}
