package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/alfred"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/ide"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/state"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/tasklaunch"
	taskrunner "github.com/davidseptimus/alfred-taskrunner"
)

// gradleTaskTTL bounds how long a cached Gradle enumeration is trusted even when
// the build-input fingerprint is unchanged, so plugin/dependency-driven task
// changes are eventually picked up.
const gradleTaskTTL = 24 * time.Hour

// cmdTasks drives the two-level `runtask` keyword (--runtask): a project picker
// and, once a project is chosen, its task list — both natively filterable
// because the selected project lives in state, not the query. With
// --enumerate-gradle it instead runs the slow Gradle enumeration in the
// background and writes it to the cache.
func cmdTasks(args []string) {
	fs := flag.NewFlagSet("tasks", flag.ExitOnError)
	runtask := fs.Bool("runtask", false, "emit the two-level runtask keyword (project picker / task list)")
	rerun := fs.Bool("rerun", false, "emit the rerun keyword (the last task you ran)")
	path := fs.String("path", "", "project path (for --enumerate-gradle, or a direct task list)")
	enumerateGradle := fs.Bool("enumerate-gradle", false, "background: enumerate Gradle tasks and cache them")
	query := fs.String("query", "", "filter text typed after the keyword")
	_ = fs.Parse(args)
	cfg := config.Load()

	switch {
	case *enumerateGradle:
		if *path == "" {
			fail("tasks: --enumerate-gradle requires --path")
		}
		refreshGradleTaskCache(cfg, *path)
	case *rerun:
		emitRerun(cfg)
	case *runtask:
		emitRuntask(cfg, *query)
	case *path != "":
		emitTaskList(cfg, *path) // direct task list for a known project (CLI / debug)
	default:
		fail("tasks: --runtask, --rerun or --path is required")
	}
}

// emitRerun shows the single most-recently-run task, re-runnable with the same
// launch matrix. Reuses the runtask launch action.
func emitRerun(cfg config.Config) {
	cwd, cmdline, ok := loadLastRun(cfg)
	if !ok {
		info := alfred.Info("No task run yet", "Run a task via the runtask keyword, then this re-runs it")
		info.Icon = &alfred.Icon{Path: iconPath("run")}
		emit([]alfred.Item{info})
		return
	}
	spec := func(kind string) string { return kind + specSep + cwd + specSep + cmdline }
	def, other := "tab", "window"
	if cfg.TaskNewWindow {
		def, other = "window", "tab"
	}
	yes := alfred.BoolPtr(true)
	emit([]alfred.Item{{
		Title:    "↻ " + cmdline,
		Subtitle: "Rerun in " + alfred.AbbreviateHome(cfg.Home, cwd),
		Arg:      spec(def),
		Icon:     &alfred.Icon{Path: iconPath("run")},
		Valid:    yes,
		Mods: map[string]alfred.Mod{
			"cmd":  {Subtitle: "Rerun in a new " + other, Arg: spec(other), Valid: yes},
			"alt":  {Subtitle: "Rerun in the background", Arg: spec("bg"), Valid: yes},
			"ctrl": {Subtitle: "Copy command to clipboard", Arg: spec("copy"), Valid: yes},
		},
	}})
}

// emitRuntask renders the active level of the runtask keyword: the task list for
// the project recorded in state (if any still exists), otherwise the project
// picker. query filters the rows.
func emitRuntask(cfg config.Config, query string) {
	if target := loadRuntaskTarget(cfg); target != "" && dirExists(target) {
		emitTaskMode(cfg, target, query)
		return
	}
	emitProjectMode(cfg, query)
}

// emitProjectMode lists the projects to choose from (the same visible set as the
// `jb` keyword), filtered by query. Selecting one records it and re-opens
// runtask on its tasks.
func emitProjectMode(cfg config.Config, query string) {
	st := state.Load(cfg.DataDir)
	projects := withDurablePins(cfg, loadProjects(cfg), st)
	sortProjects(projects, cfg.Sort)
	installed := ide.Detect(cfg)

	var pinned, rest []alfred.Item
	for _, p := range projects {
		if st.IsHidden(p.Path) || !p.Exists || p.Stub || matchesProjectIgnore(p.Path, cfg.IgnoreProjects) {
			continue
		}
		if p.IsWorktree && cfg.ExcludeWorktrees {
			continue
		}
		if p.Unopened && !st.IsPinned(p.Path) {
			continue // root-scan entries only surface under the `+` variant elsewhere
		}
		item := projectPickItem(cfg, p, installed, st.IsPinned(p.Path))
		if !queryMatches(query, item.Title+" "+item.Match) {
			continue
		}
		if st.IsPinned(p.Path) {
			pinned = append(pinned, item)
		} else {
			rest = append(rest, item)
		}
	}

	items := append(pinned, rest...)
	if len(items) == 0 {
		items = append(items, noMatchItem(query, "project", "Open a project in an IDE, then try again"))
	}
	emit(items)
}

