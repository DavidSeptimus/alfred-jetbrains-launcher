package recent

import (
	"path/filepath"
	"strings"
)

// Kind classifies a recent-project key so we can decide whether it points at a
// real local directory we can open, or at something we must exclude (v1).
type Kind int

const (
	KindLocal     Kind = iota // an absolute local path, e.g. /Users/dave/proj
	KindHomeMacro             // a $USER_HOME$-rooted path
	KindRemote                // a remote/devcontainer virtual path, e.g. /$devcontainer.ij/...
	KindUnknown               // anything we don't recognise
)

const userHomeMacro = "$USER_HOME$"

// PathRef is the canonical identity of a recent-project entry. Canonical is the
// expanded, cleaned absolute path used for dedupe and launching; it is only
// meaningful when Kind is KindLocal or KindHomeMacro.
type PathRef struct {
	Raw       string
	Canonical string
	Kind      Kind
}

// ClassifyPath turns a raw recentProjects.xml entry key into a PathRef.
func ClassifyPath(home, raw string) PathRef {
	ref := PathRef{Raw: raw}

	switch {
	case strings.HasPrefix(raw, userHomeMacro):
		ref.Kind = KindHomeMacro
		rest := strings.TrimPrefix(raw, userHomeMacro)
		ref.Canonical = filepath.Clean(filepath.Join(home, rest))
	case isRemote(raw):
		ref.Kind = KindRemote
	case strings.HasPrefix(raw, "/") && !strings.Contains(raw, "$"):
		ref.Kind = KindLocal
		ref.Canonical = filepath.Clean(raw)
	default:
		ref.Kind = KindUnknown
	}

	return ref
}

// isRemote matches JetBrains virtual roots used for remote dev / devcontainers,
// e.g. "/$devcontainer.ij/<hash>@.../IdeaProjects/foo" or other "$...ij$"-style
// macro roots that are not local directories.
func isRemote(raw string) bool {
	if strings.HasPrefix(raw, "/$") && strings.Contains(raw, ".ij/") {
		return true
	}
	if strings.Contains(raw, "$devcontainer") || strings.Contains(raw, "$wsl") {
		return true
	}
	return false
}

// Openable reports whether a PathRef resolves to a local directory we can hand
// to an IDE launcher.
func (p PathRef) Openable() bool {
	return p.Kind == KindLocal || p.Kind == KindHomeMacro
}
