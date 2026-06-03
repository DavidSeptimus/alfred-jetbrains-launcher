package recent

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Project is a deduplicated recent project, merged across every version dir and
// IDE that ever opened it.
type Project struct {
	Path           string    // canonical absolute path
	DisplayName    string    // folder name (title shown in Alfred)
	ProductionCode string    // production code of the most-recent (winning) entry
	SourceDataDir  string    // version config dir of the winning entry (e.g. IntelliJIdea2026.1)
	Timestamp      time.Time // most-recent activation/open across all entries
	Exists         bool      // the directory still exists on disk
	IsWorktree     bool      // the directory is a linked git worktree
	Stub           bool      // the directory has no real content (only hidden / ignored entries)
	AllCodes       []string  // every distinct production code that opened this path
}

// Merge collapses raw entries from all files into one sorted, deduplicated list.
// On a path collision the entry with the larger timestamp wins (its IDE/version
// becomes the default), while AllCodes accumulates every IDE that opened it.
// ignoreContent are entry-name globs treated as non-content for stub detection.
func Merge(entries []RawEntry, ignoreContent []string) []Project {
	byPath := map[string]*Project{}
	codeSets := map[string]map[string]bool{}

	for _, e := range entries {
		if !e.Ref.Openable() || e.Ref.Canonical == "" {
			continue // exclude remote/devcontainer/unknown keys in v1
		}
		key := e.Ref.Canonical

		if codeSets[key] == nil {
			codeSets[key] = map[string]bool{}
		}
		if e.ProductionCode != "" {
			codeSets[key][e.ProductionCode] = true
		}

		cur := byPath[key]
		if cur == nil {
			byPath[key] = &Project{
				Path:           key,
				DisplayName:    filepath.Base(key),
				ProductionCode: e.ProductionCode,
				SourceDataDir:  e.SourceDataDir,
				Timestamp:      e.Timestamp,
			}
			continue
		}
		if e.Timestamp.After(cur.Timestamp) {
			cur.Timestamp = e.Timestamp
			if e.ProductionCode != "" {
				cur.ProductionCode = e.ProductionCode
				cur.SourceDataDir = e.SourceDataDir
			}
		}
	}

	out := make([]Project, 0, len(byPath))
	for key, p := range byPath {
		p.AllCodes = sortedKeys(codeSets[key])
		if info, err := os.Lstat(key); err == nil && info.IsDir() {
			p.Exists = true
			p.IsWorktree = isWorktree(key)
			p.Stub = isStub(key, ignoreContent)
		}
		out = append(out, *p)
	}

	// Most-recent first; existing projects before missing ones; stable by path.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Exists != out[j].Exists {
			return out[i].Exists
		}
		if !out[i].Timestamp.Equal(out[j].Timestamp) {
			return out[i].Timestamp.After(out[j].Timestamp)
		}
		return out[i].Path < out[j].Path
	})

	return out
}

// ProjectFromPath builds a Project for a bare directory path — used to surface a
// pinned project that has aged out of every IDE's capped recents list, so a pin
// keeps it visible regardless of eviction. It returns ok=false when the path is
// not an existing directory (a pin whose folder is gone). ProductionCode is
// unknown, so the launcher resolves the IDE via its fallback chain; the
// directory's mtime is used as the timestamp for ordering.
func ProjectFromPath(path string, ignoreContent []string) (Project, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return Project{}, false
	}
	return Project{
		Path:        path,
		DisplayName: filepath.Base(path),
		Timestamp:   info.ModTime(),
		Exists:      true,
		IsWorktree:  isWorktree(path),
		Stub:        isStub(path, ignoreContent),
	}, true
}

// isWorktree reports whether dir is a linked git worktree: its ".git" is a file
// (not a directory) whose gitdir pointer references a ".../worktrees/<name>"
// path. This distinguishes worktrees from normal repos (.git is a dir) and from
// submodules (gitdir points at ".../modules/<name>").
func isWorktree(dir string) bool {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil || info.IsDir() {
		return false
	}
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "/worktrees/")
}

// GitBranch returns the current branch (or a short SHA when detached) for the
// repo or worktree at dir, or "" if it isn't a git checkout. It reads
// ".git"/HEAD directly — no git process — so it is cheap to call per result.
func GitBranch(dir string) string {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return ""
	}
	gitDir := gitPath
	if !info.IsDir() {
		// Worktree/submodule: ".git" is a file "gitdir: <path>".
		data, err := os.ReadFile(gitPath)
		if err != nil {
			return ""
		}
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
		if rest == "" {
			return ""
		}
		if !filepath.IsAbs(rest) {
			rest = filepath.Join(dir, rest)
		}
		gitDir = rest
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(head))
	if b := strings.TrimPrefix(s, "ref: refs/heads/"); b != s {
		return b
	}
	if len(s) >= 7 { // detached HEAD -> short SHA
		return s[:7]
	}
	return ""
}

// isStub reports whether dir has no real content — it is empty or every entry
// is either hidden (".something") or matches an ignoreContent glob (build, dist,
// node_modules, …). Such dirs are leftover stubs (e.g. a removed worktree, or a
// partially-deleted project), not real projects, so they are hidden.
func isStub(dir string, ignoreContent []string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || matchesGlob(name, ignoreContent) {
			continue // hidden or ignored content
		}
		return false // real content -> a real project
	}
	return true
}

// matchesGlob reports whether name matches any of the filepath.Match patterns.
func matchesGlob(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