// projectPickItem is a project row in the picker. ↩ records it as the runtask
// target (a "picktask" spec); the launch action handles the rest. The
// launch-kind modifiers are disabled here — they only mean something on a task
// row.
func projectPickItem(cfg config.Config, p recent.Project, installed []ide.Installed, pinned bool) alfred.Item {
	family := ide.FamilyOf(p.ProductionCode)
	subtitle := alfred.AbbreviateHome(cfg.Home, p.Path)
	if branch := recent.GitBranch(p.Path); branch != "" {
		subtitle += "  ·  ⎇ " + branch
	}
	title := p.DisplayName
	if pinned {
		title = "★ " + title
	}
	no := alfred.BoolPtr(false)
	return alfred.Item{
		Title:    title,
		Subtitle: subtitle,
		Arg:      "picktask" + specSep + p.Path,
		Match:    matchString(p, family, ""),
		Icon:     iconForFamily(family, installed),
		Valid:    alfred.BoolPtr(true),
		Mods: map[string]alfred.Mod{
			"cmd": {Valid: no}, "alt": {Valid: no}, "ctrl": {Valid: no},
		},
	}
}

// emitTaskMode lists the chosen project's tasks (filtered by query), preceded by
// a row to switch back to the project picker. The back row is always present, so
// the result is never empty even when the filter matches no task — which keeps
// Alfred from backfilling the list with file fallbacks.
func emitTaskMode(cfg config.Config, path, query string) {
	back := alfred.Item{
		Title:    "⬅ Switch project",
		Subtitle: "Currently: " + filepath.Base(path) + "  —  choose a different project",
		Arg:      "back",
		Match:    "back switch change project",
		Icon:     &alfred.Icon{Path: iconPath("")},
		Valid:    alfred.BoolPtr(true),
	}
	items := []alfred.Item{back}
	matched := 0
	for _, t := range detectProjectTasks(cfg, path) {
		it := taskItem(cfg, t)
		if !queryMatches(query, it.Title+" "+it.Match) {
			continue
		}
		items = append(items, it)
		matched++
	}
	if matched == 0 {
		items = append(items, noMatchItem(query, "task", "No npm / Make / just / Taskfile / Gradle / Maven tasks in "+alfred.AbbreviateHome(cfg.Home, path)))
	}
	emit(items)
}

// queryMatches reports whether every whitespace-separated token of query appears
// (case-insensitively) in haystack. An empty query matches everything.
func queryMatches(query, haystack string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	haystack = strings.ToLower(haystack)
	for _, tok := range strings.Fields(query) {
		if !strings.Contains(haystack, tok) {
			return false
		}
	}
	return true
}

// noMatchItem is the always-present "nothing matched" row, so a filtered-empty
// list still returns one of our own items (no Alfred file fallbacks). It is
// inert (Alfred's Info builds a non-valid row).
func noMatchItem(query, kind, emptyHint string) alfred.Item {
	title, sub := "No "+kind+"s found", emptyHint
	if strings.TrimSpace(query) != "" {
		title, sub = "No "+kind+` matching "`+query+`"`, "Try a different filter"
	}
	info := alfred.Info(title, sub)
	info.Icon = &alfred.Icon{Path: iconPath("")}
	return info
}

// emitTaskList renders a project's tasks without the runtask chrome — used for a
// direct `jb tasks --path` invocation (CLI / debugging).
func emitTaskList(cfg config.Config, path string) {
	items := make([]alfred.Item, 0)
	for _, t := range detectProjectTasks(cfg, path) {
		items = append(items, taskItem(cfg, t))
	}
	if len(items) == 0 {
		info := alfred.Info("No runnable tasks found", alfred.AbbreviateHome(cfg.Home, path))
		info.Icon = &alfred.Icon{Path: iconPath("")}
		items = append(items, info)
	}
	emit(items)
}

