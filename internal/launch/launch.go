// Package launch performs the side-effecting actions: opening a project in an
// IDE, revealing it in Finder, copying its path, or opening a terminal there.
package launch

import (
	"fmt"
	"os/exec"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/ide"
)

// execCommand is a seam so tests can capture the argv without executing.
var execCommand = exec.Command

// Open launches the project in the given IDE via `open -na <App> --args <path>`.
// Passing the .app bundle path (not the inner MacOS launcher) lets macOS handle
// activation, and a single argv element avoids the quote-mangling the Toolbox
// launcher scripts apply to space-containing paths.
func Open(target ide.Installed, projectPath string) error {
	if target.AppPath == "" {
		return fmt.Errorf("no application found for %s", target.Display)
	}
	return execCommand("open", "-na", target.AppPath, "--args", projectPath).Run()
}

// Reveal shows the project directory in Finder.
func Reveal(projectPath string) error {
	return execCommand("open", "-R", projectPath).Run()
}

// CopyPath copies the project path to the clipboard.
func CopyPath(projectPath string) error {
	cmd := execCommand("pbcopy")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := stdin.Write([]byte(projectPath)); err != nil {
		return err
	}
	if err := stdin.Close(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Terminal opens the project directory in the given terminal app (defaulting to
// the built-in Terminal). The app is matched by name, e.g. "iTerm", "Warp".
func Terminal(app, projectPath string) error {
	if app == "" {
		app = "Terminal"
	}
	return execCommand("open", "-a", app, projectPath).Run()
}
