// Command jb is the backend for the JetBrains project launcher Alfred workflow.
//
// Subcommands:
//
//	jb search [--product <family>] [--worktrees] [--roots] [--query <q>]  Script Filter JSON of recent projects
//	jb ides   --path <p>                            Script Filter JSON of IDEs to open <p> with
//	jb open   --path <p> [--product <family>] | --spec <code\x1fdatadir\x1fpath>
//	jb action --do reveal|copy|terminal --path <p>
//	jb tasks  --path <p> [--enumerate-gradle]      Script Filter JSON of a project's build-system tasks
//	jb runtask --spec <kind\x1fcwd\x1fcmdline>      launch a task (tab|window|bg|copy)
//	jb pin    --path <p>                            toggle a project's pinned state
//	jb forget --path <p> | --clear                  hide a project (or restore all)
//	jb update [--check | --apply]                    check for / install a newer release
//	jb refresh                                       rebuild the project cache
//	jb doctor                                        print diagnostics
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/alfred"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/cache"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/discover"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/ide"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/launch"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/state"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/update"
)

// version and channel are set at build time via -ldflags. channel is "release"
// only for binaries produced by `make dist`; source builds stay "dev" and do
// not self-update (so a developer's working copy is never overwritten).
var (
	version = "dev"
	channel = "dev"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: jb <search|ides|open|action|tasks|runtask|pin|forget|update|refresh|doctor>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "search":
		cmdSearch(os.Args[2:])
	case "ides":
		cmdIDEs(os.Args[2:])
	case "open":
		cmdOpen(os.Args[2:])
	case "action":
		cmdAction(os.Args[2:])
	case "tasks":
		cmdTasks(os.Args[2:])
	case "runtask":
		cmdRuntask(os.Args[2:])
	case "pin":
		cmdPin(os.Args[2:])
	case "forget":
		cmdForget(os.Args[2:])
	case "update":
		cmdUpdate(os.Args[2:])
	case "refresh":
		cmdRefresh()
	case "doctor":
		cmdDoctor()
	case "version", "--version", "-v":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "jb: unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

// loadProjects returns the merged project list, using the cache when valid.
func loadProjects(cfg config.Config) []recent.Project {
	files := discover.Find(cfg)
	ships := discover.FindShips(cfg)
	roots := effectiveProjectRoots(cfg)
	fp := cache.Fingerprint(cfg, files, ships, rootPaths(roots))
	if projects, ok := cache.Load(cfg, fp); ok {
		return projects
	}
	projects := recent.Merge(parseAll(cfg, files, ships), cfg.IgnoreContent)
	// Fold each project root's immediate subdirs in as un-opened projects, tagged
	// with the IDE the root implies. They are cached alongside recents (one shared
	// cache for both `jb` and `jb+`); emitSearch hides them unless `+` is used.
	if scan := scanUnopened(roots); len(scan) > 0 {
		projects = recent.AppendUnopened(projects, scan, cfg.IgnoreContent)
	}
	cache.Save(cfg, fp, projects)
	return projects
}

// projectRoot is a scanned root plus the production code its folder implies
// ("" for user-configured roots, which imply no particular IDE).
type projectRoot struct {
	Path string
	Code string
}

// effectiveProjectRoots is the list of roots scanned for the `+` variant: the
// user's configured JB_PROJECT_ROOTS when set (no implied IDE), otherwise the
// conventional ~/<IDE>Projects / ~/<IDE>Workspaces folders that exist under home,
// each tagged with the IDE it implies.
func effectiveProjectRoots(cfg config.Config) []projectRoot {
	if len(cfg.ProjectRoots) > 0 {
		roots := make([]projectRoot, len(cfg.ProjectRoots))
		for i, p := range cfg.ProjectRoots {
			// Absolute so scanned paths dedup against recents' canonical (absolute)
			// keys and never reach Alfred's launch actions as a relative arg.
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
			roots[i] = projectRoot{Path: p}
		}
		return roots
	}
	return defaultProjectRoots(cfg.Home)
}

// defaultProjectRoots returns the conventional JetBrains project/workspace
// folders that actually exist under home, matched case-insensitively so on-disk
// casing (e.g. "GoLandProjects") is honoured, each tagged with the IDE it
// implies. Only existing directories are returned, so a relocated or unused
// convention simply contributes nothing.
func defaultProjectRoots(home string) []projectRoot {
	if home == "" {
		return nil
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		return nil
	}
	actual := make(map[string]string, len(entries)) // lower-case name -> real name
	for _, e := range entries {
		if e.IsDir() {
			actual[strings.ToLower(e.Name())] = e.Name()
		}
	}
	var roots []projectRoot
	for _, d := range ide.DefaultProjectDirs() {
		if real, ok := actual[strings.ToLower(d.Name)]; ok {
			roots = append(roots, projectRoot{Path: filepath.Join(home, real), Code: d.Code})
		}
	}
	return roots
}

// rootPaths extracts just the paths, for the cache fingerprint.
func rootPaths(roots []projectRoot) []string {
	paths := make([]string, len(roots))
	for i, r := range roots {
		paths[i] = r.Path
	}
	return paths
}

// scanUnopened scans each root one level deep, tagging every subdirectory with
// the root's implied production code.
func scanUnopened(roots []projectRoot) []recent.ScanDir {
	var out []recent.ScanDir
	for _, r := range roots {
		for _, dir := range discover.FindProjectDirs([]string{r.Path}) {
			out = append(out, recent.ScanDir{Path: dir, Code: r.Code})
		}
	}
	return out
}

// parseAll parses every recentProjects.xml plus every Fleet/Air ship store into
// one slice of raw entries for merging.
func parseAll(cfg config.Config, files []discover.RecentFile, ships []discover.ShipFile) []recent.RawEntry {
	var raw []recent.RawEntry
	for _, f := range files {
		raw = append(raw, recent.Parse(cfg.Home, f)...)
	}
	for _, s := range ships {
		raw = append(raw, recent.ParseShips(cfg.Home, s)...)
	}
	return raw
}

func cmdSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	product := fs.String("product", "", "limit to one IDE family (e.g. idea, goland)")
	worktrees := fs.Bool("worktrees", false, "include git worktrees regardless of config")
	roots := fs.Bool("roots", false, "include un-opened projects from the configured project roots (the `+` variant)")
	query := fs.String("query", "", "current query (recorded so pin/forget can restore it)")
	keyword := fs.String("keyword", "", "the live Alfred keyword (so pin/forget re-opens the right, possibly-renamed, keyword)")
	_ = fs.Parse(args)
	emitSearch(config.Load(), *product, *query, *worktrees, *roots, *keyword)
}

