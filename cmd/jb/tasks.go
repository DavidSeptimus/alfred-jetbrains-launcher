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
	"strconv"
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

// gradleEnumLease bounds how long the Gradle enumeration sidecar markers are
// trusted. For the spawn marker, within the lease means an enumeration is in
// flight (debounces re-spawns, drives the live "refreshing" row); for the error
// marker, within the lease means a recent failure (shows the error row, cools
// down auto-respawns). Past it, either marker is treated as a crashed/expired
// leftover. Sized to cover a slow cold `./gradlew tasks`.
const gradleEnumLease = 90 * time.Second

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
	worktrees := fs.Bool("worktrees", false, "runtask project picker: the `~` variant (worktrees only)")
	roots := fs.Bool("roots", false, "runtask project picker: the `+` Projects variant")
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
		emitRuntask(cfg, *query, *worktrees, *roots)
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

// emitRuntask renders the active level of the runtask keyword. The plain keyword
// is two-level: the task list for the project recorded in state (if any still
// exists), otherwise the project picker. The `+`/`~` variants (worktreesFlag /
// scanRoots) instead *always* show the (widened) picker — invoking them is an
// explicit "I'm looking for a project" gesture, so they bypass the saved target
// rather than dropping the user back into its task list. They don't clear the
// target either: dismissing a variant picker leaves the prior selection intact.
// query filters the rows.
func emitRuntask(cfg config.Config, query string, worktreesFlag, scanRoots bool) {
	if !worktreesFlag && !scanRoots {
		if target := loadRuntaskTarget(cfg); target != "" && dirExists(target) {
			emitTaskMode(cfg, target, query)
			return
		}
	}
	emitProjectMode(cfg, query, worktreesFlag, scanRoots)
}

// emitProjectMode lists the projects to choose from, filtered by query. The
// candidate set mirrors the `jb` keyword's exactly — same projectInVariant
// gating — so the plain picker shows recents, `+` adds project-root entries,
// and `~` is the worktree-only list. Selecting one records it (with
// the active variant) and re-opens runtask on its tasks.
func emitProjectMode(cfg config.Config, query string, worktreesFlag, scanRoots bool) {
	st := state.Load(cfg.DataDir)
	projects := withDurablePins(cfg, loadProjects(cfg), st)
	sortProjects(projects, cfg.Sort)
	installed := ide.Detect(cfg)
	roots := projectRootSet(cfg)
	variant := variantSuffix(worktreesFlag, scanRoots)

	var pinned, rest []alfred.Item
	for _, p := range projects {
		if !projectInVariant(p, st, cfg, roots, worktreesFlag, scanRoots) {
			continue
		}
		item := projectPickItem(cfg, p, installed, st.IsPinned(p.Path), variant)
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
		items = append(items, noMatchItem(query, "project", projectPickerEmptyHint(worktreesFlag, scanRoots)))
	}
	emit(items)
}

// projectPickerEmptyHint tailors the empty-state hint to the active picker
// variant, matching how each variant sources its projects.
func projectPickerEmptyHint(worktreesFlag, scanRoots bool) string {
	switch {
	case worktreesFlag:
		return "No git worktrees of your projects were found"
	case scanRoots:
		return "No projects found in your roots"
	default:
		return "Open a project in an IDE, then try again"
	}
}

