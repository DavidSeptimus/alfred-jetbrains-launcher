// Package tasklaunch runs a detected task's command. It is the wrapper-side
// counterpart to the Alfred-agnostic taskrunner core: taskrunner decides *what*
// to run, tasklaunch decides *how* — in a new terminal tab or window, detached
// in the background, or copied to the clipboard.
//
// Each launch puts a task in its own terminal session so several can run in
// parallel; the terminal app is the process manager (this package starts tasks,
// it does not supervise them).
package tasklaunch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// execCommand is a seam so tests can capture argv without executing.
var execCommand = exec.Command

const (
	macosOpen      = "/usr/bin/open"
	macosOsascript = "/usr/bin/osascript"
	macosPbcopy    = "/usr/bin/pbcopy"
)

// Kind is how a task is launched.
type Kind int

const (
	KindTab        Kind = iota // new terminal tab (the default; parallel-friendly)
	KindWindow                 // new terminal window
	KindBackground             // detached login-shell run, no terminal, notify on exit
	KindCopy                   // copy the command to the clipboard, run nothing
)

// ParseKind maps the spec token to a Kind, defaulting to a new tab.
func ParseKind(s string) Kind {
	switch s {
	case "window":
		return KindWindow
	case "bg":
		return KindBackground
	case "copy":
		return KindCopy
	default:
		return KindTab
	}
}

// Spec is a single launch request.
type Spec struct {
	CommandLine string // shell command to run, e.g. "./gradlew runIde"
	Cwd         string // directory to run it in
	Kind        Kind
	Terminal    string // built-in terminal app name (e.g. "Terminal", "iTerm")
	TemplateCmd string // custom terminal template; overrides Terminal when set
}

// Launch performs the request.
func (s Spec) Launch() error {
	switch s.Kind {
	case KindCopy:
		return copyToClipboard(s.CommandLine)
	case KindBackground:
		return s.runBackground()
	default:
		return s.runInTerminal()
	}
}

// shellLine is the command run inside the terminal / background shell: cd into
// the project, then the task command.
func (s Spec) shellLine() string {
	return "cd " + shellQuote(s.Cwd) + " && " + s.CommandLine
}

// runInTerminal opens the task in a terminal tab or window. A custom template
// takes precedence; otherwise a built-in handler is used, falling back to
// Terminal.app for an unrecognised name.
func (s Spec) runInTerminal() error {
	if strings.TrimSpace(s.TemplateCmd) != "" {
		return s.runTemplate()
	}
	switch normalizeTerminal(s.Terminal) {
	case "iterm":
		return runOSA(itermScript(s.shellLine(), s.Kind))
	case "ghostty":
		return s.runGhostty()
	default:
		return runOSA(terminalAppScript(s.shellLine(), s.Kind))
	}
}

// runGhostty opens the task in Ghostty. A new window uses `open -na Ghostty
// --args -e …` (robust, no permissions).
//
// The shell is run as an INTERACTIVE login shell (`-ilc`): unlike Terminal.app
// and iTerm — which spawn the profile's interactive shell themselves — this path
// execs the shell directly, so without `-i` it would skip ~/.zshrc. Many users
// put their PATH there (asdf, nvm, pyenv, rbenv via oh-my-zsh), so a login-only
// `-lc` shell fails to find tools the task needs (e.g. `gofmt: command not
// found`). Ghostty gives the command a real TTY, so an interactive shell is safe
// here (the no-TTY background path deliberately stays login-only).
//
// The tab path is deliberately BRITTLE and best-effort. As of Ghostty 1.3.1
// there is no scriptable way to run a command in a new tab: no `+new-tab` CLI
// action and no AppleScript dictionary. So we synthesise it by driving the ⌘T
// (new_tab) keybind through System Events and *typing* the command into the new
// tab's shell. That means it (a) requires Accessibility permission, (b) depends
// on timing `delay`s, and (c) sends the command as literal keystrokes. ⌘↩
// (window) is the reliable fallback when this misfires.
//
// TODO: revisit if Ghostty gains a real API — e.g. a `+new-tab -e <cmd>` CLI
// action, an AppleScript dictionary, or a control socket. Any of those would let
// us drop the keystroke synthesis (and the Accessibility requirement) entirely.
func (s Spec) runGhostty() error {
	if s.Kind == KindWindow {
		shell := loginShell()
		line := s.shellLine() + "; exec " + shell + " -il"
		return execCommand(macosOpen, "-na", "Ghostty", "--args", "-e", shell, "-ilc", line).Run()
	}
	// New tab: open one with ⌘T (new tab inherits the cwd), then type the command
	// into its interactive shell, which stays open afterwards to show output.
	q := osaQuote(s.shellLine())
	return runOSA([]string{
		"-e", `tell application "Ghostty" to activate`,
		"-e", `delay 0.2`,
		"-e", `tell application "System Events" to keystroke "t" using command down`,
		"-e", `delay 0.3`,
		"-e", `tell application "System Events" to keystroke ` + q,
		"-e", `tell application "System Events" to key code 36`,
	})
}