// emitSearch renders the Script Filter results for a product family. It records
// the current keyword + query so that pin/forget can re-open Alfred in place.
func emitSearch(cfg config.Config, product, query string, worktreesFlag, scanRoots bool, keyword string) {
	// Prefer the live keyword Alfred passed (honours a user rename); fall back to
	// the built-in keyword when run from the CLI without --keyword, or when Alfred
	// handed over an unresolved/blank value. The latter guards the env-var path:
	// the Script Filter passes the keyword as $JB_KW_<NAME>, so an unexported var
	// arrives empty (or as a lone "~" for the worktree variant), and a stale plist
	// could still pass a literal "{var:…}". Any of those would otherwise be saved
	// and re-searched by pin/forget, surfacing as stray text in the Alfred box.
	if keyword == "" || keyword == "~" || keyword == "+" || strings.Contains(keyword, "{var:") {
		keyword = keywordFor(product, worktreesFlag, scanRoots)
	}
	saveLastSearch(cfg.DataDir, keyword, query)
	st := state.Load(cfg.DataDir)
	projects := withDurablePins(cfg, loadProjects(cfg), st)
	sortProjects(projects, cfg.Sort)
	installed := ide.Detect(cfg)
	showWorktrees := worktreesFlag || !cfg.ExcludeWorktrees

	// For a per-IDE keyword, note whether that IDE is actually installed.
	keywordInstalled := true
	if product != "" {
		_, keywordInstalled = ide.NewestByFamily(installed, product)
	}

	// Pinned projects float to the top, preserving recency order within each group.
	var pinned, rest []alfred.Item
	for _, p := range projects {
		if st.IsHidden(p.Path) {
			continue // user forgot this project
		}
		if !p.Exists {
			continue // hide recent entries whose directory no longer exists
		}
		if p.Stub {
			continue // hide stubs with no real content (only hidden / ignored entries)
		}
		if matchesProjectIgnore(p.Path, cfg.IgnoreProjects) {
			continue // user-configured project-level ignore
		}
		if p.IsWorktree && !showWorktrees {
			continue // hide linked git worktrees by default
		}
		if p.Unopened && !scanRoots && !st.IsPinned(p.Path) {
			// Un-opened root-scan entries only show under the `+` variant — unless
			// pinned: a pin means "keep this handy", so (like a durable pin that has
			// aged out of recents) it surfaces in the normal list too, ★-pinned.
			continue
		}
		if product != "" && !familyMatches(p, product) {
			// Un-opened projects with no implied IDE — from a custom JB_PROJECT_ROOTS,
			// which disables auto-detection — are eligible under any per-IDE keyword;
			// the keyword then drives which IDE opens them (ide.Resolve hard-limits to
			// it). Coded un-opened entries (auto-detected roots) stay scoped to their
			// implied IDE, so they don't fan out across every per-IDE keyword.
			if !(p.Unopened && p.ProductionCode == "") {
				continue
			}
		}
		target, found := ide.Resolve(installed, p.ProductionCode, p.SourceDataDir, product)

		// Icon: under a per-IDE keyword, always show that keyword's IDE (you asked
		// for `goland`, you see GoLand icons). For the unified `jb` keyword, reflect
		// the project's OWN last-used IDE (its productionCode) — using the vendored
		// fallback icon when that IDE isn't installed. The subtitle still says which
		// IDE it will actually open in.
		family := product
		if product == "" {
			family = ide.FamilyOf(p.ProductionCode)
		}
		// The IDE icon already identifies the IDE, so the subtitle omits the IDE
		// name (leaving room for the branch) — keeping only a warning when no IDE,
		// or the keyword's IDE, is installed. The IDE name still feeds the match
		// string so users can filter by it.
		matchLabel := ""
		if found {
			matchLabel = target.Display
		}

		subtitle := alfred.AbbreviateHome(cfg.Home, p.Path)
		switch {
		case !found:
			subtitle += "  —  no IDE installed"
		case product != "" && !keywordInstalled:
			subtitle += "  —  " + ide.FamilyDisplay(product) + " not installed"
		}
		if branch := recent.GitBranch(p.Path); branch != "" {
			subtitle += "  ·  ⎇ " + branch
		}

		pinnedNow := st.IsPinned(p.Path)
		title, pinLabel := p.DisplayName, "Pin to top"
		if pinnedNow {
			title, pinLabel = "★ "+p.DisplayName, "Unpin"
		}

		// No uid: Alfred uses an item's uid to re-rank results by learned action
		// frequency ("knowledge"), which overrides the order we emit and interleaves
		// pinned items with frequently-used ones. Omitting it keeps our pinned-first
		// and configured sort order authoritative.
		item := alfred.Item{
			Title:    title,
			Subtitle: subtitle,
			Arg:      p.Path,
			Match:    matchString(p, family, matchLabel),
			Icon:     iconForFamily(family, installed),
			Valid:    alfred.BoolPtr(found),
			Mods: map[string]alfred.Mod{
				"cmd":        {Subtitle: "Reveal in Finder"},
				"alt":        {Subtitle: "Open in a different IDE…"},
				"ctrl":       {Subtitle: "Copy path to clipboard"},
				"shift":      {Subtitle: "Open in terminal"},
				"ctrl+shift": openCmdMod(cfg.OpenCmd),
				"cmd+shift":  {Subtitle: pinLabel},
				"cmd+alt":    {Subtitle: "Forget (hide from this list)"},
				// ⌥⇧ jumps into the runtask keyword scoped to this project: the
				// "picktask" arg routes through the launch action, which records the
				// project as the runtask target and re-opens runtask on its tasks. Valid
				// even with no IDE installed, since running tasks doesn't need one. (⌥⇧,
				// not ⌘⌃ — macOS reserves ⌘⌃ chords system-wide.)
				"alt+shift": {
					Subtitle: "Run a task in this project…",
					Arg:      "picktask" + specSep + p.Path,
					Valid:    alfred.BoolPtr(true),
				},
			},
		}
		if pinnedNow {
			pinned = append(pinned, item)
		} else {
			rest = append(rest, item)
		}
	}

	items := append(pinned, rest...)
	if len(items) == 0 {
		items = append(items, emptyStateItem(product, keywordInstalled))
	}
	if banner, ok := updateBanner(cfg, product); ok {
		items = append([]alfred.Item{banner}, items...)
	}
	emit(items)
}