// projectPickItem is a project row in the picker. ↩ records it as the runtask
// target (a "picktask" spec carrying the active variant, so "back" returns to
// the same widened picker); the launch action handles the rest. The launch-kind
// modifiers are disabled here — they only mean something on a task row.
func projectPickItem(cfg config.Config, p recent.Project, installed []ide.Installed, pinned bool, variant string) alfred.Item {
	family := ide.FamilyOf(p.ProductionCode)
	subtitle := alfred.AbbreviateHome(cfg.Home, p.Path)
	if branch := recent.GitBranch(p.Path); branch != "" {
		subtitle += "  ·  ⎇ " + branch
	}
	// A worktree is otherwise indistinguishable from a normal repo, so mark it
	// with the same leading glyph the `jb` keyword uses, after any ★ pin marker.
	name := p.DisplayName
	if p.IsWorktree {
		name = worktreeGlyph + " " + name
	}
	title := name
	if pinned {
		title = "★ " + name
	}
	no := alfred.BoolPtr(false)
	return alfred.Item{
		Title:    title,
		Subtitle: subtitle,
		Arg:      "picktask" + specSep + p.Path + specSep + variant,
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
	// Detect first: a cold drill-in spawns the background Gradle enumeration here
	// (creating the in-flight marker), so the in-flight check below must run after
	// it to catch that first paint and start polling immediately.
	tasks := detectProjectTasks(cfg, path)

	items := []alfred.Item{back}
	// While a Gradle enumeration is running, show a live "refreshing" row instead
	// of the (re-)refresh row and ask Alfred to re-poll, so the list swaps to the
	// fresh tasks on its own when the background job lands. Otherwise offer the
	// manual rescan row.
	refreshing := gradleEnumInFlight(cfg, path)
	if refreshing {
		items = append(items, gradleRefreshingItem())
	} else {
		// A recent failure surfaces an error row above the (still-offered) manual
		// refresh row, so the user knows the list is the fixed-verb fallback and can
		// retry.
		if gradleEnumErrored(cfg, path) {
			items = append(items, gradleErrorItem())
		}
		if refresh, ok := gradleRefreshItem(cfg, path); ok {
			items = append(items, refresh)
		}
	}
	matched := 0
	for _, t := range tasks {
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
	if refreshing {
		emitWithRerun(items, gradleRerunInterval)
		return
	}
	emit(items)
}

// gradleRerunInterval is how often Alfred re-runs the runtask Script Filter while
// a Gradle enumeration is in flight, so the list updates in place when it lands.
const gradleRerunInterval = 0.7

// gradleRefreshItem is the manual "rescan" row, shown only for Gradle projects
// (the one runner whose task list is cached and can go stale; every other runner
// is re-detected from disk on each keystroke). It kicks a background re-enumeration
// and reopens the keyword, which then shows the live "refreshing" row and polls
// until the fresh list lands — so Alfred never blocks on the slow `./gradlew
// tasks`. The path travels in the spec so the action needs no state read.
func gradleRefreshItem(cfg config.Config, path string) (alfred.Item, bool) {
	disabled := disabledRunners(cfg.TaskDisable)
	if runnerDisabled(disabled, taskrunner.RunnerGradle) || taskrunner.GradleFingerprint(path) == "" {
		return alfred.Item{}, false
	}
	return alfred.Item{
		Title:    "↻ Refresh tasks",
		Subtitle: "Re-enumerate Gradle tasks (the cached list may be stale)",
		Arg:      "refresh" + specSep + path,
		Match:    "refresh rescan reload gradle tasks",
		Icon:     &alfred.Icon{Path: iconPathOr("gradle", "run")},
		Valid:    alfred.BoolPtr(true),
	}, true
}

// gradleRefreshingItem is the live progress row shown while a Gradle enumeration
// is running. It's inert (not actionable) — the Script Filter's rerun swaps it
// for the fresh task list automatically once the background job completes.
func gradleRefreshingItem() alfred.Item {
	info := alfred.Info("↻ Refreshing Gradle tasks…", "Re-enumerating in the background — the list updates automatically")
	info.Icon = &alfred.Icon{Path: iconPathOr("gradle", "run")}
	return info
}

// gradleErrorItem is shown when the most recent Gradle enumeration failed. It's
// inert; the manual refresh row beside it lets the user retry (which clears the
// error sentinel). The fixed-verb tasks are still listed below it.
func gradleErrorItem() alfred.Item {
	info := alfred.Info("⚠ Gradle task refresh failed", "Showing default tasks — select ↻ Refresh tasks to retry")
	info.Icon = &alfred.Icon{Path: iconPathOr("gradle", "")}
	return info
}

// gradleSpawnMarker / gradleErrorMarker are the sidecar files next to a project's
// Gradle task cache: the spawn marker signals an enumeration in flight; the error
// marker records that the last enumeration failed.
func gradleSpawnMarker(cfg config.Config, path string) string {
	return gradleCachePath(cfg, path) + ".spawning"
}
func gradleErrorMarker(cfg config.Config, path string) string {
	return gradleCachePath(cfg, path) + ".error"
}

// gradleEnumInFlight reports whether a background Gradle enumeration for path is
// currently running, detected via its (still-fresh) spawn marker. A stale marker
// (a crashed enumeration that never cleaned up) reads as not-in-flight so the
// loading row and rerun loop don't persist forever.
func gradleEnumInFlight(cfg config.Config, path string) bool {
	return markerFresh(gradleSpawnMarker(cfg, path))
}

// gradleEnumErrored reports whether the most recent enumeration failed within the
// lease window — drives the error row and (via spawnGradleEnumeration) the
// cooldown that stops a broken build from retrying on every keystroke.
func gradleEnumErrored(cfg config.Config, path string) bool {
	return markerFresh(gradleErrorMarker(cfg, path))
}

// markerFresh reports whether a sidecar marker file exists and is within the
// lease window (a stale one is treated as absent — a crashed leftover).
func markerFresh(path string) bool {
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) < gradleEnumLease
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
// taskSubtitle builds the row subtitle shared by the Alfred task list and the
// JSON API: the runner, then the task's description (or its command line), then
// a "not found" note when the tool is missing. Shared so the two frontends can't
// drift; the len guard keeps a non-runnable task with an empty argv from panicking.
func taskSubtitle(t taskrunner.Task, cmdline string) string {
	subtitle := string(t.Runner)
	if t.Desc != "" {
		subtitle += "  ·  " + t.Desc
	} else {
		subtitle += "  ·  " + cmdline
	}
	if !t.Runnable && len(t.Command) > 0 {
		subtitle += "   —   " + t.Command[0] + " not found"
	}
	return subtitle
}

func taskItem(cfg config.Config, t taskrunner.Task) alfred.Item {
	cmdline := shellJoinArgv(t.Command)
	subtitle := taskSubtitle(t, cmdline)

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
		// rest is path<US>variant — record both, then reopen the *plain* keyword so
		// it lands in this project's task list (the variant keywords force the picker,
		// so reopening one of them would bounce straight back out).
		path, variant, _ := strings.Cut(rest, specSep)
		setRuntaskTarget(cfg, path, variant)
		reopenRuntask("")
	case "back":
		// Drop the project but keep the variant, and reopen that variant's picker so
		// the user returns to the same (possibly widened) project list they came from.
		variant := loadRuntaskState(cfg).Variant
		clearRuntaskTarget(cfg)
		reopenRuntask(variant)
	case "refresh":
		// Kick a background re-enumeration of the project's Gradle tasks and reopen
		// immediately — the reopened keyword shows the live "refreshing" row and
		// polls (via Alfred rerun) until the fresh cache lands, so Alfred never
		// blocks on the slow `./gradlew tasks`. spawnGradleEnumeration always
		// re-enumerates (it never consults the cache), so a manual refresh refreshes
		// even a currently-valid cache; an already-running enumeration is reused.
		// Clear any error sentinel first so a manual retry isn't suppressed by the
		// post-failure cooldown.
		_ = os.Remove(gradleErrorMarker(cfg, rest))
		spawnGradleEnumeration(cfg, rest)
		reopenRuntask("") // refresh happens in task mode; reopen the plain keyword to stay there
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

// runtaskState holds what the runtask keyword is scoped to: the selected project
// Path (empty = project picker) and the picker Variant ("" / "+" / "~") the
// project was chosen from. The variant is persisted so "back" and the launch
// action's reopen return to the same widened picker, not the plain recents one.
type runtaskState struct {
	Path    string `json:"path"`
	Variant string `json:"variant,omitempty"`
}

func runtaskStatePath(cfg config.Config) string {
	return filepath.Join(cfg.DataDir, "runtask.json")
}

// loadRuntaskState returns the full runtask scope. A missing/unreadable file
// reads as the empty state (project picker, plain variant).
func loadRuntaskState(cfg config.Config) runtaskState {
	data, err := os.ReadFile(runtaskStatePath(cfg))
	if err != nil {
		return runtaskState{}
	}
	var s runtaskState
	if json.Unmarshal(data, &s) != nil {
		return runtaskState{}
	}
	return s
}

// loadRuntaskTarget returns the project the runtask keyword is currently scoped
// to, or "" for the project picker.
func loadRuntaskTarget(cfg config.Config) string {
	return loadRuntaskState(cfg).Path
}

func writeRuntaskState(cfg config.Config, s runtaskState) {
	if os.MkdirAll(cfg.DataDir, 0o755) != nil {
		return
	}
	if data, err := json.Marshal(s); err == nil {
		_ = os.WriteFile(runtaskStatePath(cfg), data, 0o644)
	}
}

// setRuntaskTarget records the chosen project and the variant it was picked
// from. The variant rides along so a later "back" reopens the same picker.
func setRuntaskTarget(cfg config.Config, path, variant string) {
	writeRuntaskState(cfg, runtaskState{Path: path, Variant: variant})
}

// clearRuntaskTarget drops the selected project but preserves the variant, so
// "back" (and a reset launch) returns to the picker the project was chosen from
// rather than the plain recents picker.
func clearRuntaskTarget(cfg config.Config) {
	v := loadRuntaskState(cfg).Variant
	if v == "" {
		_ = os.Remove(runtaskStatePath(cfg))
		return
	}
	writeRuntaskState(cfg, runtaskState{Variant: v})
}

// reopenRuntask re-opens Alfred on the runtask keyword (with the given variant
// suffix, "" / "+" / "~") so the keyword re-reads the just-changed scope and
// shows the next level. No-op outside Alfred.
func reopenRuntask(variant string) {
	if os.Getenv("alfred_workflow_bundleid") == "" {
		return
	}
	kw := os.Getenv("JB_KW_RUNTASK")
	if kw == "" {
		kw = "runtask"
	}
	script := `tell application id "com.runningwithcrayons.Alfred" to search ` + applescriptQuote(kw+variant+" ")
	_ = exec.Command("/usr/bin/osascript", "-e", script).Run()
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
		// Not (or no longer) a Gradle project: there's nothing to enumerate. Still
		// drop the in-flight marker so a stale "Refreshing…" spinner can't linger for
		// the lease. No cooldown is needed — detectProjectTasks never re-spawns for a
		// non-Gradle dir, so this can't loop.
		_ = os.Remove(gradleSpawnMarker(cfg, projectPath))
		return
	}
	// Enumerate directly (not via Detect) so only a genuine, non-empty Gradle
	// enumeration is cached — a failure, empty parse, or an unrelated detector's
	// error can never poison the cache with the fixed-verb fallback for 24h.
	gradle, err := taskrunner.EnumerateGradle(projectPath)
	if err != nil || len(gradle) == 0 {
		recordGradleEnumError(cfg, projectPath)
		return
	}
	data, err := json.Marshal(gradleTaskCache{Fingerprint: fp, SavedUnix: time.Now().Unix(), Tasks: gradle})
	if err != nil {
		recordGradleEnumError(cfg, projectPath)
		return
	}
	_ = os.MkdirAll(cfg.CacheDir, 0o755)
	if err := os.WriteFile(gradleCachePath(cfg, projectPath), data, 0o644); err != nil {
		// Couldn't record the success: throttle it like a failure rather than leave
		// neither a fresh cache nor a marker, which would let the next rerun re-spawn
		// immediately and loop.
		recordGradleEnumError(cfg, projectPath)
		return
	}
	_ = os.Remove(gradleSpawnMarker(cfg, projectPath)) // success: stop the spinner, let a future refresh spawn again
	_ = os.Remove(gradleErrorMarker(cfg, projectPath)) // clear any prior failure
}

// recordGradleEnumError marks a failed Gradle enumeration: it writes the .error
// sentinel (which surfaces the error row and, within the lease, cools down
// auto-respawns) and only then drops the .spawning marker. The ordering matters —
// if the .error write fails, .spawning is left in place as a fallback throttle
// (bounded by its own lease), so a rerun still can't tight-loop a re-spawn.
func recordGradleEnumError(cfg config.Config, projectPath string) {
	_ = os.MkdirAll(cfg.CacheDir, 0o755)
	if err := os.WriteFile(gradleErrorMarker(cfg, projectPath), nil, 0o644); err != nil {
		return
	}
	_ = os.Remove(gradleSpawnMarker(cfg, projectPath))
}

// spawnGradleEnumeration launches a detached `jb tasks --enumerate-gradle` that
// outlives this Script Filter and populates the cache for the next view. A
// short-lived marker debounces it so repeated keystrokes (each re-running the
// Script Filter) don't spawn a stampede of overlapping Gradle daemons.
func spawnGradleEnumeration(cfg config.Config, projectPath string) {
	// Back off when the last enumeration recently failed, so a broken build doesn't
	// retry on every keystroke. A manual refresh removes this sentinel first, so it
	// is never blocked by the cooldown.
	if markerFresh(gradleErrorMarker(cfg, projectPath)) {
		return
	}
	marker := gradleSpawnMarker(cfg, projectPath)
	// A fresh marker means an enumeration is already in flight. A stale one is
	// swept only when its recorded process is actually gone — a Gradle enumeration
	// that runs past the lease (cold daemon, huge project) is still in flight, so
	// we must not spawn a duplicate just because the marker aged out.
	if info, err := os.Stat(marker); err == nil {
		if time.Since(info.ModTime()) < gradleEnumLease {
			return
		}
		if pid := readMarkerPID(marker); pid > 0 && processAlive(pid) {
			return // time-stale, but the enumeration is still running
		}
		_ = os.Remove(marker) // crashed leftover (or no live process) — sweep it
	}
	_ = os.MkdirAll(cfg.CacheDir, 0o755)
	// Create the marker atomically so two concurrent Script Filter invocations
	// can't both win the missing-marker check and spawn duplicate daemons.
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return // lost the race (or unwritable) — another invocation is spawning
	}

	exe, err := os.Executable()
	if err != nil {
		_ = f.Close()
		_ = os.Remove(marker) // nothing will run; don't leave a phantom in-flight marker
		return
	}
	cmd := exec.Command(exe, "tasks", "--path", projectPath, "--enumerate-gradle")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		_ = os.Remove(marker) // failed to spawn; clear the marker so the spinner/rerun won't wedge
		return
	}
	// Record the PID so a later stale-marker check can tell "still running" from
	// "crashed", then close. mtime updates here — harmless for the time-based
	// freshness check that drives the spinner.
	_, _ = fmt.Fprintf(f, "%d", cmd.Process.Pid)
	_ = f.Close()
}

// readMarkerPID returns the PID recorded in a spawn marker, or 0 if the marker
// is empty/unreadable (older markers predate PID recording).
func readMarkerPID(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

// processAlive reports whether pid names a live process. Signal 0 probes for
// existence without delivering a signal; EPERM means it exists but is owned by
// another user (still alive).
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
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
