// Package launch performs the side-effecting actions: opening a project in an
// IDE, revealing it in Finder, copying its path, or opening a terminal there.
package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// OpenCommand runs the user's custom open command for a project (the ⌃⇧ action).
// Some tools open a project through their own CLI (an editor like `code`, a
// terminal multiplexer, an agent workspace) rather than as a macOS app, so the
// command is a free-form template. Two tokens are substituted: {path} → the
// project path, and {name} → its folder name (e.g. for naming a workspace). If
// the template has no {path} token, the path is appended as the final argument.
// The template can be any shell command line, including a path to a script.
//
// It runs through the user's login shell ($SHELL, `-lc`) for two reasons: the
// template is shell syntax (pipes, flags, args), and a login shell loads the PATH
// where such CLIs are installed (e.g. /opt/homebrew/bin or an app bundle's bin),
// which Alfred's own minimal PATH usually lacks. Each token is single-quoted
// before substitution, so it is splice-safe regardless of spaces — write {path}
// and {name} unquoted in the template.
func OpenCommand(template, projectPath string) error {
	template = strings.TrimSpace(template)
	if template == "" {
		return fmt.Errorf("no custom open command configured")
	}
	script := strings.ReplaceAll(template, "{name}", shellQuote(filepath.Base(projectPath)))
	quotedPath := shellQuote(projectPath)
	if strings.Contains(template, "{path}") {
		script = strings.ReplaceAll(script, "{path}", quotedPath)
	} else {
		script = script + " " + quotedPath
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	return execCommand(shell, "-lc", script).Run()
}

// shellQuote wraps s in single quotes, escaping any embedded single quote, so it
// is safe to splice into a shell command line regardless of spaces or metachars.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