// updateBanner returns an informational "update available" row for the unified
// keyword, driven by the cached check (no network). It also kicks off a
// debounced once-a-day background refresh of that cache. Release builds only.
func updateBanner(cfg config.Config, product string) (alfred.Item, bool) {
	if channel != "release" || product != "" {
		return alfred.Item{}, false
	}
	c := update.LoadCache(cfg.DataDir)
	if c.Stale(24 * time.Hour) {
		update.TouchChecked(cfg.DataDir) // debounce so keystrokes don't all spawn
		spawnBackgroundRefresh()
	}
	if c.LatestTag == "" || !update.IsNewer(c.LatestTag, version) {
		return alfred.Item{}, false
	}
	item := infoIcon("Workflow update available — "+c.LatestTag, "↩ to update now (you have "+version+")")
	item.Valid = alfred.BoolPtr(true)
	// The unified `jb`/`jb~` Script Filters route ↩ through a Conditional that
	// sends this row (tagged jb_action=update) to the update-apply action plus a
	// notification, and every other row to the open action. Set the tag here.
	item.Variables = map[string]string{"jb_action": "update"}
	// The project-result modifiers (reveal/pin/…) are meaningless on this row, so
	// disable them rather than have them act on an empty/foreign arg.
	no := alfred.BoolPtr(false)
	item.Mods = map[string]alfred.Mod{
		"cmd": {Valid: no}, "alt": {Valid: no}, "ctrl": {Valid: no},
		"shift": {Valid: no}, "ctrl+shift": {Valid: no},
		"cmd+shift": {Valid: no}, "cmd+alt": {Valid: no},
	}
	return item, true
}