// detectProjectTasks returns a project's tasks. Gradle is special: a fresh
// cached enumeration replaces the instant fixed-verb set, otherwise the fixed
// verbs are returned now and a background enumeration is kicked off to fill the
// cache for next time.
func detectProjectTasks(cfg config.Config, path string) []taskrunner.Task {
	disabled := disabledRunners(cfg.TaskDisable)
	tasks, _ := taskrunner.Detect(path, taskrunner.Options{Disabled: disabled})
	if fp := taskrunner.GradleFingerprint(path); fp != "" && !runnerDisabled(disabled, taskrunner.RunnerGradle) {
		if cached, ok := loadGradleTaskCache(cfg, path, fp); ok {
			tasks = replaceRunner(tasks, taskrunner.RunnerGradle, cached)
		} else {
			spawnGradleEnumeration(cfg, path)
		}
	}
	return tasks
}

// taskItem builds the Script Filter row for one task. The launch matrix: ↩ runs
// in the default view (a tab, or a window when JB_TASK_WINDOW is set), ⌘ in the
// other view, ⌥ in the background, ⌃ copies, ⇧ runs in the default view then
// resets to the project picker.
func taskItem(cfg config.Config, t taskrunner.Task) alfred.Item {
	cmdline := shellJoinArgv(t.Command)
	subtitle := string(t.Runner)
	if t.Desc != "" {
		subtitle += "  ·  " + t.Desc
	} else {
		subtitle += "  ·  " + cmdline
	}
	if !t.Runnable {
		subtitle += "   —   " + t.Command[0] + " not found"
	}

	spec := func(kind string) string {
		return kind + specSep + t.Cwd + specSep + cmdline
	}
	def, other := "tab", "window"
	if cfg.TaskNewWindow {
		def, other = "window", "tab"
	}
	yes := alfred.BoolPtr(true)
	runnable := alfred.BoolPtr(t.Runnable)

	return alfred.Item{
		Title:    t.Name,
		Subtitle: subtitle,
		Arg:      spec(def),
		Match:    t.Name + " " + string(t.Runner) + " " + t.Source,
		Icon:     &alfred.Icon{Path: iconPathOr(string(t.Runner), "run")}, // per-runner icon, fallback to run
		Valid:    runnable,                                                // a non-runnable task (tool missing) is ↩-disabled, still copyable
		Mods: map[string]alfred.Mod{
			"cmd":   {Subtitle: "Run in a new " + other, Arg: spec(other), Valid: runnable},
			"alt":   {Subtitle: "Run in the background (notify on exit)", Arg: spec("bg"), Valid: runnable},
			"ctrl":  {Subtitle: "Copy command to clipboard", Arg: spec("copy"), Valid: yes},
			"shift": {Subtitle: "Run, then reset to the project picker", Arg: spec(def + "reset"), Valid: runnable},
		},
	}
}

// cmdRuntask dispatches a spec built by the runtask rows. The leading kind token
// selects the behaviour: picktask/back navigate the keyword (set or clear the
// target project, then re-open runtask), while the launch kinds run the task.
func cmdRuntask(args []string) {
	fs := flag.NewFlagSet("runtask", flag.ExitOnError)
	spec := fs.String("spec", "", "kind<US>… (picktask<US>path | back | tab|window|bg|copy<US>cwd<US>cmdline)")
	_ = fs.Parse(args)
	cfg := config.Load()

	kind, rest, _ := strings.Cut(*spec, specSep)
	switch kind {
	case "picktask":
		setRuntaskTarget(cfg, rest)
		reopenRuntask()
	case "back":
		clearRuntaskTarget(cfg)
		reopenRuntask()
	default: // a launch kind: rest is cwd<US>cmdline
		cwd, cmdline, ok := strings.Cut(rest, specSep)
		if !ok {
			fail("runtask: malformed launch spec")
		}
		// A "<kind>reset" kind (tabreset / windowreset) runs in that view and then
		// drops the runtask scope, so the next time the keyword is opened it starts
		// at the project picker.
		reset := strings.HasSuffix(kind, "reset")
		launchKind := strings.TrimSuffix(kind, "reset")
		err := tasklaunch.Spec{
			Kind:        tasklaunch.ParseKind(launchKind),
			Cwd:         cwd,
			CommandLine: cmdline,
			Terminal:    cfg.TaskTerminal,
			TemplateCmd: cfg.TaskTerminalCmd,
		}.Launch()
		if err != nil {
			fail("runtask: " + err.Error())
		}
		if launchKind != "copy" { // copy isn't an execution; don't record it as the last run
			saveLastRun(cfg, cwd, cmdline)
		}
		if reset {
			clearRuntaskTarget(cfg)
		}
		// Background launches have no terminal to confirm them, so print a line for
		// the downstream Alfred Post Notification (only-if-populated) to show —
		// the same native-notification pattern as the update flow. Other kinds
		// print nothing, so no notification fires for them.
		if launchKind == "bg" {
			fmt.Println("Running " + cmdline + " in the background")
		}
	}
}

