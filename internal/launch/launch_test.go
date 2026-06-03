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