// spawnBackgroundRefresh launches a detached `jb update --refresh-cache` that
// outlives this Script Filter invocation and updates the cache for next time.
func spawnBackgroundRefresh() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "update", "--refresh-cache")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // fully detach
	_ = cmd.Start()
}

func cmdIDEs(args []string) {
	fs := flag.NewFlagSet("ides", flag.ExitOnError)
	path := fs.String("path", "", "project path to open")
	_ = fs.Parse(args)

	cfg := config.Load()
	installed := ide.Detect(cfg)
	sort.Slice(installed, func(i, j int) bool { return installed[i].Display < installed[j].Display })

	items := make([]alfred.Item, 0, len(installed))
	for _, in := range installed {
		items = append(items, alfred.Item{
			UID:      "pick:" + in.Code + ":" + in.DataDir,
			Title:    in.Display,
			Subtitle: "Open in " + in.Display + " " + in.Version,
			Arg:      in.Code + specSep + in.DataDir + specSep + *path,
			Match:    in.Display + " " + in.Family + " " + in.Version,
			Icon:     &alfred.Icon{Type: "fileicon", Path: in.AppPath},
		})
	}
	if len(items) == 0 {
		items = append(items, alfred.Info("No JetBrains IDEs installed", ""))
	}
	emit(items)
}

// specSep separates the fields of an --spec value (ASCII unit separator), used
// by the "pick a different IDE" flow to convey the exact chosen IDE.
const specSep = "\x1f"

func cmdOpen(args []string) {
	fs := flag.NewFlagSet("open", flag.ExitOnError)
	path := fs.String("path", "", "project path")
	product := fs.String("product", "", "IDE family hard-limit")
	spec := fs.String("spec", "", "exact pick: code<US>datadir<US>path")
	_ = fs.Parse(args)

	cfg := config.Load()
	var code, datadir, p, family string

	if *spec != "" {
		parts := strings.SplitN(*spec, specSep, 3)
		if len(parts) != 3 {
			fail("open: malformed --spec")
		}
		code, datadir, p = parts[0], parts[1], parts[2]
		family = ide.FamilyOf(code) // hard-limit to the picked IDE's family + exact version
	} else {
		p = *path
		family = *product
		// Re-resolve the recorded IDE/version by looking the path up in the
		// merged list — so the open action only needs the path as $1. If the path
		// has aged out of recents but is a durable pin, fall back to the IDE
		// association snapshotted when it was pinned.
		if proj, ok := lookupByPath(cfg, p); ok {
			code, datadir = proj.ProductionCode, proj.SourceDataDir
		} else if pi, ok := state.Load(cfg.DataDir).PinInfoFor(p); ok {
			code, datadir = pi.Code, pi.DataDir
		}
	}

	if p == "" {
		fail("open: --path is required")
	}
	installed := ide.Detect(cfg)
	target, ok := ide.Resolve(installed, code, datadir, family)
	if !ok {
		fail(fmt.Sprintf("open: no JetBrains IDE installed to open %s", p))
	}
	// Reuse an already-running IDE of the same product (any version) if there is one.
	target = ide.PreferRunning(installed, target)
	if err := launch.Open(target, p); err != nil {
		fail(fmt.Sprintf("open: %v", err))
	}
}

func lookupByPath(cfg config.Config, path string) (recent.Project, bool) {
	if path == "" {
		return recent.Project{}, false
	}
	for _, pr := range loadProjects(cfg) {
		if pr.Path == path {
			return pr, true
		}
	}
	return recent.Project{}, false
}

func cmdAction(args []string) {
	fs := flag.NewFlagSet("action", flag.ExitOnError)
	do := fs.String("do", "", "reveal|copy|terminal")
	path := fs.String("path", "", "project path")
	_ = fs.Parse(args)

	if *path == "" {
		fail("action: --path is required")
	}
	var err error
	switch *do {
	case "reveal":
		err = launch.Reveal(*path)
	case "copy":
		err = launch.CopyPath(*path)
	case "terminal":
		err = launch.Terminal(config.Load().Terminal, *path)
	case "command":
		err = launch.OpenCommand(config.Load().OpenCmd, *path)
	default:
		fail(fmt.Sprintf("action: unknown --do %q", *do))
	}
	if err != nil {
		fail(fmt.Sprintf("action %s: %v", *do, err))
	}
}

