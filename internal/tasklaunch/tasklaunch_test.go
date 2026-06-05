package tasklaunch

import (
	"os/exec"
	"strings"
	"testing"
)

// capture swaps execCommand for one that records the argv of the last call and
// returns a harmless command, restoring the original on cleanup.
func capture(t *testing.T) *[]string {
	t.Helper()
	var got []string
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		got = append([]string{name}, args...)
		return exec.Command("true")
	}
	t.Cleanup(func() { execCommand = orig })
	return &got
}

func joined(args []string) string { return strings.Join(args, "\x00") }

func TestTerminalAppTabUsesSystemEventsKeystroke(t *testing.T) {
	got := capture(t)
	err := Spec{CommandLine: "./gradlew runIde", Cwd: "/p", Kind: KindTab, Terminal: "Terminal"}.Launch()
	if err != nil {
		t.Fatal(err)
	}
	a := joined(*got)
	if !strings.HasPrefix((*got)[0], "osascript") {
		t.Fatalf("expected osascript, got %v", *got)
	}
	if !strings.Contains(a, `keystroke "t" using command down`) {
		t.Error("tab launch should issue Cmd-T via System Events")
	}
	if !strings.Contains(a, `cd '/p' && ./gradlew runIde`) {
		t.Errorf("missing cd+command; got %v", *got)
	}
}

func TestTerminalAppWindowUsesDoScript(t *testing.T) {
	got := capture(t)
	_ = Spec{CommandLine: "make build", Cwd: "/p", Kind: KindWindow, Terminal: "Terminal"}.Launch()
	a := joined(*got)
	if strings.Contains(a, "System Events") {
		t.Error("window launch should not use System Events")
	}
	if !strings.Contains(a, "do script") {
		t.Error("window launch should use do script")
	}
}

func TestITermTabCreatesTab(t *testing.T) {
	got := capture(t)
	_ = Spec{CommandLine: "npm run dev", Cwd: "/p", Kind: KindTab, Terminal: "iTerm"}.Launch()
	a := joined(*got)
	if !strings.Contains(a, "create tab with default profile") {
		t.Errorf("iTerm tab should create a tab; got %v", *got)
	}
	if !strings.Contains(a, "write text") {
		t.Error("iTerm should write the command via write text")
	}
}

func TestCustomTemplateSubstitutesTokens(t *testing.T) {
	got := capture(t)
	_ = Spec{
		CommandLine: "npm run dev",
		Cwd:         "/my proj",
		Kind:        KindTab,
		Terminal:    "Terminal", // overridden by the template
		TemplateCmd: "kitty @ launch --type=tab --cwd {cwd} {cmd}",
	}.Launch()
	a := joined(*got)
	if (*got)[0] == "osascript" {
		t.Fatal("template should bypass the built-in osascript path")
	}
	if !strings.Contains(a, "kitty @ launch --type=tab --cwd '/my proj' npm run dev") {
		t.Errorf("token substitution wrong; got %v", *got)
	}
}

func TestGhosttyWindowUsesOpenDashE(t *testing.T) {
	got := capture(t)
	_ = Spec{CommandLine: "make build", Cwd: "/p", Kind: KindWindow, Terminal: "Ghostty"}.Launch()
	if (*got)[0] != "open" {
		t.Fatalf("ghostty window should launch via open, got %v", *got)
	}
	a := joined(*got)
	if !strings.Contains(a, "Ghostty") || !strings.Contains(a, "-e") {
		t.Errorf("expected `open -na Ghostty --args -e …`; got %v", *got)
	}
	if !strings.Contains(a, "cd '/p' && make build") {
		t.Errorf("missing cd+command; got %v", *got)
	}
}

func TestGhosttyTabUsesSystemEventsNewTab(t *testing.T) {
	got := capture(t)
	_ = Spec{CommandLine: "./gradlew runIde", Cwd: "/p", Kind: KindTab, Terminal: "Ghostty"}.Launch()
	if (*got)[0] != "osascript" {
		t.Fatalf("ghostty tab should use osascript/System Events, got %v", *got)
	}
	a := joined(*got)
	if !strings.Contains(a, `keystroke "t" using command down`) {
		t.Error("ghostty tab should open a new tab with Cmd-T")
	}
	if !strings.Contains(a, "cd '/p' && ./gradlew runIde") {
		t.Errorf("ghostty tab should type the command; got %v", *got)
	}
}

func TestCopyKindUsesPbcopy(t *testing.T) {
	got := capture(t)
	_ = Spec{CommandLine: "./gradlew test", Cwd: "/p", Kind: KindCopy}.Launch()
	if (*got)[0] != "pbcopy" {
		t.Errorf("copy should invoke pbcopy, got %v", *got)
	}
}

func TestParseKind(t *testing.T) {
	cases := map[string]Kind{"window": KindWindow, "bg": KindBackground, "copy": KindCopy, "tab": KindTab, "": KindTab}
	for in, want := range cases {
		if got := ParseKind(in); got != want {
			t.Errorf("ParseKind(%q) = %v, want %v", in, got, want)
		}
	}
}
