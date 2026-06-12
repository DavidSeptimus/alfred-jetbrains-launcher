package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/alfred"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/config"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/ide"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/recent"
	"github.com/davidseptimus/alfred-jetbrains-launcher/internal/state"
	taskrunner "github.com/davidseptimus/alfred-taskrunner"
)

type apiOutput[T any] struct {
	Items        []T     `json:"items"`
	RerunSeconds float64 `json:"rerunSeconds,omitempty"`
}

type apiProject struct {
	Title        string      `json:"title"`
	Subtitle     string      `json:"subtitle"`
	Path         string      `json:"path"`
	Match        string      `json:"match"`
	Family       string      `json:"family,omitempty"`
	IDE          string      `json:"ide,omitempty"`
	Valid        bool        `json:"valid"`
	Pinned       bool        `json:"pinned"`
	Worktree     bool        `json:"worktree"`
	ProjectRoot  bool        `json:"projectRoot"`
	Branch       string      `json:"branch,omitempty"`
	Icon         alfred.Icon `json:"icon"`
	OpenSpec     string      `json:"openSpec,omitempty"`
	TaskPickSpec string      `json:"taskPickSpec"`
}

type apiIDE struct {
	Title    string      `json:"title"`
	Subtitle string      `json:"subtitle"`
	Spec     string      `json:"spec"`
	Family   string      `json:"family"`
	Version  string      `json:"version"`
	Icon     alfred.Icon `json:"icon"`
}

type apiTask struct {
	Title       string      `json:"title"`
	Subtitle    string      `json:"subtitle"`
	Match       string      `json:"match"`
	Runner      string      `json:"runner,omitempty"`
	CommandLine string      `json:"commandLine,omitempty"`
	Cwd         string      `json:"cwd,omitempty"`
	Runnable    bool        `json:"runnable"`
	Kind        string      `json:"kind"`
	Spec        string      `json:"spec,omitempty"`
	WindowSpec  string      `json:"windowSpec,omitempty"`
	TabSpec     string      `json:"tabSpec,omitempty"`
	BGSpec      string      `json:"backgroundSpec,omitempty"`
	CopySpec    string      `json:"copySpec,omitempty"`
	ResetSpec   string      `json:"resetSpec,omitempty"`
	Icon        alfred.Icon `json:"icon"`
}

func cmdAPI(args []string) {
	if len(args) == 0 {
		fail("api: expected projects, ides, tasks, or rerun")
	}
	switch args[0] {
	case "projects":
		apiProjects(args[1:])
	case "ides":
		apiIDEs(args[1:])
	case "tasks":
		apiTasks(args[1:])
	case "rerun":
		apiRerun(args[1:])
	default:
		fail("api: unknown command " + args[0])
	}
}

func apiProjects(args []string) {
	fs := flag.NewFlagSet("api projects", flag.ExitOnError)
	product := fs.String("product", "", "IDE family hard-limit")
	variant := fs.String("variant", "recent", "recent|roots|worktrees")
	query := fs.String("query", "", "filter text")
	_ = fs.Parse(args)

	worktreesFlag, scanRoots := apiVariantFlags(*variant)
	cfg := config.Load()
	st := state.Load(cfg.DataDir)
	// Same cached, build-then-filter loader the Alfred search and runtask picker
	// use (recents + roots + worktrees, all folded in); projectInVariant below is
	// the sole visibility gate, so the two frontends can never drift on which
	// projects a variant shows.
	projects := withDurablePins(cfg, loadProjects(cfg), st)
	sortProjects(projects, cfg.Sort)
	installed := ide.Detect(cfg)
	roots := projectRootSet(cfg)
	keywordInstalled := true
	if *product != "" {
		_, keywordInstalled = ide.NewestByFamily(installed, *product)
	}

	var pinned, rest []apiProject
	for _, p := range projects {
		if !projectInVariant(p, st, cfg, roots, worktreesFlag, scanRoots) {
			continue
		}
		if *product != "" && !familyMatches(p, *product) {
			if !(p.Unopened && p.ProductionCode == "") {
				continue
			}
		}
		item := apiProjectItem(cfg, p, installed, st.IsPinned(p.Path), *product, keywordInstalled, worktreesFlag, scanRoots)
		if !queryMatches(*query, item.Title+" "+item.Match) {
			continue
		}
		if item.Pinned {
			pinned = append(pinned, item)
		} else {
			rest = append(rest, item)
		}
	}
	items := append(pinned, rest...)
	if items == nil {
		items = []apiProject{}
	}
	emitAPI(apiOutput[apiProject]{Items: items})
}

