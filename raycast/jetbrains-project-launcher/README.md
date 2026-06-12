# JetBrains Project Launcher

Search and open JetBrains projects from Raycast.

This extension is a Raycast front end for the same Go backend used by the
Alfred workflow. It bundles a universal macOS `jb` binary, keeps Raycast state
under Raycast's extension support directory, and reuses the backend's project
discovery, IDE resolution, pinning, task running, and terminal integrations.

## Features

- Search recent JetBrains projects by name, path, IDE, or Git branch.
- Open a project in the best matching installed JetBrains IDE.
- Pick a different installed IDE for the selected project.
- Search project-root directories that have not been opened in an IDE yet.
- Discover linked Git worktrees.
- Pin and unpin projects.
- Forget projects from launcher state.
- Open a project in Finder or a terminal.
- Run project tasks discovered by Gradle, npm, Make, and other supported backend
  task runners.
- Rerun the most recent task.
- Use a custom open command for non-JetBrains editors or project tools.

## Search Projects

Run the `Search Projects` command in Raycast.

The search bar filters through the backend, so results can match project names,
paths, IDE metadata, and branch names. Raycast's built-in filtering is disabled
for the main project list so the backend can preserve its own ranking and
matching behavior.

Use the scope dropdown to choose both the source and the IDE family:

- `Recent`: recently opened JetBrains projects.
- `Projects`: recent projects plus immediate child directories from the
  configured project roots.
- `Worktrees`: linked Git worktrees.
- `All IDEs`: search across all supported IDE families.
- Product-specific scopes: IntelliJ IDEA, PyCharm, WebStorm, GoLand, CLion,
  RubyMine, DataGrip, PhpStorm, Rider, RustRover, Android Studio, DataSpell,
  Aqua, Writerside, Fleet, and Air.

Two Alfred-style query prefixes are also supported:

- Start a query with `+` to temporarily use `Projects`.
- Start a query with `~` to temporarily use `Worktrees`.

The prefix is stripped before searching. For example, `~ api` searches worktrees
for `api`, and `+ launcher` searches projects for
`launcher`.

## Project Actions

Each project result supports:

- `Open Project`: open with the currently selected IDE scope or the backend's
  best match.
- `Open in Different IDE`: pick from installed IDEs.
- `Run Task`: show runnable tasks for the project.
- `Open in Terminal`: open the project directory in the configured terminal.
- `Open with Custom Command`: available when `Custom Open Command` is set.
- `Show in Finder`.
- `Copy Path`.
- `Pin Project` / `Unpin Project`: `Command` + `Shift` + `P`.
- `Forget Project`: `Command` + `Shift` + `Delete`.
- `Rerun Last Task`.

Pinned projects are shown with a star accessory and are sorted by the backend.
Worktrees and project-root entries are shown with list tags.

## Task Actions

The task list searches tasks through the backend and supports:

- `Run Task`.
- `Run in New Window`.
- `Run in New Tab`.
- `Run in Background`.
- `Copy Command`.
- `Run and Reset Project`.
- `Refresh Tasks` when the backend reports a refresh item.

Task execution uses the same backend command specs as the Alfred workflow, so
terminal behavior and task runner support are shared.

## Preferences

Raycast preferences map to backend environment variables at runtime:

- `Exclude Git Worktrees`: hide worktrees from the Recent source. Worktrees still
  appear in the Worktrees source.
- `Terminal App`: terminal for `Open in Terminal`.
- `Custom Open Command`: command template for `Open with Custom Command`, for
  example `code {path}`.
- `Task Terminal`: terminal used for task execution.
- `Task Window`: run tasks in a new terminal window by default.
- `Custom Task Terminal Command`: custom task launcher template. Supports
  `{cmd}`, `{cwd}`, and `{name}`.
- `Disable Task Runners`: comma-separated task runner names to skip, such as
  `gradle,npm,make`.
- `Sort Order`: project ordering before filtering.
- `Ignore Content`: comma-separated entry-name globs treated as non-content.
- `Ignore Projects`: comma-separated globs matched against project names and
  paths.
- `Config Roots`: colon-separated JetBrains and Google IDE config roots.
- `Application Folders`: colon-separated folders scanned for JetBrains `.app`
  bundles.
- `Project Roots`: colon-separated directories whose immediate subfolders are
  offered by `Projects`. When empty, the backend auto-detects
  conventional JetBrains folders.
- `Toolbox Script Dirs`: colon-separated directories containing JetBrains
  Toolbox launcher scripts.

## Local Development

Install dependencies from the extension directory:

```sh
cd raycast/jetbrains-project-launcher
npm install
```

Run the extension in Raycast:

```sh
npm run dev
```

For a local smoke test that explicitly rebuilds the backend and then starts
Raycast development mode:

```sh
npm run smoke
```

Build the extension package:

```sh
npm run build
```

Run Raycast lint checks:

```sh
npm run lint
```

`predev` and `prebuild` run `npm run prepare-backend` automatically.

## Bundled Backend

`scripts/prepare-backend.mjs` builds the Go backend from the repository root:

- Builds `./cmd/jb` for `darwin/arm64`.
- Builds `./cmd/jb` for `darwin/amd64`.
- Combines both binaries into `assets/bin/jb` with `lipo`.
- Applies an ad-hoc signature with `codesign --force -s -`.
- Copies runtime icons from the repository's `assets/icons` directory.

At runtime the extension performs a best-effort preparation step before the
first backend call:

- `chmod 755` on the bundled binary.
- Remove `com.apple.quarantine` from the bundled binary if present.

The extension uses Raycast's `environment.supportPath` for `JB_DATA_DIR` and
`JB_CACHE_DIR`, so Raycast state does not share Alfred's data and cache files.

## Troubleshooting

If the project list stays loading, first verify the bundled backend runs:

```sh
./assets/bin/jb version
./assets/bin/jb api projects --variant recent --product "" --query ""
```

If a freshly built binary hangs before producing output, macOS security
assessment may be blocked. Rebuilding with `npm run prepare-backend` re-signs
the binary, and the extension removes quarantine at runtime, but a wedged
`syspolicyd` can still block first launch until macOS recovers.

If expected project-root entries do not appear, set `Project Roots` in Raycast preferences.
Only immediate child directories of those roots are offered by the
`Projects` source.

If tasks are missing or slow, use `Disable Task Runners` to skip expensive
runners, or run the task refresh item shown by the backend.

## Store Notes

The extension does not require paid Raycast features for local development or
custom extension use.

Before submitting to the Raycast Store, confirm that the `author` field in
`package.json` matches a valid Raycast username. Store linting validates that
metadata separately from TypeScript and build correctness.
