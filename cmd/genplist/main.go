// Command genplist generates the Alfred workflow info.plist from workflow/ides.json.
//
// It is regenerated rather than hand-edited because the object graph (one Script
// Filter per keyword, shared action objects, and the connections wiring every
// keyword to them through modifier keys) is large and easy to desync by hand.
// UIDs are derived deterministically (UUIDv5) from the bundle id + a role, so
// the output is stable across runs and diffs cleanly.
package main

import (
	"crypto/sha1"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Alfred modifier-key bitmasks (NSEvent modifier flags).
const (
	modNone  = 0
	modShift = 131072
	modCtrl  = 262144
	modAlt   = 524288
	modCmd   = 1048576
)

type idesFile struct {
	Workflow struct {
		BundleID    string `json:"bundleid"`
		Name        string `json:"name"`
		Description string `json:"description"`
		CreatedBy   string `json:"createdby"`
		WebAddress  string `json:"webaddress"`
		Category    string `json:"category"`
	} `json:"workflow"`
	Keywords []struct {
		Keyword string `json:"keyword"`
		Product string `json:"product"`
		Title   string `json:"title"`
		Subtext string `json:"subtext"`
	} `json:"keywords"`
}

func main() {
	in := flag.String("ides", "workflow/ides.json", "path to ides.json")
	out := flag.String("o", "info.plist", "output info.plist path")
	version := flag.String("version", "0.0.0", "workflow version")
	bundle := flag.String("bundle", "", "bundle dir; when set, also writes per-object <uid>.png canvas icons")
	channel := flag.String("channel", "release", "build channel; only 'release' includes the in-app update UI")
	flag.Parse()

	data, err := os.ReadFile(*in)
	if err != nil {
		fatal(err)
	}
	var spec idesFile
	if err := json.Unmarshal(data, &spec); err != nil {
		fatal(err)
	}

	ns := namespace(spec.Workflow.BundleID)
	uid := func(role string) string { return uuid5(ns, spec.Workflow.BundleID+":"+role) }

	// The in-app updater is release-only: a source build updates via git, so its
	// plist omits the whole update UI (the banner Conditional + its notifications)
	// rather than offering an update path that doesn't apply to a dev build.
	release := *channel == "release"

	// Shared modifier actions + the "pick a different IDE" drill-down. The project
	// path reaches every action as $1 (the actioned item's arg) — the proven Alfred
	// mechanism; we do not rely on item variables.
	revealUID := uid("action:reveal")
	copyUID := uid("action:copy")
	terminalUID := uid("action:terminal")
	customCmdUID := uid("action:command")
	pinUID := uid("action:pin")
	forgetUID := uid("action:forget")
	pickUID := uid("sf:pick")
	openPickUID := uid("action:openpick")
	// Task runner: a standalone two-level `runtask` keyword (project picker → task
	// list, both natively filterable because the chosen project lives in state).
	// One launch action handles every row — its leading "kind" token selects pick
	// / back / launch — so ↩ and the launch modifiers all route to it.
	runtaskSfUID := uid("sf:runtask")
	runtaskPlusUID := uid("sf:runtask+") // `runtask+` variant: widened project-roots picker
	runtaskWtUID := uid("sf:runtask~")   // `runtask~` variant: worktree-only project picker
	runtaskUID := uid("action:runtask")
	runtaskKwVar := "JB_KW_RUNTASK"
	// Rerun keyword: a one-row Script Filter showing the last task run, routed to
	// the same launch action.
	rerunSfUID := uid("sf:rerun")
	rerunKwVar := "JB_KW_RERUN"
	// Native "running in the background" notification for ⌥ launches — fired by the
	// launch action printing a line, shown only when populated (same pattern as the
	// update flow's notifications, not an osascript one).
	taskNotifyUID := uid("output:task-notify")
	updateApplyUID := uid("action:update")
	// The unified-keyword update banner routes ↩ through a Conditional that sends
	// the banner row (jb_action=update) to update-apply + a notification, and any
	// other row to the open action.
	updateCondUID := uid("cond:update")
	updateCondOutUID := uid("cond:update:matched")
	notifyUID := uid("output:notify")
	errorNotifyUID := uid("output:notify-error")

	var objects []any
	var objIcons []iconRef // per-object canvas icons (uid -> family; "" = main workflow icon)
	uidata := map[string]any{}

	addUI := func(id string, x, y float64) { uidata[id] = map[string]any{"xpos": x, "ypos": y} }

	objects = append(objects,
		scriptAction(revealUID, `./jb action --do reveal --path "$1"`),
		scriptAction(copyUID, `./jb action --do copy --path "$1"`),
		scriptAction(terminalUID, `./jb action --do terminal --path "$1"`),
		// Custom open command (⌃⇧): runs the user's JB_OPEN_CMD template (e.g.
		// `code {path}`). The binary reads the template + path and runs it in the
		// login shell; this node only forwards the path.
		scriptAction(customCmdUID, `./jb action --do command --path "$1"`),
		scriptAction(openPickUID, `./jb open --spec "$1"`),
		// Pin/forget apply the change, then re-open Alfred on the same keyword +
		// query (handled inside the binary) so the window stays open in place.
		scriptAction(pinUID, `./jb pin --path "$1"`),
		scriptAction(forgetUID, `./jb forget --path "$1"`),
		// filterResults=false: this filter is reached by drill-down, so Alfred
		// passes the project path as the query — we must NOT let it filter the
		// IDE list against that path (which would hide every IDE).
		scriptFilter(pickUID, "", `./jb ides --path "$1"`, "Open in a Different IDE", "Pick an installed IDE", false),
		// The runtask keyword. It filters its own results (so both the project
		// picker and the task list are type-to-filter), and the selected project is
		// kept in state by the launch action — never in the query — so filtering is
		// never fighting the project path.
		runtaskFilter(runtaskSfUID, "{var:"+runtaskKwVar+"}", "",
			"Run a Task", "Pick a project, then a task to run"),
		// `runtask+` / `runtask~` — same two-level keyword, but their project picker
		// mirrors `jb+` / `jb~` (project-root entries / git worktrees). They
		// always open the picker (never a saved project's task list), since invoking
		// them is an explicit "find me a project" gesture.
		runtaskFilter(runtaskPlusUID, "{var:"+runtaskKwVar+"}+", " --roots",
			"Run a Task (+ projects)", "Pick from your project roots, then a task to run"),
		runtaskFilter(runtaskWtUID, "{var:"+runtaskKwVar+"}~", " --worktrees",
			"Run a Task (+ worktrees)", "Pick a git worktree, then a task to run"),
		scriptAction(runtaskUID, `./jb runtask --spec "$1"`),
		// Rerun keyword: shows the single most-recent task, re-runnable via the same
		// launch action. filterResults=false (one row, nothing to filter).
		scriptFilter(rerunSfUID, "{var:"+rerunKwVar+"}", `./jb tasks --rerun --query "$1"`,
			"Rerun Last Task", "Re-run the task you most recently ran", false),
		notification(taskNotifyUID, "Task runner", "{query}", true),
	)
	addUI(runtaskPlusUID, 170, 1060)
	addUI(runtaskWtUID, 290, 1060)
	addUI(runtaskSfUID, 420, 1060)
	addUI(runtaskUID, 760, 1060)
	addUI(rerunSfUID, 420, 1200)
	addUI(taskNotifyUID, 980, 1060)
	objIcons = append(objIcons,
		iconRef{runtaskSfUID, "run"}, iconRef{runtaskPlusUID, "run"}, iconRef{runtaskWtUID, "run"},
		iconRef{runtaskUID, "run"}, iconRef{rerunSfUID, "run"})
	addUI(customCmdUID, 760, 80)
	addUI(revealUID, 760, 220)
	addUI(copyUID, 760, 360)
	addUI(terminalUID, 760, 500)
	addUI(pinUID, 760, 640)
	addUI(forgetUID, 760, 780)
	addUI(pickUID, 420, 920)
	addUI(openPickUID, 760, 920)

	// Shared/utility objects use the main (Toolbox) icon on the canvas.
	objIcons = append(objIcons,
		iconRef{customCmdUID, ""}, iconRef{pickUID, ""}, iconRef{openPickUID, ""})

	connections := map[string]any{}
	connections[pickUID] = []any{conn(openPickUID, modNone)} // pick (ides) -> open-by-spec
	// runtask rows -> the launch action. ↩ covers pick-project / back / run-tab
	// (the action dispatches on the row's kind token); the modifiers add the other
	// launch kinds for task rows (project rows disable them).
	runtaskConns := []any{
		conn(runtaskUID, modNone),
		connSub(runtaskUID, modCmd, "Run in a new window"),
		connSub(runtaskUID, modAlt, "Run in the background"),
		connSub(runtaskUID, modCtrl, "Copy command"),
		connSub(runtaskUID, modShift, "Run, then reset to the project picker"),
	}
	// The `+`/`~` variants share the launch action and the same row mods — they
	// differ only in which projects their picker surfaces.
	connections[runtaskSfUID] = runtaskConns
	connections[runtaskPlusUID] = runtaskConns
	connections[runtaskWtUID] = runtaskConns
	// Rerun keyword rows route to the same launch action (↩ / launch modifiers).
	connections[rerunSfUID] = []any{
		conn(runtaskUID, modNone),
		connSub(runtaskUID, modCmd, "Rerun in a new window"),
		connSub(runtaskUID, modAlt, "Rerun in the background"),
		connSub(runtaskUID, modCtrl, "Copy command"),
	}
	// Launch action -> "running in background" notification (suppressed unless the
	// action printed a line, i.e. only for ⌥ background launches).
	connections[runtaskUID] = []any{conn(taskNotifyUID, modNone)}

	// Update UI — release only, and surfaced solely through the in-`jb` banner
	// (updateBanner in the binary emits the row; the Conditional in the loop wires
	// it). A source build's plist has none of this, and there is no update keyword.
	if release {
		objects = append(objects,
			scriptAction(updateApplyUID, `./jb update --apply`),
			conditional(updateCondUID, updateCondOutUID, "{var:jb_action}", "update", "Update", "Open"),
			notification(notifyUID, "JetBrains IDE Project Launcher", "Downloading the update…", false),
			notification(errorNotifyUID, "Couldn't update the launcher", "{query}", true),
		)
		addUI(updateApplyUID, 760, 1060)
		addUI(updateCondUID, 600, 1200)
		addUI(notifyUID, 980, 1200)
		addUI(errorNotifyUID, 980, 1340)
		objIcons = append(objIcons, iconRef{updateApplyUID, ""})

		// update-apply -> error notification (shown only when apply prints an error,
		// suppressed on success). The banner Conditional (in the loop) routes the
		// matched row to update-apply + the "Downloading…" notification.
		connections[updateApplyUID] = []any{conn(errorNotifyUID, modNone)}
	}

	// Each keyword's results route to its open action (↩) plus the shared
	// reveal/pick/copy/terminal/pin/forget actions on modifier keys.
	keyMods := func(enter string) []any {
		return []any{
			conn(enter, modNone),
			connSub(revealUID, modCmd, "Reveal in Finder"),
			connSub(pickUID, modAlt, "Open in a different IDE…"),
			connSub(copyUID, modCtrl, "Copy path"),
			connSub(terminalUID, modShift, "Open in terminal"),
			connSub(customCmdUID, modCtrl+modShift, "Open with your custom command (e.g. VS Code)"),
			connSub(pinUID, modCmd+modShift, "Pin / unpin"),
			connSub(forgetUID, modCmd+modAlt, "Forget (hide)"),
			// ⌥⇧ jumps straight into this project's tasks: it records the project as
			// the runtask target (the item's "picktask" arg) via the launch action,
			// which re-opens the runtask keyword in task mode. A fast lane into the
			// standalone runtask keyword. (⌘⌃ was avoided — macOS reserves it.)
			connSub(runtaskUID, modAlt+modShift, "Run a task in this project…"),
		}
	}

	// One Script Filter + dedicated open action per keyword, plus a "<keyword>~"
	// variant that also includes git worktrees (reusing the same open + mods).
	// The keyword family is baked into its open action; the path arrives as $1.
	// Each keyword is read from a workflow variable (JB_KW_<NAME>) so it can be
	// renamed in the Configure Workflow panel and survive updates (unlike editing
	// the node directly, which an import/regenerate would overwrite). The live
	// keyword is also passed to the binary via --keyword so pin/forget can re-open
	// Alfred on the renamed keyword.
	//
	// The keyword *field* uses the {var:JB_KW_<NAME>} placeholder, which Alfred
	// expands when matching the trigger. The script *body*, however, must reference
	// the keyword as the exported $JB_KW_<NAME> environment variable: Alfred does
	// NOT expand {var:…} inside a Script Filter's script — it only exports config
	// variables to the environment. Passing {var:…} there forwards the literal text,
	// which the binary then saves and pin/forget re-searches (showing a stray
	// "{var:JB_KW_JB}" in the Alfred box).
	y := 40.0
	var kwConfig []any
	for _, k := range spec.Keywords {
		sfUID := uid("sf:" + k.Keyword)
		openUID := uid("action:open:" + k.Keyword)
		kwVar := "JB_KW_" + strings.ToUpper(k.Keyword)
		kwRef := "{var:" + kwVar + "}" // keyword field — Alfred expands {var:…} here
		kwEnv := "${" + kwVar + "}"    // script body — Alfred exports the var to the env, not {var:…}
		base := `./jb search`
		if k.Product != "" {
			base += ` --product ` + k.Product
		}
		openScript := `./jb open --product "` + k.Product + `" --path "$1"`
		objects = append(objects,
			scriptFilter(sfUID, kwRef, base+` --keyword "`+kwEnv+`" --query "$1"`, k.Title, k.Subtext, true),
			scriptAction(openUID, openScript),
		)
		// Canvas icons: the keyword + its open action show the IDE's icon
		// (the unified `jb` keyword, product "", uses the main workflow icon).
		objIcons = append(objIcons, iconRef{sfUID, k.Product}, iconRef{openUID, k.Product})
		addUI(sfUID, 50, y)
		addUI(openUID, 980, y)

		// On release builds the unified `jb` / `jb~` keywords carry the update
		// banner, so route their ↩ through the Conditional (banner row -> notify +
		// apply; everything else -> open). Per-IDE keywords — and every keyword on a
		// source build, which has no update UI — open directly.
		enterUID := openUID
		if release && k.Product == "" {
			enterUID = updateCondUID
			connections[updateCondUID] = []any{
				connFrom(notifyUID, updateCondOutUID),
				connFrom(updateApplyUID, updateCondOutUID),
				conn(openUID, modNone), // else: open the selected project normally
			}
		}
		connections[sfUID] = keyMods(enterUID)

		// `<keyword>+` — same search, but also including projects found by scanning
		// the project roots: JB_PROJECT_ROOTS when set, otherwise the
		// auto-detected conventional ~/<IDE>Projects / ~/<IDE>Workspaces folders.
		plusUID := uid("sf:" + k.Keyword + "+")
		objects = append(objects, scriptFilter(plusUID, kwRef+"+",
			base+` --roots --keyword "`+kwEnv+`+" --query "$1"`, k.Title+" (+ projects)",
			k.Subtext+", including projects from your roots", true))
		objIcons = append(objIcons, iconRef{plusUID, k.Product})
		addUI(plusUID, 170, y)
		connections[plusUID] = keyMods(enterUID)

		// `<keyword>~` — same search, but including git worktrees.
		wtUID := uid("sf:" + k.Keyword + "~")
		objects = append(objects, scriptFilter(wtUID, kwRef+"~",
			base+` --worktrees --keyword "`+kwEnv+`~" --query "$1"`, k.Title+" (+ worktrees)",
			k.Subtext+", including git worktrees", true))
		objIcons = append(objIcons, iconRef{wtUID, k.Product})
		addUI(wtUID, 290, y)
		connections[wtUID] = keyMods(enterUID)

		kwConfig = append(kwConfig, keywordField(kwVar, k.Keyword, k.Title))
		y += 120
	}

	// The standalone task-runner keyword, configurable like the others. Its
	// default text ("runtask") backs the {var:JB_KW_RUNTASK} placeholder in the
	// Script Filter and the reopen the launch action issues after each navigation.
	kwConfig = append(kwConfig, keywordField(runtaskKwVar, "runtask", "Run a Task"))
	kwConfig = append(kwConfig, keywordField(rerunKwVar, "rerun", "Rerun Last Task"))

	plist := map[string]any{
		"bundleid":                spec.Workflow.BundleID,
		"name":                    spec.Workflow.Name,
		"description":             spec.Workflow.Description,
		"createdby":               spec.Workflow.CreatedBy,
		"webaddress":              spec.Workflow.WebAddress,
		"category":                spec.Workflow.Category,
		"version":                 *version,
		"disabled":                false,
		"readme":                  readme,
		"objects":                 objects,
		"connections":             connections,
		"uidata":                  uidata,
		"variablesdontexport":     []any{},
		"userconfigurationconfig": append(userConfig(), kwConfig...),
	}

	var buf strings.Builder
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	buf.WriteString(`<plist version="1.0">` + "\n")
	encode(&buf, plist, 0)
	buf.WriteString("\n</plist>\n")

	if err := os.WriteFile(*out, []byte(buf.String()), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d keywords)\n", *out, len(spec.Keywords))

	if *bundle != "" {
		n := writeObjectIcons(*bundle, objIcons)
		fmt.Printf("wrote %d per-object canvas icons into %s\n", n, *bundle)
	}
}

type iconRef struct {
	uid    string
	family string // "" means use the main workflow icon (icon.png)
}

// uidIconRe matches a per-object canvas icon file (a deterministic UUID + .png).
var uidIconRe = regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}\.png$`)

// pruneObjectIcons removes every existing per-object canvas icon from the bundle
// root so icons for renamed/removed objects (or objects whose family icon was
// dropped) don't linger across builds. The current set is rewritten right after.
func pruneObjectIcons(bundle string) {
	entries, err := os.ReadDir(bundle)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && uidIconRe.MatchString(e.Name()) {
			_ = os.Remove(filepath.Join(bundle, e.Name()))
		}
	}
}

// writeObjectIcons gives each object its own canvas icon by copying the matching
// family icon to <bundle>/<uid>.png (Alfred's per-object icon convention).
func writeObjectIcons(bundle string, refs []iconRef) int {
	pruneObjectIcons(bundle) // clear stale per-object icons from earlier builds
	n := 0
	for _, r := range refs {
		src := filepath.Join(bundle, "icons", r.family+".png")
		if r.family == "" {
			src = filepath.Join(bundle, "icon.png")
		}
		data, err := os.ReadFile(src)
		if err != nil {
			continue // icon not present for this family; Alfred falls back to the workflow icon
		}
		if err := os.WriteFile(filepath.Join(bundle, r.uid+".png"), data, 0o644); err == nil {
			n++
		}
	}
	return n
}

// dequarantinePrefix strips the com.apple.quarantine flag that a browser attaches
// to a manually-downloaded release, before the (ad-hoc-signed) jb binary is
// exec'd. macOS Gatekeeper blocks a never-approved quarantined binary on first
// launch (it's killed before main() runs, so the binary cannot clear its own
// flag) — which would make the workflow show nothing until the user ran xattr by
// hand. This runs in Alfred's inline shell (a system binary, not gated) and
// clears the flag first, so the launch succeeds. It is prepended to every Script
// Filter because those are the only entry points reachable before any jb run
// (modifier actions always follow a Script Filter, by which point jb is cleared).
//
// /usr/bin/xattr is spelled out so a pyenv/conda xattr on PATH (which lacks -r)
// can't shadow the macOS built-in. A .dequarantined marker keeps the directory
// sweep to one pass per install (cwd is the workflow dir, so "$PWD" is the
// bundle); Alfred wipes it when it re-imports the bundle, so an upgrade that is
// itself quarantined is cleaned again. All steps are best-effort.
const dequarantinePrefix = `if [ ! -e .dequarantined ]; then /usr/bin/xattr -dr com.apple.quarantine "$PWD" >/dev/null 2>&1; touch .dequarantined; fi
`

func scriptFilter(uid, keyword, script, title, subtext string, filterResults bool) map[string]any {
	script = dequarantinePrefix + script
	cfg := map[string]any{
		"alfredfiltersresults":           filterResults,
		"alfredfiltersresultsmatchmode":  2,
		"argumenttreatemptyqueryasnil":   false,
		"argumenttrimmode":               0,
		"argumenttype":                   1,
		"escaping":                       102,
		"keyword":                        keyword,
		"queuedelaycustom":               3,
		"queuedelayimmediatelyinitially": true,
		"queuedelaymode":                 0,
		"queuemode":                      1,
		"runningsubtext":                 "Searching projects…",
		"script":                         script,
		"scriptargtype":                  1,
		"scriptfile":                     "",
		"subtext":                        subtext,
		"title":                          title,
		"type":                           5,
		"withspace":                      true,
	}
	return map[string]any{
		"config":  cfg,
		"type":    "alfred.workflow.input.scriptfilter",
		"uid":     uid,
		"version": 3,
	}
}

// runtaskFilter builds the standalone two-level runtask keyword Script Filter.
// filterResults=false: the binary filters its own rows by the query and always
// returns at least one of its own items, so Alfred never sees an empty list to
// backfill with file/web fallback results. (With Alfred filtering instead, a
// query that matched no task left an empty set and Alfred padded it with random
// files.)
func runtaskFilter(uid, keyword, flags, title, subtext string) map[string]any {
	sf := scriptFilter(uid, keyword, `./jb tasks --runtask`+flags+` --query "$1"`, title, subtext, false)
	sf["config"].(map[string]any)["runningsubtext"] = "Loading…"
	return sf
}

func scriptAction(uid, script string) map[string]any {
	return map[string]any{
		"config": map[string]any{
			"concurrently":  false,
			"escaping":      102,
			"script":        script,
			"scriptargtype": 1,
			"scriptfile":    "",
			"type":          5,
		},
		"type":    "alfred.workflow.action.script",
		"uid":     uid,
		"version": 2,
	}
}

func conn(dest string, mod int) map[string]any {
	return map[string]any{
		"destinationuid":  dest,
		"modifiers":       mod,
		"modifiersubtext": "",
		"vitoclose":       false,
	}
}

func connSub(dest string, mod int, subtext string) map[string]any {
	c := conn(dest, mod)
	c["modifiersubtext"] = subtext
	return c
}

// connFrom is a connection that originates from a specific Conditional output
// (its matched-condition uid). Else-branch connections use plain conn() and omit
// sourceoutputuid — that omission is how Alfred encodes the "otherwise" path.
func connFrom(dest, sourceOutput string) map[string]any {
	c := conn(dest, modNone)
	c["sourceoutputuid"] = sourceOutput
	return c
}

// conditional builds a Conditional utility with one "is equal to" rule
// (matchmode 0) on input: matching input flows out of outUID (connFrom), and
// anything else flows out of the unlabelled else path (plain conn).
func conditional(uid, outUID, input, matchStr, matchLabel, elseLabel string) map[string]any {
	return map[string]any{
		"config": map[string]any{
			"conditions": []any{
				map[string]any{
					"inputstring":        input,
					"matchcasesensitive": false,
					"matchmode":          0, // 0 = "is equal to"
					"matchstring":        matchStr,
					"outputlabel":        matchLabel,
					"uid":                outUID,
				},
			},
			"elselabel": elseLabel,
			"hideelse":  false,
		},
		"type":    "alfred.workflow.utility.conditional",
		"uid":     uid,
		"version": 1,
	}
}

// notification builds a Post Notification output. Pass onlyIfQuery=true for a
// notification that should only fire when non-empty input flows in (used for the
// error message, which is suppressed on success when the action prints nothing);
// false for a static message that always shows (the "Downloading…" start).
func notification(uid, title, text string, onlyIfQuery bool) map[string]any {
	return map[string]any{
		"config": map[string]any{
			"lastpathcomponent":        false,
			"onlyshowifquerypopulated": onlyIfQuery,
			"removeextension":          false,
			"text":                     text,
			"title":                    title,
		},
		"type":    "alfred.workflow.output.notification",
		"uid":     uid,
		"version": 1,
	}
}

// keywordField builds a Configure-Workflow text field that overrides one
// keyword. The default is the built-in keyword; clearing the field disables that
// keyword's trigger.
func keywordField(variable, def, title string) map[string]any {
	return map[string]any{
		"type":        "textfield",
		"variable":    variable,
		"label":       title + " keyword",
		"description": "Alfred keyword that triggers " + title + " (its `~` worktree and `+` project-roots variants follow it). Clear to disable.",
		"config": map[string]any{
			"default":     def,
			"placeholder": "",
			"required":    false,
			"trim":        true,
		},
	}
}

func userConfig() []any {
	// tfp is a text field with a greyed placeholder — used where an empty value is
	// meaningful (it triggers a behaviour) so the field shouldn't read as "unset"
	// (e.g. Project roots shows "Auto-detect" when left blank).
	tfp := func(variable, label, desc, def, placeholder string) map[string]any {
		return map[string]any{
			"type":        "textfield",
			"variable":    variable,
			"label":       label,
			"description": desc,
			"config": map[string]any{
				"default":     def,
				"placeholder": placeholder,
				"required":    false,
				"trim":        true,
			},
		}
	}
	tf := func(variable, label, desc, def string) map[string]any {
		return map[string]any{
			"type":        "textfield",
			"variable":    variable,
			"label":       label,
			"description": desc, // examples live here, not in a misleading placeholder
			"config": map[string]any{
				"default":     def,
				"placeholder": "",
				"required":    false,
				"trim":        true,
			},
		}
	}
	worktreeCheckbox := map[string]any{
		"type":        "checkbox",
		"variable":    "JB_EXCLUDE_WORKTREES",
		"label":       "Worktrees",
		"description": "Linked git worktrees are hidden from results when checked.",
		"config": map[string]any{
			"default": true,
			"text":    "Exclude git worktrees",
		},
	}
	terminalPopup := map[string]any{
		"type":        "popupbutton",
		"variable":    "JB_TERMINAL",
		"label":       "Terminal app",
		"description": "App used by the ⇧ (open in terminal) action.",
		"config": map[string]any{
			"default": "Terminal",
			"pairs": []any{
				[]any{"Terminal", "Terminal"},
				[]any{"iTerm", "iTerm"},
				[]any{"Warp", "Warp"},
				[]any{"Ghostty", "Ghostty"},
				[]any{"WezTerm", "WezTerm"},
				[]any{"kitty", "kitty"},
				[]any{"Alacritty", "Alacritty"},
				[]any{"Hyper", "Hyper"},
			},
		},
	}
	taskTerminalPopup := map[string]any{
		"type":        "popupbutton",
		"variable":    "JB_TASK_TERMINAL",
		"label":       "Task terminal",
		"description": "Terminal the task runner (the runtask keyword) launches into. Terminal.app and iTerm get real tabs/windows; for others set a custom launch template below. Leave as Terminal to inherit, or pick one.",
		"config": map[string]any{
			"default": "Terminal",
			"pairs": []any{
				[]any{"Terminal", "Terminal"},
				[]any{"iTerm", "iTerm"},
				[]any{"Ghostty", "Ghostty"},
			},
		},
	}
	sortPopup := map[string]any{
		"type":        "popupbutton",
		"variable":    "JB_SORT",
		"label":       "Sort order",
		"description": "Order of results. Alfred re-ranks by relevance once you type a query.",
		"config": map[string]any{
			"default": "recency",
			"pairs": []any{
				[]any{"Most recently used first", "recency"},
				[]any{"Least recently used first", "recency-asc"},
				[]any{"Name (A–Z)", "name"},
				[]any{"Name (Z–A)", "name-desc"},
				[]any{"Path (A–Z)", "path"},
			},
		},
	}
	// The path fields are pre-populated with their defaults so the values are
	// always visible and editable; the binary falls back to the same defaults if
	// a field is cleared.
	return []any{
		worktreeCheckbox,
		terminalPopup,
		tf("JB_OPEN_CMD", "Custom open command",
			"Command run by the ⌃⇧ action. {path} → the project path, {name} → its folder name (both quoted for you — leave them unquoted). Runs in your login shell (a script path works too). Examples: code {path}  ·  ~/bin/open-project.sh {name} {path}",
			""),
		taskTerminalPopup,
		map[string]any{
			"type":        "checkbox",
			"variable":    "JB_TASK_WINDOW",
			"label":       "Task window",
			"description": "When checked, ↩ runs a task in a new terminal window instead of a new tab (and ⌘↩ then does the opposite). Applies to the built-in terminals.",
			"config":      map[string]any{"default": false, "text": "Open tasks in a new window (not a tab)"},
		},
		tf("JB_TASK_TERMINAL_CMD", "Custom task terminal command",
			"Overrides the Task terminal above to launch tasks in any terminal. {cmd} → the task command (raw), {cwd} → the project dir, {name} → its folder name ({cwd}/{name} quoted for you). The template runs in your login shell, which under Alfred has a minimal PATH — so launch the terminal via `open` (found regardless of PATH) and have the task source your shell rc so its own PATH (asdf/nvm/pyenv) loads. Examples: open -na kitty.app --args --hold -d {cwd} /bin/zsh -lc \"source ~/.zshrc; {cmd}\"  ·  open -na WezTerm.app --args start --cwd {cwd} -- /bin/zsh -lc \"source ~/.zshrc; {cmd}; exec /bin/zsh -il\"",
			""),
		tf("JB_TASK_DISABLE", "Disable task runners",
			"Comma-separated build systems to skip when listing tasks (npm, make, just, task, composer, deno, rake, gradle, maven, cargo, go, dotnet). Disabling gradle also skips its slow task enumeration. Leave empty to detect all.",
			""),
		sortPopup,
		tf("JB_IGNORE_CONTENT", "Ignore content",
			"Comma-separated entry-name globs treated as non-content; a project whose only contents are these (or hidden files) is hidden as a stub.",
			"build,dist,node_modules"),
		tf("JB_IGNORE_PROJECTS", "Ignore projects",
			"Comma-separated globs matched against a project's name and full path; matches are hidden. For example: *-scratch, ~/Downloads/*",
			""),
		tf("JB_CONFIG_ROOTS", "Config roots",
			"Colon-separated dirs holding per-version IDE config dirs (JetBrains & Google). Clear to restore the defaults.",
			"~/Library/Application Support/JetBrains:~/Library/Application Support/Google"),
		tf("JB_APP_ROOTS", "Application folders",
			"Colon-separated folders scanned for JetBrains .app bundles. Clear to restore the defaults.",
			"/Applications:~/Applications"),
		tfp("JB_PROJECT_ROOTS", "Project roots",
			"Colon-separated dirs whose immediate subfolders are offered as projects even if you've never opened them — reachable via the `+` keyword variant (e.g. `jb+`, `idea+`). Leave empty to auto-detect the conventional ~/<IDE>Projects and ~/<IDE>Workspaces folders that exist. Set to override. For example: ~/IdeaProjects:~/GolandProjects",
			"",
			"Auto-detect"),
		tf("JB_TOOLBOX_DIR", "Toolbox script dirs",
			"Colon-separated dirs holding JetBrains Toolbox launcher scripts. Clear to restore the default.",
			"~/Library/Application Support/JetBrains/Toolbox/scripts"),
	}
}

const readme = `# JetBrains IDE Project Launcher