func apiIDEs(args []string) {
	fs := flag.NewFlagSet("api ides", flag.ExitOnError)
	path := fs.String("path", "", "project path")
	_ = fs.Parse(args)

	cfg := config.Load()
	installed := ide.Detect(cfg)
	sort.Slice(installed, func(i, j int) bool { return installed[i].Display < installed[j].Display })

	items := make([]apiIDE, 0, len(installed))
	for _, in := range installed {
		items = append(items, apiIDE{
			Title:    in.Display,
			Subtitle: "Open in " + in.Display + " " + in.Version,
			Spec:     in.Code + specSep + in.DataDir + specSep + *path,
			Family:   in.Family,
			Version:  in.Version,
			Icon:     alfred.Icon{Type: "fileicon", Path: in.AppPath},
		})
	}
	emitAPI(apiOutput[apiIDE]{Items: items})
}

func apiTasks(args []string) {
	fs := flag.NewFlagSet("api tasks", flag.ExitOnError)
	path := fs.String("path", "", "project path")
	query := fs.String("query", "", "filter text")
	_ = fs.Parse(args)
	if *path == "" {
		fail("api tasks: --path is required")
	}

	cfg := config.Load()
	tasks := detectProjectTasks(cfg, *path)
	refreshing := gradleEnumInFlight(cfg, *path)
	items := []apiTask{}
	if refreshing {
		items = append(items, apiTask{
			Title:    "Refreshing Gradle tasks...",
			Subtitle: "Re-enumerating in the background",
			Kind:     "info",
			Icon:     alfred.Icon{Path: iconPathOr("gradle", "run")},
		})
	} else {
		if gradleEnumErrored(cfg, *path) {
			items = append(items, apiTask{
				Title:    "Gradle task refresh failed",
				Subtitle: "Showing default tasks; refresh to retry",
				Kind:     "info",
				Icon:     alfred.Icon{Path: iconPathOr("gradle", "")},
			})
		}
		if refresh, ok := apiGradleRefreshTask(cfg, *path); ok {
			items = append(items, refresh)
		}
	}
	for _, t := range tasks {
		item := apiTaskItem(cfg, t)
		if !queryMatches(*query, item.Title+" "+item.Match) {
			continue
		}
		items = append(items, item)
	}
	out := apiOutput[apiTask]{Items: items}
	if refreshing {
		out.RerunSeconds = gradleRerunInterval
	}
	emitAPI(out)
}

func apiRerun(args []string) {
	fs := flag.NewFlagSet("api rerun", flag.ExitOnError)
	_ = fs.Parse(args)

	cfg := config.Load()
	cwd, cmdline, ok := loadLastRun(cfg)
	if !ok {
		emitAPI(apiOutput[apiTask]{Items: []apiTask{}})
		return
	}
	def := "tab"
	if cfg.TaskNewWindow {
		def = "window"
	}
	spec := func(kind string) string { return kind + specSep + cwd + specSep + cmdline }
	emitAPI(apiOutput[apiTask]{Items: []apiTask{{
		Title:       cmdline,
		Subtitle:    "Rerun in " + alfred.AbbreviateHome(cfg.Home, cwd),
		Match:       cmdline + " " + cwd,
		CommandLine: cmdline,
		Cwd:         cwd,
		Runnable:    true,
		Kind:        "task",
		Spec:        spec(def),
		WindowSpec:  spec("window"),
		TabSpec:     spec("tab"),
		BGSpec:      spec("bg"),
		CopySpec:    spec("copy"),
		ResetSpec:   spec(def + "reset"),
		Icon:        alfred.Icon{Path: iconPathOr("run", "")},
	}}})
}