// --- last-run state (for the rerun keyword) ---

type lastRunState struct {
	Cwd     string `json:"cwd"`
	Cmdline string `json:"cmdline"`
}

func lastRunPath(cfg config.Config) string { return filepath.Join(cfg.DataDir, "lastrun.json") }

func saveLastRun(cfg config.Config, cwd, cmdline string) {
	if os.MkdirAll(cfg.DataDir, 0o755) != nil {
		return
	}
	if data, err := json.Marshal(lastRunState{Cwd: cwd, Cmdline: cmdline}); err == nil {
		_ = os.WriteFile(lastRunPath(cfg), data, 0o644)
	}
}

func loadLastRun(cfg config.Config) (cwd, cmdline string, ok bool) {
	data, err := os.ReadFile(lastRunPath(cfg))
	if err != nil {
		return "", "", false
	}
	var s lastRunState
	if json.Unmarshal(data, &s) != nil || s.Cmdline == "" {
		return "", "", false
	}
	return s.Cwd, s.Cmdline, true
}

// --- runtask target state ---

type runtaskState struct {
	Path string `json:"path"`
}

func runtaskStatePath(cfg config.Config) string {
	return filepath.Join(cfg.DataDir, "runtask.json")
}

// loadRuntaskTarget returns the project the runtask keyword is currently scoped
// to, or "" for the project picker.
func loadRuntaskTarget(cfg config.Config) string {
	data, err := os.ReadFile(runtaskStatePath(cfg))
	if err != nil {
		return ""
	}
	var s runtaskState
	if json.Unmarshal(data, &s) != nil {
		return ""
	}
	return s.Path
}

func setRuntaskTarget(cfg config.Config, path string) {
	if os.MkdirAll(cfg.DataDir, 0o755) != nil {
		return
	}
	if data, err := json.Marshal(runtaskState{Path: path}); err == nil {
		_ = os.WriteFile(runtaskStatePath(cfg), data, 0o644)
	}
}

func clearRuntaskTarget(cfg config.Config) { _ = os.Remove(runtaskStatePath(cfg)) }

// reopenRuntask re-opens Alfred on the runtask keyword so the keyword re-reads
// the (just-changed) target and shows the next level. No-op outside Alfred.
func reopenRuntask() {
	if os.Getenv("alfred_workflow_bundleid") == "" {
		return
	}
	kw := os.Getenv("JB_KW_RUNTASK")
	if kw == "" {
		kw = "runtask"
	}
	script := `tell application id "com.runningwithcrayons.Alfred" to search ` + applescriptQuote(kw+" ")
	_ = exec.Command("osascript", "-e", script).Run()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// --- Gradle enumeration cache ---

type gradleTaskCache struct {
	Fingerprint string            `json:"fingerprint"`
	SavedUnix   int64             `json:"saved_unix"`
	Tasks       []taskrunner.Task `json:"tasks"`
}

func gradleCachePath(cfg config.Config, projectPath string) string {
	sum := sha1.Sum([]byte(projectPath))
	return filepath.Join(cfg.CacheDir, "jb-gradle-tasks-"+hex.EncodeToString(sum[:])+".json")
}

// loadGradleTaskCache returns the cached enumeration when its fingerprint
// matches the project's current build inputs and it is within the TTL.
func loadGradleTaskCache(cfg config.Config, projectPath, fingerprint string) ([]taskrunner.Task, bool) {
	data, err := os.ReadFile(gradleCachePath(cfg, projectPath))
	if err != nil {
		return nil, false
	}
	var c gradleTaskCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, false
	}
	if c.Fingerprint != fingerprint {
		return nil, false
	}
	if time.Since(time.Unix(c.SavedUnix, 0)) > gradleTaskTTL {
		return nil, false
	}
	return c.Tasks, true
}

