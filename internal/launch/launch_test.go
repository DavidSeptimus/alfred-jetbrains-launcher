package launch

import (
	"os/exec"
	"reflect"
	"testing"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/ide"
)

func captureArgv(t *testing.T, fn func()) []string {
	t.Helper()
	var got []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		got = append([]string{name}, args...)
		return exec.Command("true") // /usr/bin/true: succeeds, ignores stdin
	}
	defer func() { execCommand = exec.Command }()
	fn()
	return got
}

func TestOpenArgvHandlesSpaces(t *testing.T) {
	target := ide.Installed{AppPath: "/Users/dave/Applications/IntelliJ IDEA.app", Display: "IntelliJ IDEA"}
	got := captureArgv(t, func() {
		if err := Open(target, "/Users/dave/My Projects/demo"); err != nil {
			t.Fatal(err)
		}
	})
	want := []string{"open", "-na", "/Users/dave/Applications/IntelliJ IDEA.app", "--args", "/Users/dave/My Projects/demo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv:\n got %q\nwant %q", got, want)
	}
}

func TestOpenNoAppErrors(t *testing.T) {
	if err := Open(ide.Installed{Display: "Ghost"}, "/x"); err == nil {
		t.Error("Open with no AppPath should error")
	}
}

func TestRevealAndTerminalArgv(t *testing.T) {
	got := captureArgv(t, func() { _ = Reveal("/Users/dave/My Project") })
	if !reflect.DeepEqual(got, []string{"open", "-R", "/Users/dave/My Project"}) {
		t.Errorf("reveal argv: %q", got)
	}
	got = captureArgv(t, func() { _ = Terminal("iTerm", "/Users/dave/My Project") })
	if !reflect.DeepEqual(got, []string{"open", "-a", "iTerm", "/Users/dave/My Project"}) {
		t.Errorf("terminal argv: %q", got)
	}
	got = captureArgv(t, func() { _ = Terminal("", "/x") }) // empty -> default Terminal
	if !reflect.DeepEqual(got, []string{"open", "-a", "Terminal", "/x"}) {
		t.Errorf("terminal default argv: %q", got)
	}
}

func TestOpenCommand(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh") // deterministic shell for the argv assertion

	// {path} token is replaced with the single-quoted path (space-safe).
	got := captureArgv(t, func() {
		if err := OpenCommand("code {path}", "/Users/dave/My Project"); err != nil {
			t.Fatal(err)
		}
	})
	want := []string{"/bin/zsh", "-lc", `code '/Users/dave/My Project'`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("token argv:\n got %q\nwant %q", got, want)
	}

	// No {path} token: the quoted path is appended as the final argument.
	got = captureArgv(t, func() { _ = OpenCommand("code", "/x") })
	if !reflect.DeepEqual(got, []string{"/bin/zsh", "-lc", `code '/x'`}) {
		t.Errorf("append argv: %q", got)
	}

	// A single quote in the path is escaped, not left to break out of the quoting.
	got = captureArgv(t, func() { _ = OpenCommand("code {path}", "/a/b'c") })
	if !reflect.DeepEqual(got, []string{"/bin/zsh", "-lc", `code '/a/b'\''c'`}) {
		t.Errorf("escape argv: %q", got)
	}

	// {name} → the project's folder name (quoted), alongside {path}.
	got = captureArgv(t, func() {
		_ = OpenCommand("editor --name {name} --dir {path}", "/Users/dave/My Project")
	})
	want = []string{"/bin/zsh", "-lc", `editor --name 'My Project' --dir '/Users/dave/My Project'`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("name+path argv:\n got %q\nwant %q", got, want)
	}

	// A script path works like any other command; {name}/{path} substitute the same.
	got = captureArgv(t, func() { _ = OpenCommand("~/bin/open-project.sh {name} {path}", "/u/My App") })
	if !reflect.DeepEqual(got, []string{"/bin/zsh", "-lc", `~/bin/open-project.sh 'My App' '/u/My App'`}) {
		t.Errorf("script argv: %q", got)
	}
}

func TestOpenCommandEmptyErrors(t *testing.T) {
	if err := OpenCommand("   ", "/x"); err == nil {
		t.Error("OpenCommand with a blank template should error")
	}
}
