/// <reference types="@raycast/api">

/* 🚧 🚧 🚧
 * This file is auto-generated from the extension's manifest.
 * Do not modify manually. Instead, update the `package.json` file.
 * 🚧 🚧 🚧 */

/* eslint-disable @typescript-eslint/ban-types */

type ExtensionPreferences = {
  /** Exclude Git Worktrees - Hide linked git worktrees from the Recent source. Worktrees still appear under the Worktrees source. */
  "excludeWorktrees": boolean,
  /** Terminal App - Terminal for the Open in Terminal action. */
  "terminal": "Terminal" | "iTerm" | "Warp" | "Ghostty" | "WezTerm" | "kitty" | "Alacritty" | "Hyper",
  /** Custom Open Command - Command for the custom open action, for example: code {path} */
  "customOpenCommand"?: string,
  /** Task Terminal - Terminal the task runner launches into. */
  "taskTerminal": "Terminal" | "iTerm" | "Ghostty",
  /** Task Window - Run tasks in a new terminal window by default. */
  "taskWindow": boolean,
  /** Custom Task Terminal Command - Template that launches tasks in any terminal. Supports {cmd}, {cwd}, and {name}. */
  "customTaskTerminalCommand"?: string,
  /** Disable Task Runners - Comma-separated task runners to skip, such as gradle,npm,make. */
  "disabledTaskRunners"?: string,
  /** Sort Order - Project result order before filtering. */
  "sortOrder": "recency" | "recency-asc" | "name" | "name-desc" | "path",
  /** Ignore Content - Comma-separated entry-name globs treated as non-content. */
  "ignoreContent": string,
  /** Ignore Projects - Comma-separated globs matched against project names and paths. */
  "ignoreProjects"?: string,
  /** Config Roots - Colon-separated dirs holding JetBrains and Google IDE config dirs. */
  "configRoots": string,
  /** Application Folders - Colon-separated folders scanned for JetBrains .app bundles. */
  "appRoots": string,
  /** Project Roots - Colon-separated dirs whose immediate subfolders are offered by the Projects source. Empty auto-detects conventional JetBrains folders. */
  "projectRoots"?: string,
  /** Toolbox Script Dirs - Colon-separated dirs holding JetBrains Toolbox launcher scripts. */
  "toolboxDirs": string
}

/** Preferences accessible in all the extension's commands */
declare type Preferences = ExtensionPreferences

declare namespace Preferences {
  /** Preferences accessible in the `search-projects` command */
  export type SearchProjects = ExtensionPreferences & {}
  /** Preferences accessible in the `rerun-last-task` command */
  export type RerunLastTask = ExtensionPreferences & {}
}

declare namespace Arguments {
  /** Arguments passed to the `search-projects` command */
  export type SearchProjects = {}
  /** Arguments passed to the `rerun-last-task` command */
  export type RerunLastTask = {}
}