// refreshGradleTaskCache (background invocation) enumerates the project's Gradle
// tasks and writes them to the cache, keyed on the current fingerprint.
func refreshGradleTaskCache(cfg config.Config, projectPath string) {
	fp := taskrunner.GradleFingerprint(projectPath)
	if fp == "" {
		return
	}
	// Enumerate directly (not via Detect) so only a genuine, non-empty Gradle
	// enumeration is cached — a failure, empty parse, or an unrelated detector's
	// error can never poison the cache with the fixed-verb fallback for 24h.
	gradle, err := taskrunner.EnumerateGradle(projectPath)
	if err != nil || len(gradle) == 0 {
		return
	}
	data, err := json.Marshal(gradleTaskCache{Fingerprint: fp, SavedUnix: time.Now().Unix(), Tasks: gradle})
	if err != nil {
		return
	}
	_ = os.MkdirAll(cfg.CacheDir, 0o755)
	_ = os.WriteFile(gradleCachePath(cfg, projectPath), data, 0o644)
	_ = os.Remove(gradleCachePath(cfg, projectPath) + ".spawning") // let a future refresh spawn again
}

// spawnGradleEnumeration launches a detached `jb tasks --enumerate-gradle` that
// outlives this Script Filter and populates the cache for the next view. A
// short-lived marker debounces it so repeated keystrokes (each re-running the
// Script Filter) don't spawn a stampede of overlapping Gradle daemons.
func spawnGradleEnumeration(cfg config.Config, projectPath string) {
	marker := gradleCachePath(cfg, projectPath) + ".spawning"
	// A fresh marker means an enumeration is already in flight; a stale one (a
	// crashed prior spawn) is swept so it can't block forever.
	if info, err := os.Stat(marker); err == nil {
		if time.Since(info.ModTime()) < 60*time.Second {
			return
		}
		_ = os.Remove(marker)
	}
	_ = os.MkdirAll(cfg.CacheDir, 0o755)
	// Create the marker atomically so two concurrent Script Filter invocations
	// can't both win the missing-marker check and spawn duplicate daemons.
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return // lost the race (or unwritable) — another invocation is spawning
	}
	_ = f.Close()

	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "tasks", "--path", projectPath, "--enumerate-gradle")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = cmd.Start()
}

// --- helpers ---

// disabledRunners maps configured runner names to taskrunner.Runner values,
// ignoring unknown entries.
func disabledRunners(names []string) []taskrunner.Runner {
	out := make([]taskrunner.Runner, 0, len(names))
	for _, n := range names {
		switch r := taskrunner.Runner(strings.ToLower(strings.TrimSpace(n))); r {
		case taskrunner.RunnerNpm, taskrunner.RunnerMake, taskrunner.RunnerJust,
			taskrunner.RunnerTask, taskrunner.RunnerComposer, taskrunner.RunnerDeno,
			taskrunner.RunnerRake, taskrunner.RunnerGradle, taskrunner.RunnerMaven,
			taskrunner.RunnerCargo, taskrunner.RunnerGo, taskrunner.RunnerDotnet:
			out = append(out, r)
		}
	}
	return out
}

func runnerDisabled(disabled []taskrunner.Runner, r taskrunner.Runner) bool {
	return slices.Contains(disabled, r)
}

// replaceRunner swaps every task of the given runner for replacement (used to
// substitute the fixed Gradle verbs with a cached enumeration).
func replaceRunner(tasks []taskrunner.Task, runner taskrunner.Runner, replacement []taskrunner.Task) []taskrunner.Task {
	out := make([]taskrunner.Task, 0, len(tasks)+len(replacement))
	for _, t := range tasks {
		if t.Runner != runner {
			out = append(out, t)
		}
	}
	return append(out, replacement...)
}

// shellSafe matches tokens that need no quoting: an allowlist of characters that
// are inert in a shell word. Anything else (spaces, newlines, control chars,
// $, quotes, globs, …) forces single-quoting, so a hostile task name from a
// repo's manifest can't break out of its word.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellJoinArgv joins argv into a shell command line, single-quoting any element
// that isn't purely shell-safe characters.
func shellJoinArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if shellSafe.MatchString(a) {
			parts[i] = a
		} else {
			parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(parts, " ")
}