Search and open your recent JetBrains projects across **all** installed IDEs and
**all** installed versions.

- ` + "`jb`" + ` — search every recent project; each opens in the IDE it was last used in.
- per-IDE keywords (` + "`idea`, `pycharm`, `goland`, …" + `) — limit to one IDE.
- append ` + "`~`" + ` to any keyword (` + "`jb~`, `goland~`" + `) to include git worktrees.
- append ` + "`+`" + ` to any keyword (` + "`jb+`, `idea+`" + `) to also include projects from your configured project roots.

Modifiers on a result: ⌘ reveal · ⌥ open in a different IDE · ⌃ copy path · ⇧ open in terminal · ⌃⇧ custom open command (e.g. VS Code) · ⌘⇧ pin/unpin · ⌘⌥ forget · ⌥⇧ run a build-system task in this project.

Run tasks with the ` + "`runtask`" + ` keyword — pick a project, then a task (npm / Make / just / Taskfile / Gradle / Maven), filtering at each step. ⌥⇧ on a ` + "`jb`" + ` result jumps straight to that project's tasks. On a task: ↩ new terminal tab · ⌘ new window · ⌥ background · ⌃ copy the command · ⇧ run then reset to the project picker. ` + "`runtask`" + ` stays scoped to the last project until you switch; ` + "`rerun`" + ` re-runs your most recent task.