func cmdRefresh() {
	cfg := config.Load()
	files := discover.Find(cfg)
	ships := discover.FindShips(cfg)
	roots := effectiveProjectRoots(cfg)
	projects := recent.Merge(parseAll(cfg, files, ships), cfg.IgnoreContent)
	if scan := scanUnopened(roots); len(scan) > 0 {
		projects = recent.AppendUnopened(projects, scan, cfg.IgnoreContent)
	}
	cache.Save(cfg, cache.Fingerprint(cfg, files, ships, rootPaths(roots)), projects)
	fmt.Printf("cached %d projects from %d files\n", len(projects), len(files)+len(ships))
}

func cmdPin(args []string) {
	fs := flag.NewFlagSet("pin", flag.ExitOnError)
	path := fs.String("path", "", "project path to pin/unpin")
	_ = fs.Parse(args)
	if *path == "" {
		fail("pin: --path is required")
	}
	cfg := config.Load()
	st := state.Load(cfg.DataDir)
	if st.TogglePinned(*path) {
		// Now pinned: snapshot the IDE association from the live recents entry so
		// the pin still resolves to the right IDE (and icon) after it ages out.
		code, dataDir := "", ""
		if proj, ok := lookupByPath(cfg, *path); ok {
			code, dataDir = proj.ProductionCode, proj.SourceDataDir
		}
		st.SetPinMeta(*path, code, dataDir)
	} else {
		st.ClearPinMeta(*path)
	}
	if err := state.Save(cfg.DataDir, st); err != nil {
		fail(fmt.Sprintf("pin: %v", err))
	}
	reopenAlfred(cfg.DataDir)
}

func cmdForget(args []string) {
	fs := flag.NewFlagSet("forget", flag.ExitOnError)
	path := fs.String("path", "", "project path to hide from results")
	clear := fs.Bool("clear", false, "restore all forgotten projects")
	_ = fs.Parse(args)
	cfg := config.Load()
	st := state.Load(cfg.DataDir)
	switch {
	case *clear:
		st.ClearHidden()
		fmt.Println("restored all forgotten projects")
	case *path != "":
		st.Hide(*path)
		fmt.Printf("forgot %s\n", *path)
	default:
		fail("forget: --path or --clear is required")
	}
	if err := state.Save(cfg.DataDir, st); err != nil {
		fail(fmt.Sprintf("forget: %v", err))
	}
	if !*clear {
		reopenAlfred(cfg.DataDir)
	}
}

// cmdUpdate checks GitHub Releases for a newer version. With --check it emits
// Script Filter JSON (status / install row); with --apply it downloads the
// latest .alfredworkflow and opens it so Alfred imports the upgrade in place.
func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	check := fs.Bool("check", false, "emit Script Filter JSON describing update status")
	apply := fs.Bool("apply", false, "download and install the latest release")
	refresh := fs.Bool("refresh-cache", false, "background: refresh the cached release check")
	_ = fs.String("query", "", "ignored (accepted for Alfred compatibility)")
	_ = fs.Parse(args)

	// Background cache refresh (spawned by search); silent no-op on dev builds.
	if *refresh {
		if channel == "release" {
			update.RefreshCache(config.Load().DataDir)
		}
		return
	}

	// Self-update is only for official release builds; a source build manages
	// itself via git/make and must not be overwritten by a downloaded release.
	if channel != "release" {
		if *apply {
			fail("update: disabled for local builds — update via git pull && make install")
		}
		item := infoIcon("Workflow auto-update disabled (local build)", "Built from source — update with git pull && make install")
		emit([]alfred.Item{item})
		return
	}

	if *apply {
		updateApply()
		return
	}
	_ = *check // --check is the default behaviour
	updateCheck()
}

func updateCheck() {
	info := func(title, sub string) { emit([]alfred.Item{infoIcon(title, sub)}) }

	rel, err := update.Latest()
	if err != nil {
		info("Couldn't check for workflow updates", err.Error())
		return
	}
	if !update.IsNewer(rel.TagName, version) {
		info("Workflow is up to date ("+version+")", "JetBrains IDE Project Launcher — no update available")
		return
	}
	item := infoIcon("Install workflow update "+rel.TagName, "Update the JetBrains IDE Project Launcher workflow (you have "+version+")")
	item.Valid = alfred.BoolPtr(true)
	item.Arg = "apply"
	emit([]alfred.Item{item})
}