// runTemplate runs the user's terminal template via the login shell, the same
// mechanism as the launcher's custom open command. Tokens: {cmd} (the task
// command, spliced raw so the template can place it as a command), {cwd} and
// {name} (single-quoted, splice-safe).
func (s Spec) runTemplate() error {
	script := s.TemplateCmd
	script = strings.ReplaceAll(script, "{cwd}", shellQuote(s.Cwd))
	script = strings.ReplaceAll(script, "{name}", shellQuote(filepath.Base(s.Cwd)))
	script = strings.ReplaceAll(script, "{cmd}", s.CommandLine)
	return execCommand(loginShell(), "-lc", script).Run()
}

// runBackground runs the task detached from this process via the login shell, so
// it outlives the Script Filter, and posts a notification with the exit code
// when it finishes. The notification text embeds only the integer exit code (no
// user-controlled text), so it is quoting-safe.
func (s Spec) runBackground() error {
	line := s.shellLine() +
		`; code=$?; ` + macosOsascript + ` -e "display notification \"Finished (exit $code)\" with title \"jb task\"" >/dev/null 2>&1`
	cmd := execCommand(loginShell(), "-lc", line)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach so it outlives the Script Filter
	return cmd.Start()
}

// --- terminal scripts ---

// terminalAppScript runs line in Terminal.app. A new window uses `do script`
// directly; a new tab issues ⌘T via System Events (which needs Accessibility
// permission) and runs in the resulting front tab.
func terminalAppScript(line string, kind Kind) []string {
	q := osaQuote(line)
	if kind == KindWindow {
		return []string{
			"-e", `tell application "Terminal" to activate`,
			"-e", `tell application "Terminal" to do script ` + q,
		}
	}
	return []string{
		"-e", `tell application "Terminal" to activate`,
		"-e", `tell application "System Events" to keystroke "t" using command down`,
		"-e", `delay 0.3`,
		"-e", `tell application "Terminal" to do script ` + q + ` in front window`,
	}
}

// itermScript runs line in iTerm2, creating a new window or a tab in the current
// window (creating a window first if none is open).
func itermScript(line string, kind Kind) []string {
	q := osaQuote(line)
	if kind == KindWindow {
		return []string{
			"-e", `tell application "iTerm"`,
			"-e", `activate`,
			"-e", `set w to (create window with default profile)`,
			"-e", `tell current session of w to write text ` + q,
			"-e", `end tell`,
		}
	}
	return []string{
		"-e", `tell application "iTerm"`,
		"-e", `activate`,
		"-e", `if (count of windows) = 0 then`,
		"-e", `create window with default profile`,
		"-e", `else`,
		"-e", `tell current window to create tab with default profile`,
		"-e", `end if`,
		"-e", `tell current session of current window to write text ` + q,
		"-e", `end tell`,
	}
}

func runOSA(args []string) error {
	return execCommand(macosOsascript, args...).Run()
}

func copyToClipboard(s string) error {
	cmd := execCommand(macosPbcopy)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := in.Write([]byte(s)); err != nil {
		return err
	}
	if err := in.Close(); err != nil {
		return err
	}
	return cmd.Wait()
}

// --- helpers ---

func normalizeTerminal(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "iterm", "iterm2":
		return "iterm"
	case "ghostty":
		return "ghostty"
	default:
		return "terminal"
	}
}

func loginShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/zsh"
}

// shellQuote wraps s in single quotes, escaping embedded single quotes, so it is
// splice-safe in a shell command line.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// osaQuote renders s as an AppleScript double-quoted string literal.
func osaQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
