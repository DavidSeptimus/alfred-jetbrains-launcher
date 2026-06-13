# JetBrains IDE Project Launcher

An [Alfred 5](https://www.alfredapp.com/) workflow that opens your recent
JetBrains projects — across **every installed IDE and every installed version**.

Most launchers read only the *newest* version directory of each IDE, so projects
you opened in an older version (or in anything other than your highest-numbered
install) silently disappear. JetBrains also caps each `recentProjects.xml` at
~50 entries. This workflow merges **all** of them, so your projects actually
show up.

The flip side of "show everything" is noise, which is the other reason this
exists: JetBrains' recent list fills up with linked git **worktrees** and
projects whose folders you've since **deleted** — entries you'd never want to
launch. The workflow hides those by default (worktrees are one keystroke away
when you *do* want them), so the list stays to the projects you'd actually
reopen.

There's also a **Raycast** edition: a sibling extension in
[`raycast/jetbrains-project-launcher/`](raycast/jetbrains-project-launcher/) that
drives the same `jb` binary, so both launchers share project discovery, filtering,
and task-running. The rest of this README documents the Alfred workflow.

> Not affiliated with, sponsored by, or endorsed by JetBrains. See
> [Trademarks](#trademarks--attribution).

![Searching recent projects with the jb keyword — a pinned project on top, each result showing its IDE icon and git branch](docs/img/search.png)

---

## Features

- **Complete discovery** — merges `recentProjects.xml` from every version
  directory of every classic IDE, plus Android Studio, plus Fleet and Air
  workspaces, deduplicated by path and sorted by most-recently used.
- **Opens in the right IDE** — each project opens in the IDE it was last used in;
  if a different version of that IDE is already running, it reuses it.
- **Unified + per-IDE keywords** — `jb` for everything, or `idea` / `goland` /
  `pycharm` / … to scope to one IDE.
- **Project-root search too** — the `+` variant (`jb+`, `idea+`) surfaces
  freshly-cloned projects from your project roots, auto-detecting your
  `~/<IDE>Projects` folders (override via config) and kept out of the default list.
- **A dedicated worktree list** — the `~` variant (`jb~`, `idea~`) shows *only*
  your linked **git worktrees**, found on disk via git (wherever they live — e.g.
  `.worktrees/…`), not just ones you've already opened, kept out of the default
  list.
- **Quick actions** — reveal in Finder, copy path, open in a terminal, pick a
  different IDE, or run your own open command (VS Code, Cursor, …), all from
  modifier keys.
- **Run build tasks** — the `runtask` keyword (or ⌥⇧ on any project) finds and
  runs a project's tasks in a terminal: npm/pnpm/yarn/bun, Make, just, Taskfile,
  **Gradle** (its *real* tasks, including custom ones like `runIde`), Maven,
  Composer, Deno, Rake, Cargo, Go, and .NET — with `rerun` to repeat the last one.
- **Tidy by default** — hides stale entries whose folder is gone, stub dirs with
  no visible files (only leftover `.idea`/`.git`/dotfiles), and linked git
  worktrees (with an opt-in to show them).
- **Pin & forget** — pin frequent projects to the top (★), or forget ones you
  don't want cluttering the list (reversible; never touches JetBrains' files).
- **Git branch** shown inline for projects in a git checkout.
- **Native & open source** — one static Go binary with an mtime-keyed cache:
  fast, no Python or Node runtime to install, nothing else to set up, and
  [MIT-licensed](LICENSE) end to end.

---

## Requirements

- macOS, Alfred 5 with the **Powerpack**.
- One or more JetBrains IDEs (standalone or via JetBrains Toolbox).
- To build from source: Go 1.23+.

---

## Installation

### From a release

Download `jb-<version>.alfredworkflow` and double-click it to import.

macOS tags any browser download as quarantined, and Gatekeeper blocks the
workflow's (ad-hoc-signed) binary on first launch. The workflow clears its own
quarantine flag the first time you trigger it — from inside Alfred, no Terminal
step: Alfred runs the Script Filter through the system shell (which isn't gated),
so it strips the flag before launching the binary.

If the binary somehow stays blocked (results never appear), clear it by hand
once. Alfred imports each workflow into a randomly-named `user.workflow.<UUID>`
folder, so locate ours by its bundle id (stored in every workflow's `info.plist`)
and clear only that folder — never the whole workflows directory:

```sh
wf=$(grep -l com.davidseptimus.jetbrains-launcher \
  "$HOME/Library/Application Support/Alfred/Alfred.alfredpreferences/workflows"/*/info.plist | head -1)
[ -n "$wf" ] && /usr/bin/xattr -dr com.apple.quarantine "$(dirname "$wf")"
```

(`/usr/bin/xattr` is spelled out so a pyenv/conda `xattr` on your `PATH` — which
lacks `-r` — can't shadow the macOS built-in.)

### From source

```sh
git clone https://github.com/davidseptimus/alfred-jetbrains-launcher.git
cd alfred-jetbrains-launcher
make install      # build (arm64) + generate info.plist + stage icons + symlink into Alfred
```

`make install` symlinks the built bundle into Alfred's workflows directory, so
later `make build` runs are live immediately.

### Updating

When a newer release exists, an **"Update available" banner** appears at the top
of the `jb` results — **press ↩ on it to update in place** (your config, pins, and
forgotten projects are preserved). A background check runs about once a day, so
the banner shows up within a day of a release. The update downloads via the binary
(not a browser), so the new workflow isn't quarantined — it's seamless. (A
*manual* browser download of the `.alfredworkflow` is quarantined, but the
workflow clears that itself on first run, as described above.)

Self-update only applies to **released builds**. A build from source (`make
build`/`make install`) omits the update banner entirely — update it with
`git pull && make install` instead, so your working copy is never overwritten.
This is controlled by a build-time `channel` flag (`dev` by default; `make dist`
sets `release`).

---

## Usage

| Type                                                                                                                                                    | What you get                                                                                                                                |
|---------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------|
| `jb <query>`                                                                                                                                            | All recent projects, each opening in its last-used IDE                                                                                      |
| `idea`, `pycharm`, `webstorm`, `goland`, `clion`, `rubymine`, `datagrip`, `phpstorm`, `rider`, `rustrover`, `studio`, `dataspell`, `aqua`, `writerside` | Scoped to that IDE                                                                                                                          |
| `fleet`, `air`                                                                                                                                          | Scoped to Fleet / Air workspaces                                                                                                            |
| `<keyword>~`                                                                                                                                            | A dedicated list of **just your git [worktrees](#git-worktrees-the--variant)** — discovered on disk, not only opened ones (`jb~`, `goland~`, …) |
| `<keyword>+`                                                                                                                                            | The same search, **plus projects** found in your configured [project roots](#projects-the--variant) (`jb+`, `idea+`, …) |

Alfred fuzzy-matches your query against the project name and its path
components, so `jb webfoo` finds `~/work/web/foo`.

#### Projects (the `+` variant)

The plain keywords come from your IDEs' recents — a project is only listed once
an IDE has *opened* it. So a freshly `git clone`d repo you've never opened is
invisible. The `+` variant is the broader **Projects** list: recents plus every
immediate subfolder of your **project roots**.

By default it **auto-detects** the conventional JetBrains folders under your home
— `~/<IDE>Projects` for the classic IDEs and Android Studio (`~/IdeaProjects`,
`~/GolandProjects`, `~/AndroidStudioProjects`, …) and `~/<IDE>Workspaces` for
Fleet and Air — matched case-insensitively, and only those that actually exist.
Set **Project roots** (`JB_PROJECT_ROOTS`) to a `:`-separated list to point it
somewhere else instead.

Because an auto-detected folder names its IDE, a project-root entry **opens in
the IDE its root implies** — a folder in `~/GolandProjects` opens in GoLand, even
under unified `jb+` — falling back through the [resolution
chain](#which-ide-opens-a-project) when that IDE isn't installed. Folders from a
custom `JB_PROJECT_ROOTS` imply no IDE: under `jb+` they use the fallback chain,
and you can steer them with a **per-IDE `+` keyword** (`goland+` opens the
highlighted folder in GoLand).

Project-root-only results stay **out of the plain `jb` list**, appearing only when you ask
with `+` — with one exception: **pin** one (⌘⇧) and it's
promoted into the normal list too, ★-pinned, just like a durable pin that has
aged out of recents. **Forget** one (⌘⌥) and it's hidden from `jb+`; that hide is
durable and path-keyed, so if you later actually open the project it **stays
hidden** from your recents until you restore it with `jb forget --clear`. Once an
a project-root entry is opened in an IDE it simply becomes a normal recent, carrying
whatever pin/forget state you'd attached to it.

#### Git worktrees (the `~` variant)

Linked git **worktrees** are hidden from the default list — they'd otherwise
flood your recents with one entry per branch. The `~` variant (`jb~`, `idea~`, …)
is a **dedicated worktree list**: it shows *only* worktrees (no regular projects),
and it discovers them **on disk**, not just the ones you've opened — for every
project it knows (your recents and `+` project roots) it asks git for that repo's
worktrees and lists them all, including never-opened ones.

The three keywords give you three distinct lists: `jb` is your recents, `jb+` is
your projects (recents + project roots, no worktrees), and `jb~` is your worktrees.
Worktree rows are still marked with a **`⑂`** glyph in their title (after the `★`
pin marker if pinned) so they're recognisable when `JB_EXCLUDE_WORKTREES` is off
and recent worktrees mix into the plain `jb` list; you can also type `worktree`
in the query to filter to them.

This matters because worktrees rarely sit where a folder scan would find them —
they commonly live in a dot-dir *inside* the repo (e.g.
`myrepo/.worktrees/<branch>`), which the `+` scan deliberately skips. Reading
them straight from git finds every worktree wherever it actually lives. It's
cheap: a repo only has worktrees once git records them, so repos without any are
skipped without running git at all.

A discovered worktree **opens in the same IDE as its parent repo**, and an
already-opened worktree keeps its own real IDE association and recency.

Disk discovery is exclusive to `~` — just as project-root entries are exclusive to
`+`. The default `jb` list mirrors your IDE recents, so unticking **Exclude git
worktrees** in the workflow config (`JB_EXCLUDE_WORKTREES`) only stops *recent*
(already-opened) worktrees from being filtered out of every search; worktrees
that exist only on disk still appear solely under `~`.

### Modifier keys (on a highlighted result)

| Key | Action                                                                                                            |
|-----|-------------------------------------------------------------------------------------------------------------------|
| ↩   | Open in the resolved IDE                                                                                          |
| ⌘   | Reveal in Finder                                                                                                  |
| ⌥   | Open in a different IDE (pick from installed)                                                                     |
| ⌃   | Copy project path                                                                                                 |
| ⇧   | Open in terminal (configurable app)                                                                               |
| ⌃⇧  | Open with a custom command (`JB_OPEN_CMD`, e.g. VS Code) — off until set                                          |
| ⌘⇧  | Pin / unpin (pinned float to the top, marked ★) — stays open, list refreshes                                      |
| ⌘⌥  | Forget — hide from the launcher (stays open; `jb forget --clear` restores)                                        |
| ⌥⇧  | Run a build-system task in this project — jumps into `runtask` scoped to it (see [Running tasks](#running-tasks)) |

### Which IDE opens a project

1. The IDE recorded for that project (`productionCode`), in the **exact version**
   that last opened it, if installed.
2. The **newest installed version** of that same product.
3. **IntelliJ IDEA Ultimate** (latest), when the project type is first-class in
   IDEA (Java/Kotlin/web/Python/Go/PHP/DB/Ruby).
4. The newest installed IDE that fits — otherwise nothing (reveal / copy still
   work).

Then, if a *different* version of the resolved product is already running, the
project opens in that running version (rather than spawning another).

Per-IDE keywords hard-limit to their IDE; if that IDE isn't installed, they fall
back to the chain above and label the result accordingly.

#### Open in a different IDE

That resolution is only the default — press **⌥** on any result to override it and
pick from your installed IDEs:

![Pressing ⌥ on a result drills down to a picker of installed IDEs, reopening the project in the one you choose](docs/img/demo.gif)

---

## Running tasks

Beyond opening a project, the workflow can **run its build-system tasks** in a
terminal. Two ways in:

- **`runtask`** — a standalone keyword: pick a project, then pick a task. Both
  steps filter as you type. Its project picker mirrors `jb` (your IDE recents),
  and it takes the same `+`/`~` modifiers: **`runtask+`** also lists projects
  from your roots, and **`runtask~`** is a git-worktree picker — each
  exactly the project set the matching `jb`/`jb+`/`jb~` keyword shows. (The
  modifiers always open the picker, even if you'd already scoped `runtask` to a
  project — they're how you say "let me pick a different one".)
- **⌥⇧ on any `jb` result** — jumps straight into that project's tasks.

![Typing runtask to pick a project, then choosing a detected build task to run it in a terminal](docs/img/runtask.gif)

It detects tasks from whatever the project uses, with no setup:

| Build system            | Source                                       | Notes                                                                                                                                                           |
|-------------------------|----------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| npm / pnpm / yarn / bun | `package.json`                               | picks the package manager from your lockfile                                                                                                                    |
| Make                    | `Makefile`                                   |                                                                                                                                                                 |
| just · Taskfile · Rake  | `justfile` · `Taskfile.yml` · `Rakefile`     | needs the tool installed                                                                                                                                        |
| **Gradle**              | `build.gradle[.kts]`                         | runs `./gradlew tasks` to list the project's **real** tasks (including custom ones like `runIde`, `buildPlugin`) — cached, so it's instant after the first time (use **↻ Refresh tasks** to rescan) |
| Maven                   | `pom.xml`                                    | common lifecycle goals                                                                                                                                          |
| Cargo · Go · .NET       | `Cargo.toml` · `go.mod` · `*.csproj`/`*.sln` | common commands (`build`/`test`/`run`/…)                                                                                                                        |
| Composer · Deno         | `composer.json` · `deno.json[c]`             | scripts / tasks                                                                                                                                                 |

A task whose tool isn't on your `PATH` still shows, but greyed (you can still
copy its command). Tasks run in your **login shell**, so anything on your `PATH`
resolves.

**Refreshing Gradle tasks.** Gradle is the one runner whose list is cached (every
other runner is re-read from disk each keystroke). When the cache might be stale —
after you add or rename a task — pick the **↻ Refresh tasks** row to rescan. It
re-enumerates in the background while a live *Refreshing Gradle tasks…* row shows,
and the list updates itself the moment the rescan lands (no need to retype). If
the rescan fails (e.g. a broken build), it falls back to the default Gradle verbs
and shows a brief error row instead of hanging. The cache also auto-refreshes when
the build files change or after 24h, so manual refresh is only for the in-between
cases.

### Launching a task

Each task runs in its own terminal session, so you can fire several in parallel.
Modifiers on a task:

| Key | Action                                                             |
|-----|--------------------------------------------------------------------|
| ↩   | Run in a new terminal **tab**                                      |
| ⌘   | Run in a new **window**                                            |
| ⌥   | Run in the **background** (no terminal; notifies when it finishes) |
| ⌃   | **Copy** the command to the clipboard                              |
| ⇧   | Run, then **reset** to the project picker                          |

`runtask` **stays scoped to the last project** you ran something in — reopen it
and you're back on that project's tasks (handy for iterating). Pick **⬅ Switch
project** (or use ⇧ above) to go back to the picker — and if you'd reached the
project through `runtask+`/`runtask~`, *Switch project* returns you to that same
widened picker, not just your recents. The **`rerun`** keyword re-runs your most
recent task directly.

The launch terminal is configurable: **Terminal.app**, **iTerm2**, and
**Ghostty** are built in, or set a `JB_TASK_TERMINAL_CMD` template for any other
terminal (kitty, WezTerm, …). The **Task window** toggle (`JB_TASK_WINDOW`) flips
the ↩/⌘ default if you'd rather a new window than a tab. Real Terminal.app /
Ghostty *tabs* use a System Events keystroke, which asks for Accessibility
permission the first time.

> Task detection isn't JetBrains-specific — it works for any project in your
> recents. Clear the `runtask` / `rerun` keyword fields (or `JB_TASK_DISABLE` a
> runner) if you don't want it.

---

## Configuration

Open **Configure Workflow…** in Alfred:

![The Configure Workflow panel — worktrees, terminal app, sort order, ignore patterns, and path overrides](docs/img/configure.png)

| Setting               | Variable               | Default                          | Effect                                                                                                                                                                                                                                                                           |
|-----------------------|------------------------|----------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Exclude git worktrees | `JB_EXCLUDE_WORKTREES` | on                               | Hide linked git worktrees that are in your recents; untick to keep those in every search. On-disk worktrees (incl. never-opened ones) appear only under the [`<keyword>~` variant](#git-worktrees-the--variant)                                                                    |
| Terminal app          | `JB_TERMINAL`          | Terminal                         | App for the ⇧ open-in-terminal action (iTerm, Warp, Ghostty, …)                                                                                                                                                                                                                  |
| Custom open command   | `JB_OPEN_CMD`          | _(none)_                         | Command for the ⌃⇧ action; `{path}` → project path (quoted for you). Runs in your login shell. See [Custom open command](#custom-open-command)                                                                                                                                   |
| Task terminal         | `JB_TASK_TERMINAL`     | Terminal                         | Terminal the `runtask` keyword launches into (Terminal, iTerm, Ghostty)                                                                                                                                                                                                          |
| Task window           | `JB_TASK_WINDOW`       | off                              | Run tasks in a new window instead of a tab by default (↩ ↔ ⌘↩ swap)                                                                                                                                                                                                              |
| Custom task terminal  | `JB_TASK_TERMINAL_CMD` | _(none)_                         | Launch tasks in any terminal: `{cmd}` (raw) / `{cwd}` / `{name}`. Launch via `open` (Alfred's PATH is minimal) and source your shell rc so the task's PATH loads, e.g. `open -na kitty.app --args --hold -d {cwd} /bin/zsh -lc "source ~/.zshrc; {cmd}"`. Overrides the built-in |
| Disable task runners  | `JB_TASK_DISABLE`      | _(none)_                         | Comma-separated build systems to skip (npm, make, just, task, composer, deno, rake, gradle, maven, cargo, go, dotnet)                                                                                                                                                            |
| Sort order            | `JB_SORT`              | Most recent first                | Result order: recency / least-recent / name (A–Z, Z–A) / path. Alfred re-ranks by relevance once you type a query                                                                                                                                                                |
| Ignore content        | `JB_IGNORE_CONTENT`    | `build,dist,node_modules`        | Comma-separated entry-name globs treated as non-content. A project whose only contents are these (plus hidden files) is hidden as a stub                                                                                                                                         |
| Ignore projects       | `JB_IGNORE_PROJECTS`   | _(none)_                         | Comma-separated globs matched against a project's name and full path; matches are hidden (e.g. `*-scratch`, `~/Downloads/*`)                                                                                                                                                     |
| Config roots          | `JB_CONFIG_ROOTS`      | standard JetBrains & Google dirs | `:`-separated dirs holding per-version IDE config dirs                                                                                                                                                                                                                           |
| Application folders   | `JB_APP_ROOTS`         | `/Applications:~/Applications`   | `:`-separated folders scanned for JetBrains `.app` bundles                                                                                                                                                                                                                       |
| Project roots         | `JB_PROJECT_ROOTS`     | auto-detected `~/<IDE>Projects`  | `:`-separated dirs whose immediate subfolders are offered via the `<keyword>+` Projects variant. Empty = auto-detect the conventional folders; set to override                                                                                                                   |
| Toolbox script dirs   | `JB_TOOLBOX_DIR`       | standard Toolbox scripts dir     | `:`-separated dirs of Toolbox launcher scripts                                                                                                                                                                                                                                   |

The path fields are **pre-filled with their defaults**, so you can see and edit
the exact values; clear a field to restore its default. `jb doctor` prints the
resolved list.

### Custom open command

`↩` always opens the resolved JetBrains IDE; **⌃⇧** runs whatever you put in
`JB_OPEN_CMD` instead — open the project in a different editor, a terminal
multiplexer, or your own script. Two tokens are substituted (both already quoted,
so leave them bare): **`{path}`** → the project path, **`{name}`** → its folder
name. With no `{path}` token the path is appended as the last argument. The
command runs in your **login shell**, so anything on your `PATH` resolves — and
since it's just a shell line, a path to a script works too.

| Tool            | `JB_OPEN_CMD`                         |
|-----------------|---------------------------------------|
| VS Code         | `code {path}`                         |
| Cursor          | `cursor {path}`                       |
| Zed             | `zed {path}`                          |
| Your own script | `~/bin/open-project.sh {name} {path}` |

Until you set it, the ⌃⇧ row stays visible but inert (it tells you to configure
it). If a CLI isn't on your login-shell `PATH`, use its absolute path.

### Keywords

Every keyword (`jb`, `idea`, …, `studio`, `air`, plus `runtask` and `rerun`) has
its own field in the same panel, so you can rename any that you like. This matters when a keyword is also a
word Alfred's default search matches: typing `studio`, for example, mixes your
projects with file/app hits for *Visual **Studio** Code* and other "studio"
files — rename it to something distinctive like `astudio` and it triggers
cleanly (its `~` worktree and `+` project-roots variants follow). Clear a field to disable that
keyword. These overrides live in the workflow's configuration, so they **persist
across updates** (editing the keyword node in the editor directly would be reset
on the next update).

---

## What's shown (and what isn't)

**Shown:** classic IDEs with a `recentProjects.xml` — IntelliJ IDEA (Ultimate &
Community), PyCharm (Pro & Community), WebStorm, GoLand, CLion, RubyMine,
DataGrip, PhpStorm, Rider, RustRover, DataSpell, Aqua, Writerside — plus Android
Studio, plus **Fleet** and **Air** (whose recent *workspaces* are read from their
`recent_ships.*.json` store; scratch sessions and remote/agent ships are skipped).

**Hidden:**

- Projects whose directory no longer exists on disk.
- Stub directories with no visible files (only hidden entries like `.idea` or
  `.git` remain — e.g. a removed worktree), and empty directories.
- Leftover directories that aren't a project anymore — anything with **no git
  checkout of its own that also isn't a direct child of one of your project
  roots** (e.g. a **removed worktree's husk** left behind with only build output
  and `.idea`). Real repos and live worktrees keep their `.git`, so they're never
  caught; a non-git folder you keep directly under a project root is still shown.
- Linked git worktrees, unless you use a `~` keyword or untick the setting.
- Remote-dev / devcontainer entries (detected and skipped).

**Not yet supported:** JetBrains Gateway (remote development), AppCode
(discontinued by JetBrains in 2022), and the legacy `recentPaths`/`<list>` XML
schema that much older IDEs wrote.

## Supported versions

There's no per-IDE version list to maintain: discovery reads whichever
`recentProjects.xml` files carry the modern `additionalInfo` →
`RecentProjectMetaInfo` map — the format current JetBrains IDEs and Android Studio
write, which the IntelliJ platform settled on around the **2019.2 / 2019.3**
releases (the older `recentPaths` list lingered alongside it for a while, so the
exact cutoff is fuzzy). Files that carry only the legacy `recentPaths`/`<list>`
schema are detected and skipped, so a much older IDE's entries simply won't
appear. Fleet and Air are read from their `recent_ships.*.json` workspace store
instead of XML.

---

## Verifying without Alfred

The binary speaks Alfred Script Filter JSON on stdout:

```sh
./build/jb-bundle/jb search | jq '.items | length'
./build/jb-bundle/jb search --product goland | jq '.items[].title'
./build/jb-bundle/jb search --worktrees | jq '.items | length'
./build/jb-bundle/jb tasks --path ~/myproject | jq '.items[].title'   # build-system tasks detected for a project
./build/jb-bundle/jb tasks --runtask | jq '.items[].title'           # what the `runtask` keyword emits (project picker, or the last project's tasks)
./build/jb-bundle/jb refresh         # rebuild the cache
./build/jb-bundle/jb doctor          # diagnostics: detected IDEs, roots, why things are hidden
```

The same binary also exposes a **frontend-neutral JSON** interface — `jb api
projects|ides|tasks|rerun` — which is what the Raycast extension consumes (it
shares the discovery, filtering, and task-running core; only the output shape
differs from the Alfred Script Filter JSON above):

```sh
./build/jb-bundle/jb api projects --variant recent | jq '.items | length'
./build/jb-bundle/jb api projects --variant worktrees | jq '.items[].title'
./build/jb-bundle/jb api tasks --path ~/myproject | jq '.items[].title'
```

---

## Development

The non-obvious control flows — project discovery, IDE resolution, the cached
update check, and the first-run quarantine self-heal — are diagrammed in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

```
Shared core (repo root) drives two frontend siblings — alfred/ and raycast/:

cmd/jb            shared backend: search / ides / open / action / tasks / runtask / refresh (Alfred), plus api (frontend-neutral JSON for Raycast)
internal/discover find every recent file across all version dirs
internal/recent   parse + merge/dedupe (worktree, .idea-only, existence checks)
internal/ide      product catalogue, installed-IDE detection, resolution, running check
internal/launch   open / reveal / copy / terminal / custom open command
internal/tasklaunch  run a task in a terminal tab/window, background, or copy
internal/alfred   Script Filter JSON (and shared helpers like AbbreviateHome/Icon)
internal/cache    mtime-keyed cache of the merged list
taskrunner/       standalone, Alfred-agnostic module: detect build-system tasks (own go.mod)
alfred/cmd/genplist      generates info.plist + per-object canvas icons from alfred/workflow/ides.json
alfred/workflow/ides.json  the IDE/keyword table that drives the generated plist
raycast/jetbrains-project-launcher  Raycast extension (TypeScript) over the same jb binary (own go.mod; build via scripts/prepare-backend.mjs)
assets/icons      vendored fallback IDE icons; assets/icon.png is the workflow icon
scripts/gen-task-icons.sh  (re)generate the task-runner icons from JetBrains' icon set
```

The task runner's design and the `runtask` flow are documented in
[docs/TASK-RUNNER.md](docs/TASK-RUNNER.md).

| Target                   | Does                                                                       |
|--------------------------|----------------------------------------------------------------------------|
| `make build`             | arm64 binary into the bundle (fast dev)                                    |
| `make build-universal`   | fat arm64+amd64 binary (releases)                                          |
| `make plist`             | regenerate `info.plist` + per-object icons                                 |
| `make icons`             | stage the vendored fallback icons into the bundle                          |
| `make bundle`            | assemble + ad-hoc codesign + de-quarantine                                 |
| `make install`           | symlink the bundle into Alfred                                             |
| `make dist`              | package `dist/jb-<version>.alfredworkflow`                                 |
| `make test` / `make vet` | `go test ./...` / `go vet ./...`                                           |
| `make hooks`             | enable the Conventional Commits hook (`core.hooksPath=.githooks`)          |
| `make changelog`         | regenerate `CHANGELOG.md` from commits (needs `git-cliff`)                 |
| `make wipe-update-cache` | delete the cached release check so `jb` re-checks now (keeps pins/forgets) |

`info.plist` is **generated** (deterministic UUIDv5 UIDs) — edit
`alfred/workflow/ides.json`, not the plist.

### Cutting a release

Releases are cut entirely by GitHub Actions — there's no local release step.
In the repo, go to **Actions → release → Run workflow** and choose the bump:
**`auto`** (default) derives the next version from the [Conventional
Commits](CONTRIBUTING.md) since the last tag (`fix` → patch, `feat` → minor,
breaking → major), or force `patch` / `minor` / `major`. The job regenerates
`CHANGELOG.md`, builds the universal `.alfredworkflow`, commits the version +
changelog, tags it, and publishes a GitHub Release whose notes are that version's
changelog section (the in-app update banner then surfaces it).

Commit messages follow Conventional Commits (enforced by a hook + CI) and drive
both the changelog and the version bump — see [CONTRIBUTING.md](CONTRIBUTING.md).

![The generated workflow object graph in Alfred's editor — a Script Filter per keyword wired to shared Run Script actions](docs/img/workflow.png)

---

## Trademarks & attribution

This is an independent, community-built workflow and is **not affiliated with,
sponsored by, or endorsed by JetBrains s.r.o.**

The bundled IDE logos are JetBrains product logos and the JetBrains Toolbox logo
(© JetBrains s.r.o.), used for identification only in accordance with the
[JetBrains Website Terms of Use](https://www.jetbrains.com/legal/docs/company/useterms/)
and [Brand Guidelines](https://www.jetbrains.com/company/brand/). Icons for IDEs
you have installed — including Android Studio, Fleet, and Air — are drawn by
macOS from the application itself and aren't bundled.

The task runner's icons are JetBrains' IntelliJ icons (from the local IDE under
the Apache 2.0 license, and from the public [IntelliJ Icons
catalog](https://intellij-icons.jetbrains.design)); the underlying tool marks
(npm, Gradle, Maven, Go, Rust, PHP, Ruby, .NET) are trademarks of their projects,
shown for identification only. See
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).

Inspired by [bchatard/alfred-jetbrains](https://github.com/bchatard/alfred-jetbrains).

---

## AI-generated code

This workflow was written by Claude Code with human oversight and testing.

---

## License

[MIT](LICENSE) — applies to the source code only. The bundled logos are not
covered by the MIT license; see [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).