func updateApply() {
	// Notifications are posted by the workflow graph, not from here: a Post
	// Notification wired alongside this action shows "Downloading…" at the start,
	// and a second one downstream surfaces any error by showing this action's
	// stdout (suppressed when empty). So on failure we print a short message to
	// stdout and exit 0 — exit 0 lets Alfred run that downstream notification
	// rather than treating it as a script error — and on success we print nothing,
	// leaving Alfred's import sheet as the only confirmation. (Alfred's own
	// notification is reliable; an osascript one spawned via Alfred is not.)
	updateFail := func(msg string) {
		fmt.Println(msg)
		os.Exit(0)
	}
	rel, err := update.Latest()
	if err != nil {
		updateFail("Couldn't reach GitHub — " + err.Error())
	}
	url, ok := rel.WorkflowAsset()
	if !ok {
		// No packaged asset — open the release page so the user can grab it.
		_ = exec.Command("open", rel.HTMLURL).Run()
		updateFail("That release has no downloadable asset — opened its page instead")
	}
	path, err := update.Download(url)
	if err != nil {
		updateFail("Download failed — " + err.Error())
	}
	// Opening the .alfredworkflow hands it to Alfred, which imports it in place
	// (same bundle id), preserving config + pins/forgets.
	if err := exec.Command("open", path).Run(); err != nil {
		updateFail("Couldn't open the downloaded update — " + err.Error())
	}
}

// infoIcon is an update-related row carrying the workflow's own (Toolbox) icon,
// so update prompts read as "the launcher" rather than the IntelliJ fallback that
// iconPath("") resolves to.
func infoIcon(title, subtitle string) alfred.Item {
	item := alfred.Info(title, subtitle)
	item.Icon = &alfred.Icon{Path: workflowIcon()}
	return item
}

// workflowIcon is the absolute path to the bundle's Toolbox icon (icon.png at the
// bundle root) — absolute, like iconPath, so Alfred doesn't mis-resolve it through
// the install symlink. Falls back to the default icon only if icon.png is absent.
func workflowIcon() string {
	if wd, err := os.Getwd(); err == nil {
		if p := filepath.Join(wd, "icon.png"); fileExists(p) {
			return p
		}
	}
	return iconPath("")
}

// cmdDoctor prints a human-readable diagnostic of detected IDEs, config roots,
// and why projects are shown or hidden — for self-serve troubleshooting.
func cmdDoctor() {
	cfg := config.Load()
	files := discover.Find(cfg)
	ships := discover.FindShips(cfg)
	roots := effectiveProjectRoots(cfg)
	projects := recent.Merge(parseAll(cfg, files, ships), cfg.IgnoreContent)
	if scan := scanUnopened(roots); len(scan) > 0 {
		projects = recent.AppendUnopened(projects, scan, cfg.IgnoreContent)
	}
	installed := ide.Detect(cfg)
	st := state.Load(cfg.DataDir)

	line := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	mark := func(p string) string {
		if _, err := os.Stat(p); err == nil {
			return "ok"
		}
		return "missing"
	}

	line("jb %s (%s channel) — diagnostics\n", version, channel)
	line("Config roots:")
	for _, r := range cfg.ConfigRoots {
		line("  [%-9s] %s (%s)", r.Vendor, r.Dir, mark(r.Dir))
	}
	line("App roots:         %s", strings.Join(cfg.AppRoots, "  "))
	if len(roots) > 0 {
		src := "auto-detected"
		if len(cfg.ProjectRoots) > 0 {
			src = "JB_PROJECT_ROOTS"
		}
		line("Project roots:     %s  (%s)", strings.Join(rootPaths(roots), "  "), src)
	}
	for i, dir := range cfg.ToolboxDirs {
		label := "Toolbox scripts:"
		if i > 0 {
			label = "                "
		}
		line("%s   %s (%s)", label, dir, mark(dir))
	}
	line("Cache dir:         %s", cfg.CacheDir)
	line("Data dir:          %s", cfg.DataDir)
	line("Terminal app:      %s", cfg.Terminal)
	line("Sort order:        %s", cfg.Sort)
	line("Exclude worktrees: %v\n", cfg.ExcludeWorktrees)

	line("Installed IDEs (%d):", len(installed))
	for _, in := range installed {
		line("  %-20s %-10s %s", in.Display, in.Version, in.AppPath)
	}
	line("")
	line("Recent files (%d):", len(files))
	for _, f := range files {
		line("  %s", f.Path)
	}
	if len(ships) > 0 {
		line("Ship stores (%d):", len(ships))
		for _, s := range ships {
			line("  [%-5s] %s", s.Product, s.Path)
		}
	}
	line("")

	var shown, missing, stub, worktrees, hidden, ignored, unopened int
	for _, p := range projects {
		switch {
		case st.IsHidden(p.Path):
			hidden++
		case !p.Exists:
			missing++
		case p.Stub:
			stub++
		case matchesProjectIgnore(p.Path, cfg.IgnoreProjects):
			ignored++
		case p.IsWorktree:
			worktrees++
		case p.Unopened:
			unopened++ // shown only under the `+` variant
		default:
			shown++
		}
	}
	line("")
	line("Ignore content:  %s", strings.Join(cfg.IgnoreContent, ", "))
	line("Ignore projects: %s", strings.Join(cfg.IgnoreProjects, ", "))
	line("")
	line("Projects: %d merged", len(projects))
	line("  shown by default:      %d", shown)
	line("  hidden — missing dir:  %d", missing)
	line("  hidden — stub (no real content): %d", stub)
	line("  hidden — ignore pattern: %d", ignored)
	line("  hidden — worktree:     %d (use <keyword>~ to show)", worktrees)
	line("  un-opened (root scan): %d (use <keyword>+ to show)", unopened)
	line("  hidden — forgotten:    %d", hidden)
	line("  pinned:                %d", len(st.Pinned))
}