---
Not affiliated with or endorsed by JetBrains. IDE logos are trademarks of their
respective owners, used for identification only. MIT-licensed (code).`

// --- deterministic UID (UUIDv5) ---

func namespace(seed string) [16]byte {
	// A fixed namespace UUID for this workflow family.
	return [16]byte{0x6b, 0x2f, 0x1a, 0x9c, 0x4d, 0x8e, 0x5f, 0x70, 0x91, 0xa2, 0xb3, 0xc4, 0xd5, 0xe6, 0xf7, 0x08}
}

func uuid5(ns [16]byte, name string) string {
	h := sha1.New()
	h.Write(ns[:])
	h.Write([]byte(name))
	s := h.Sum(nil)
	var u [16]byte
	copy(u[:], s[:16])
	u[6] = (u[6] & 0x0f) | 0x50 // version 5
	u[8] = (u[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%X-%X-%X-%X-%X", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// --- minimal plist encoder ---

func encode(b *strings.Builder, v any, indent int) {
	pad := strings.Repeat("\t", indent)
	switch val := v.(type) {
	case string:
		fmt.Fprintf(b, "%s<string>%s</string>", pad, esc(val))
	case bool:
		if val {
			fmt.Fprintf(b, "%s<true/>", pad)
		} else {
			fmt.Fprintf(b, "%s<false/>", pad)
		}
	case int:
		fmt.Fprintf(b, "%s<integer>%d</integer>", pad, val)
	case float64:
		fmt.Fprintf(b, "%s<real>%g</real>", pad, val)
	case []any:
		if len(val) == 0 {
			fmt.Fprintf(b, "%s<array/>", pad)
			return
		}
		fmt.Fprintf(b, "%s<array>\n", pad)
		for i, e := range val {
			encode(b, e, indent+1)
			if i < len(val)-1 {
				b.WriteString("\n")
			}
		}
		fmt.Fprintf(b, "\n%s</array>", pad)
	case map[string]any:
		if len(val) == 0 {
			fmt.Fprintf(b, "%s<dict/>", pad)
			return
		}
		fmt.Fprintf(b, "%s<dict>\n", pad)
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			fmt.Fprintf(b, "%s\t<key>%s</key>\n", pad, esc(k))
			encode(b, val[k], indent+1)
			if i < len(keys)-1 {
				b.WriteString("\n")
			}
		}
		fmt.Fprintf(b, "\n%s</dict>", pad)
	default:
		panic(fmt.Sprintf("unsupported plist type %T", v))
	}
}

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "genplist:", err)
	os.Exit(1)
}
