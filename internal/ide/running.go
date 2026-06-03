package ide

import (
	"os/exec"
	"strings"
)

// psCommand lists running process command lines; a seam for tests.
var psCommand = func() ([]byte, error) {
	return exec.Command("ps", "-axo", "command").Output()
}

// runningSet returns the set of installed IDE AppPaths that currently have a
// running process (matched by the bundle's MacOS launcher path appearing in the
// process table).
func runningSet(installed []Installed) map[string]bool {
	out, err := psCommand()
	if err != nil {
		return nil
	}
	text := string(out)
	set := map[string]bool{}
	for _, in := range installed {
		if in.AppPath == "" {
			continue
		}
		if strings.Contains(text, in.AppPath+"/Contents/MacOS/") {
			set[in.AppPath] = true
		}
	}
	return set
}

// PreferRunning swaps the resolved target for an already-running IDE of the same
// product (any version) when one is running, so we reuse a live IDE instead of
// spawning a different version. The target is returned unchanged when it is
// itself running or no same-product IDE is running.
func PreferRunning(installed []Installed, target Installed) Installed {
	running := runningSet(installed)
	if len(running) == 0 || running[target.AppPath] {
		return target
	}
	best, found := target, false
	for _, in := range installed {
		if in.Code != target.Code || !running[in.AppPath] {
			continue
		}
		if !found || cmpVersion(in.Version, best.Version) > 0 {
			best, found = in, true
		}
	}
	if found {
		return best
	}
	return target
}