// openCmdMod renders the ⌃⇧ custom-open modifier. With a command configured it
// opens the project through it, labelled by the command's program (e.g. "Open in
// code"); with none set the row is inert and hints at configuring it.
func openCmdMod(cmd string) alfred.Mod {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return alfred.Mod{
			Subtitle: "Set a custom open command in the workflow config (e.g. code {path})",
			Valid:    alfred.BoolPtr(false),
		}
	}
	return alfred.Mod{Subtitle: "Open in " + openCmdName(cmd)}
}

// openCmdName derives a friendly program name from a command template: the base
// name of its first whitespace-separated token (e.g. "code {path}" → "code",
// "/usr/local/bin/cursor --new-window" → "cursor").
func openCmdName(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "custom command"
	}
	return filepath.Base(fields[0])
}

// iconForFamily renders an installed IDE's own live icon via Alfred's fileicon
// type (no bundled or extracted image — macOS draws the icon of the app you
// have). When the family's IDE isn't installed it falls back to the vendored
// brand-site icon, then default.png.
func iconForFamily(family string, installed []ide.Installed) *alfred.Icon {
	if family != "" {
		if app, ok := ide.NewestByFamily(installed, family); ok && app.AppPath != "" {
			return &alfred.Icon{Type: "fileicon", Path: app.AppPath}
		}
	}
	return &alfred.Icon{Path: iconPath(family)}
}

// iconPath returns an absolute path to the family's icon, falling back to
// default.png when that family has no icon. Absolute paths avoid Alfred
// mis-resolving bundle-relative icon paths through the install symlink.
func iconPath(family string) string {
	wd, err := os.Getwd()
	if err != nil {
		return alfred.IconPath(family) // relative fallback
	}
	p := filepath.Join(wd, "icons", family+".png")
	if family != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if def := filepath.Join(wd, "icons", "default.png"); fileExists(def) {
		return def
	}
	return p
}

// existingIcon returns the absolute path to icons/<name>.png if it exists, else
// "". Used to resolve task icons with a chain of fallbacks.
func existingIcon(name string) string {
	if name == "" {
		return ""
	}
	if wd, err := os.Getwd(); err == nil {
		if p := filepath.Join(wd, "icons", name+".png"); fileExists(p) {
			return p
		}
	}
	return ""
}

// iconPathOr returns icons/<name>.png if present, else icons/<fallback>.png if
// present, else default.png — for task rows that prefer a per-runner icon but
// fall back to the generic run icon.
func iconPathOr(name, fallback string) string {
	if p := existingIcon(name); p != "" {
		return p
	}
	if p := existingIcon(fallback); p != "" {
		return p
	}
	return iconPath("")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// matchesProjectIgnore reports whether a project path matches any project-level
// ignore glob, tested against both the folder name and the full path.
func matchesProjectIgnore(path string, patterns []string) bool {
	base := filepath.Base(path)
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
		if ok, _ := filepath.Match(p, path); ok {
			return true
		}
	}
	return false
}

// withDurablePins appends any pinned project that has aged out of the capped
// recents list, so a pin keeps it visible regardless of recents eviction. Pins
// already in the list, hidden, or whose folder no longer exists are skipped.
// These synthesised entries have no productionCode (it aged out too), so they
// surface under the unified `jb` keyword and resolve via the IDE fallback chain.
func withDurablePins(cfg config.Config, projects []recent.Project, st state.State) []recent.Project {
	if len(st.Pinned) == 0 {
		return projects
	}
	have := make(map[string]bool, len(projects))
	for _, p := range projects {
		have[p.Path] = true
	}
	for _, path := range st.Pinned {
		if have[path] || st.IsHidden(path) {
			continue
		}
		if p, ok := recent.ProjectFromPath(path, cfg.IgnoreContent); ok {
			// Restore the IDE association snapshotted at pin time so the durable
			// pin keeps its real icon, resolves to the right IDE, and matches its
			// per-IDE keyword.
			if pi, ok := st.PinInfoFor(path); ok {
				p.ProductionCode = pi.Code
				p.SourceDataDir = pi.DataDir
				if pi.Code != "" {
					p.AllCodes = []string{pi.Code}
				}
			}
			projects = append(projects, p)
		}
	}
	return projects
}

