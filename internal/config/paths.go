// Package config resolves the filesystem locations the workflow scans, with
// environment-variable overrides so the workflow works on non-default layouts.
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Root is a directory that holds per-version IDE config directories.
type Root struct {
	Dir    string
	Vendor string // "JetBrains" or "Google"
}

// Config captures every path the binary needs, after applying overrides.
type Config struct {
	Home             string
	ConfigRoots      []Root
	AppRoots         []string
	ToolboxDirs      []string
	CacheDir         string
	DataDir          string   // durable per-workflow state (pins, hidden list)
	ExcludeWorktrees bool     // hide linked git worktrees from results (default true)
	Terminal         string   // app name used by the "open in terminal" action
	IgnoreContent    []string // entry-name globs treated as non-content for stub detection
	IgnoreProjects   []string // path/name globs; matching projects are hidden
	Sort             string   // result order: recency|recency-asc|name|name-desc|path
}

// Environment variables (settable as Alfred workflow variables) that override
// the built-in defaults. Each is an os.PathListSeparator-separated list where
// it makes sense.
const (
	envConfigRoots = "JB_CONFIG_ROOTS" // colon-separated dirs (assumed JetBrains vendor unless they contain "Google")
	envAppRoots    = "JB_APP_ROOTS"    // colon-separated dirs holding *.app bundles
	envToolbox     = "JB_TOOLBOX_DIR"  // dir holding Toolbox launcher scripts
	envCacheDir    = "JB_CACHE_DIR"    // overrides the cache directory
	envAlfredCache = "alfred_workflow_cache"
	envExcludeWT   = "JB_EXCLUDE_WORKTREES" // "0"/"false" to include git worktrees
	envTerminal    = "JB_TERMINAL"          // app name for the "open in terminal" action
	envAlfredData  = "alfred_workflow_data"
	envDataDir     = "JB_DATA_DIR"       // overrides the durable data directory
	envIgnoreCont  = "JB_IGNORE_CONTENT" // comma-separated entry-name globs
	envIgnoreProj  = "JB_IGNORE_PROJECTS"
	envSort        = "JB_SORT" // result order; defaults to recency (newest first)
)

// normalizeSort validates a JB_SORT value, defaulting to "recency" (the most
// recently used first) for empty or unrecognised input.
func normalizeSort(v string) string {
	switch s := strings.ToLower(strings.TrimSpace(v)); s {
	case "recency-asc", "name", "name-desc", "path":
		return s
	default:
		return "recency"
	}
}

// defaultIgnoreContent are directories that often linger after a project is
// partly deleted; a dir containing only these (and hidden files) is a stub.
var defaultIgnoreContent = []string{"build", "dist", "node_modules"}

// parseList splits a comma-separated env var into trimmed entries, returning def
// when the variable is unset (an explicitly empty value yields no entries).
func parseList(env string, def []string) []string {
	v, ok := os.LookupEnv(env)
	if !ok {
		return def
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Load builds a Config from the environment and the current user's home dir.
func Load() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}

	cfg := Config{Home: home}

	// Each list comes from its env var (a colon-separated path list that replaces
	// the defaults). The Configure-Workflow fields are pre-populated with the
	// defaults so the values are always visible and editable; an empty/unset var
	// falls back to the same defaults (e.g. when run outside Alfred).
	if dirs := splitPaths(home, os.Getenv(envConfigRoots)); len(dirs) > 0 {
		for _, d := range dirs {
			vendor := "JetBrains"
			if strings.Contains(d, "Google") {
				vendor = "Google"
			}
			cfg.ConfigRoots = append(cfg.ConfigRoots, Root{Dir: d, Vendor: vendor})
		}
	} else {
		cfg.ConfigRoots = []Root{
			{Dir: filepath.Join(home, "Library", "Application Support", "JetBrains"), Vendor: "JetBrains"},
			{Dir: filepath.Join(home, "Library", "Application Support", "Google"), Vendor: "Google"},
		}
	}

	if dirs := splitPaths(home, os.Getenv(envAppRoots)); len(dirs) > 0 {
		cfg.AppRoots = dirs
	} else {
		cfg.AppRoots = []string{"/Applications", filepath.Join(home, "Applications")}
	}

	if dirs := splitPaths(home, os.Getenv(envToolbox)); len(dirs) > 0 {
		cfg.ToolboxDirs = dirs
	} else {
		cfg.ToolboxDirs = []string{filepath.Join(home, "Library", "Application Support", "JetBrains", "Toolbox", "scripts")}
	}

	switch {
	case os.Getenv(envCacheDir) != "":
		cfg.CacheDir = expandHome(home, os.Getenv(envCacheDir))
	case os.Getenv(envAlfredCache) != "":
		cfg.CacheDir = os.Getenv(envAlfredCache)
	default:
		cfg.CacheDir = os.TempDir()
	}

	// Worktrees are excluded by default; the Configure-Workflow checkbox sends
	// "0" (or "false") to include them.
	cfg.ExcludeWorktrees = true
	if v := os.Getenv(envExcludeWT); v == "0" || strings.EqualFold(v, "false") {
		cfg.ExcludeWorktrees = false
	}

	cfg.Terminal = os.Getenv(envTerminal)
	if cfg.Terminal == "" {
		cfg.Terminal = "Terminal"
	}

	cfg.IgnoreContent = parseList(envIgnoreCont, defaultIgnoreContent)
	cfg.IgnoreProjects = parseList(envIgnoreProj, nil)
	cfg.Sort = normalizeSort(os.Getenv(envSort))

	switch {
	case os.Getenv(envDataDir) != "":
		cfg.DataDir = expandHome(home, os.Getenv(envDataDir))
	case os.Getenv(envAlfredData) != "":
		cfg.DataDir = os.Getenv(envAlfredData)
	default:
		cfg.DataDir = filepath.Join(home, "Library", "Application Support", "jb-alfred")
	}

	return cfg
}

func splitPaths(home, v string) []string {
	parts := strings.Split(v, string(os.PathListSeparator))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, expandHome(home, p))
	}
	return out
}

func expandHome(home, p string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}
