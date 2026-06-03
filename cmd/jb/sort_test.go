package main

import (
	"testing"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
)

func TestSortProjects(t *testing.T) {
	mk := func(name, path string, ms int64) recent.Project {
		return recent.Project{DisplayName: name, Path: path, Timestamp: time.UnixMilli(ms)}
	}
	// Names, recency and paths each sort to a different order, so every mode is
	// distinguishable. Input is in canonical recency-desc order (as Merge emits).
	base := []recent.Project{
		mk("Beta", "/u/1-beta", 300),
		mk("alpha", "/u/2-alpha", 200),
		mk("gamma", "/u/0-gamma", 100),
	}
	names := func(ps []recent.Project) []string {
		out := make([]string, len(ps))
		for i, p := range ps {
			out[i] = p.DisplayName
		}
		return out
	}

	cases := []struct {
		mode string
		want []string
	}{
		{"recency", []string{"Beta", "alpha", "gamma"}}, // unchanged
		{"", []string{"Beta", "alpha", "gamma"}},        // default == recency
		{"recency-asc", []string{"gamma", "alpha", "Beta"}},
		{"name", []string{"alpha", "Beta", "gamma"}},      // case-insensitive
		{"name-desc", []string{"gamma", "Beta", "alpha"}}, // reverse
		{"path", []string{"gamma", "Beta", "alpha"}},      // /u/0-,1-,2-
	}
	for _, c := range cases {
		ps := make([]recent.Project, len(base))
		copy(ps, base)
		sortProjects(ps, c.mode)
		got := names(ps)
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("mode %q: got %v, want %v", c.mode, got, c.want)
				break
			}
		}
	}
}