// sortProjects orders the shown projects per the configured sort mode. The
// merged/cached list is already most-recent-first, so "recency" is a no-op;
// other modes re-sort the slice stably in place. Pinned projects still float to
// the top in emitSearch, keeping this order within the pinned and rest groups.
func sortProjects(projects []recent.Project, mode string) {
	switch mode {
	case "recency-asc":
		sort.SliceStable(projects, func(i, j int) bool {
			return projects[i].Timestamp.Before(projects[j].Timestamp)
		})
	case "name":
		sort.SliceStable(projects, func(i, j int) bool { return lessByName(projects[i], projects[j]) })
	case "name-desc":
		sort.SliceStable(projects, func(i, j int) bool { return lessByName(projects[j], projects[i]) })
	case "path":
		sort.SliceStable(projects, func(i, j int) bool { return projects[i].Path < projects[j].Path })
	}
}

// lessByName orders by display name (case-insensitive), then path as a tiebreak.
func lessByName(a, b recent.Project) bool {
	an, bn := strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName)
	if an != bn {
		return an < bn
	}
	return a.Path < b.Path
}

func familyMatches(p recent.Project, family string) bool {
	if ide.FamilyOf(p.ProductionCode) == family {
		return true
	}
	for _, c := range p.AllCodes {
		if ide.FamilyOf(c) == family {
			return true
		}
	}
	return false
}

// emptyStateItem tailors the no-results row, distinguishing "you have no such
// projects" from "that IDE isn't even installed".
func emptyStateItem(product string, keywordInstalled bool) alfred.Item {
	var item alfred.Item
	switch {
	case product == "":
		item = alfred.Info("No recent JetBrains projects found", "Open a project in an IDE, then try again")
	case !keywordInstalled:
		name := ide.FamilyDisplay(product)
		item = alfred.Info(name+" is not installed", "No "+name+" projects in your recents either")
	default:
		item = alfred.Info("No "+ide.FamilyDisplay(product)+" projects found", "")
	}
	item.Icon = &alfred.Icon{Path: iconPath(product)}
	return item
}

func matchString(p recent.Project, family, ideLabel string) string {
	parts := []string{p.DisplayName, family, ideLabel}
	// Include path components so users can match on parent folders too.
	parts = append(parts, strings.FieldsFunc(p.Path, func(r rune) bool { return r == filepath.Separator })...)
	return strings.Join(parts, " ")
}

// keywordFor reconstructs the Alfred keyword from a product family plus the
// worktrees (`~`) / project-roots (`+`) variant flags. A Script Filter is exactly
// one variant, so at most one suffix applies.
func keywordFor(product string, worktrees, roots bool) string {
	kw := "jb"
	if product != "" {
		kw = product
	}
	switch {
	case worktrees:
		kw += "~"
	case roots:
		kw += "+"
	}
	return kw
}

type lastSearch struct {
	Keyword string `json:"keyword"`
	Query   string `json:"query"`
}

func lastSearchPath(dataDir string) string { return filepath.Join(dataDir, "lastsearch.json") }

// saveLastSearch records the current keyword + query so a subsequent pin/forget
// can re-open Alfred exactly where the user was.
func saveLastSearch(dataDir, keyword, query string) {
	if os.MkdirAll(dataDir, 0o755) != nil {
		return
	}
	if data, err := json.Marshal(lastSearch{Keyword: keyword, Query: query}); err == nil {
		_ = os.WriteFile(lastSearchPath(dataDir), data, 0o644)
	}
}

func loadLastSearch(dataDir string) lastSearch {
	ls := lastSearch{Keyword: "jb"}
	if data, err := os.ReadFile(lastSearchPath(dataDir)); err == nil {
		_ = json.Unmarshal(data, &ls)
	}
	if ls.Keyword == "" {
		ls.Keyword = "jb"
	}
	return ls
}

// reopenAlfred re-shows Alfred on the last keyword + query so pin/forget keep the
// window open in place. No-op outside Alfred (e.g. terminal CLI use).
func reopenAlfred(dataDir string) {
	if os.Getenv("alfred_workflow_bundleid") == "" {
		return
	}
	ls := loadLastSearch(dataDir)
	text := ls.Keyword + " " + ls.Query // trailing space (empty query) enters keyword arg mode
	script := `tell application id "com.runningwithcrayons.Alfred" to search ` + applescriptQuote(text)
	_ = exec.Command("osascript", "-e", script).Run()
}

func applescriptQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func emit(items []alfred.Item) {
	out, err := alfred.Render(items)
	if err != nil {
		fail(fmt.Sprintf("render: %v", err))
	}
	os.Stdout.Write(out)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "jb: "+msg)
	os.Exit(1)
}