func apiVariantFlags(variant string) (worktrees, roots bool) {
	switch strings.ToLower(strings.TrimSpace(variant)) {
	case "worktrees", "worktree", "~":
		return true, false
	case "roots", "unopened", "+":
		return false, true
	default:
		return false, false
	}
}

func apiProjectItem(cfg config.Config, p recent.Project, installed []ide.Installed, pinned bool, product string, keywordInstalled, worktreesFlag, scanRoots bool) apiProject {
	target, found := ide.Resolve(installed, p.ProductionCode, p.SourceDataDir, product)
	family := product
	if product == "" {
		family = ide.FamilyOf(p.ProductionCode)
	}
	matchLabel := ""
	ideName := ""
	if found {
		matchLabel = target.Display
		ideName = target.Display
	}

	branch := recent.GitBranch(p.Path)
	// The branch is carried in the Branch field and rendered as a tag by the
	// frontend, so it's left out of the subtitle here (passing "") to avoid showing
	// it twice. (Alfred has no tag space, so its subtitle still includes it.)
	subtitle := projectSubtitle(cfg, p, found, keywordInstalled, product, "")

	name := p.DisplayName
	if p.IsWorktree {
		name = worktreeGlyph + " " + name
	}

	return apiProject{
		Title:        name,
		Subtitle:     subtitle,
		Path:         p.Path,
		Match:        matchString(p, family, matchLabel),
		Family:       family,
		IDE:          ideName,
		Valid:        found,
		Pinned:       pinned,
		Worktree:     p.IsWorktree,
		ProjectRoot:  p.Unopened,
		Branch:       branch,
		Icon:         derefIcon(iconForFamily(family, installed)),
		TaskPickSpec: "picktask" + specSep + p.Path + specSep + variantSuffix(worktreesFlag, scanRoots),
	}
}

func apiGradleRefreshTask(cfg config.Config, path string) (apiTask, bool) {
	disabled := disabledRunners(cfg.TaskDisable)
	if runnerDisabled(disabled, taskrunner.RunnerGradle) || taskrunner.GradleFingerprint(path) == "" {
		return apiTask{}, false
	}
	return apiTask{
		Title:    "Refresh tasks",
		Subtitle: "Re-enumerate Gradle tasks",
		Match:    "refresh rescan reload gradle tasks",
		Runnable: true,
		Kind:     "refresh",
		Spec:     "refresh" + specSep + path,
		Icon:     alfred.Icon{Path: iconPathOr("gradle", "run")},
	}, true
}

func apiTaskItem(cfg config.Config, t taskrunner.Task) apiTask {
	cmdline := shellJoinArgv(t.Command)
	subtitle := taskSubtitle(t, cmdline)
	spec := func(kind string) string { return kind + specSep + t.Cwd + specSep + cmdline }
	def := "tab"
	if cfg.TaskNewWindow {
		def = "window"
	}
	return apiTask{
		Title:       t.Name,
		Subtitle:    subtitle,
		Match:       t.Name + " " + string(t.Runner) + " " + t.Source,
		Runner:      string(t.Runner),
		CommandLine: cmdline,
		Cwd:         t.Cwd,
		Runnable:    t.Runnable,
		Kind:        "task",
		Spec:        spec(def),
		WindowSpec:  spec("window"),
		TabSpec:     spec("tab"),
		BGSpec:      spec("bg"),
		CopySpec:    spec("copy"),
		ResetSpec:   spec(def + "reset"),
		Icon:        alfred.Icon{Path: iconPathOr(string(t.Runner), "run")},
	}
}

func derefIcon(icon *alfred.Icon) alfred.Icon {
	if icon == nil {
		return alfred.Icon{}
	}
	return *icon
}

func emitAPI(v any) {
	out, err := json.Marshal(v)
	if err != nil {
		fail(fmt.Sprintf("api render: %v", err))
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fail(fmt.Sprintf("api write: %v", err))
	}
}
