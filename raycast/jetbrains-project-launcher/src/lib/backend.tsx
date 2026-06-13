import {
  Action,
  ActionPanel,
  closeMainWindow,
  environment,
  getPreferenceValues,
  Icon,
  List,
  showToast,
  Toast,
} from "@raycast/api";
import { execFile, ExecFileOptions } from "node:child_process";
import { chmodSync, mkdirSync } from "node:fs";
import path from "node:path";
import { promisify } from "node:util";
import { useEffect, useState } from "react";

const execFileAsync = promisify(execFile);
let backendPrepared = false;
const taskSpecSeparator = "";

export type Preferences = {
  excludeWorktrees?: boolean;
  terminal?: string;
  customOpenCommand?: string;
  taskTerminal?: string;
  taskWindow?: boolean;
  customTaskTerminalCommand?: string;
  disabledTaskRunners?: string;
  sortOrder?: string;
  ignoreContent?: string;
  ignoreProjects?: string;
  configRoots?: string;
  appRoots?: string;
  projectRoots?: string;
  toolboxDirs?: string;
};

export type APIOutput<T> = {
  items: T[];
  rerunSeconds?: number;
};

export type APIIcon = {
  type?: string;
  path?: string;
};

export type Task = {
  title: string;
  subtitle: string;
  match: string;
  runner?: string;
  commandLine?: string;
  cwd?: string;
  runnable: boolean;
  kind: "task" | "refresh" | "info";
  spec?: string;
  windowSpec?: string;
  tabSpec?: string;
  backgroundSpec?: string;
  copySpec?: string;
  resetSpec?: string;
  icon?: APIIcon;
};

// Preferences only change on command relaunch, so read them once per process
// rather than on every backend call (each keystroke triggers one).
let cachedPrefs: Preferences | undefined;
export function getPrefs(): Preferences {
  if (!cachedPrefs) {
    cachedPrefs = getPreferenceValues<Preferences>();
  }
  return cachedPrefs;
}

// The support data/cache dirs only need creating once per process; doing it on
// every backend call adds two syscalls per keystroke.
let supportDirs: { dataDir: string; cacheDir: string } | undefined;
function ensureSupportDirs(): { dataDir: string; cacheDir: string } {
  if (!supportDirs) {
    const support = environment.supportPath;
    const dataDir = path.join(support, "data");
    const cacheDir = path.join(support, "cache");
    mkdirSync(dataDir, { recursive: true });
    mkdirSync(cacheDir, { recursive: true });
    supportDirs = { dataDir, cacheDir };
  }
  return supportDirs;
}

export async function api<T>(
  command: string,
  args: string[],
  signal?: AbortSignal,
): Promise<APIOutput<T>> {
  const stdout = await runBackend(["api", command, ...args], {
    silent: true,
    signal,
  });
  const parsed = JSON.parse(stdout) as APIOutput<T>;
  return { ...parsed, items: parsed.items ?? [] };
}

export async function runTaskSpec(
  spec: string | undefined,
  options: { fallbackSpec?: string } = {},
): Promise<void> {
  if (!spec) {
    return;
  }
  try {
    await runBackend(["runtask", "--spec", spec], {
      suppressFailureToast: Boolean(options.fallbackSpec),
    });
  } catch (error) {
    if (!options.fallbackSpec || options.fallbackSpec === spec) {
      await showFailure("Command failed", error);
      throw error;
    }
    await runBackend(["runtask", "--spec", options.fallbackSpec]);
  }
  await closeMainWindow();
}

export async function runBackend(
  args: string[],
  options: {
    silent?: boolean;
    signal?: AbortSignal;
    suppressFailureToast?: boolean;
  } = {},
): Promise<string> {
  const prefs = getPrefs();
  const { dataDir, cacheDir } = ensureSupportDirs();

  try {
    const backend = backendPath();
    await prepareBackendExecutable(backend);
    const { stdout } = await execFileWithTimeout(backend, args, {
      cwd: environment.assetsPath,
      encoding: "utf8",
      signal: options.signal,
      env: {
        ...process.env,
        JB_DATA_DIR: dataDir,
        JB_CACHE_DIR: cacheDir,
        JB_EXCLUDE_WORKTREES: prefs.excludeWorktrees === false ? "0" : "1",
        JB_TERMINAL: prefs.terminal ?? "Terminal",
        JB_OPEN_CMD: prefs.customOpenCommand ?? "",
        JB_TASK_TERMINAL: prefs.taskTerminal ?? prefs.terminal ?? "Terminal",
        JB_TASK_TERMINAL_CMD: prefs.customTaskTerminalCommand ?? "",
        JB_TASK_WINDOW: prefs.taskWindow ? "1" : "0",
        JB_TASK_DISABLE: prefs.disabledTaskRunners ?? "",
        JB_SORT: prefs.sortOrder ?? "recency",
        JB_IGNORE_CONTENT: prefs.ignoreContent ?? "build,dist,node_modules",
        JB_IGNORE_PROJECTS: prefs.ignoreProjects ?? "",
        JB_CONFIG_ROOTS: prefs.configRoots ?? "",
        JB_APP_ROOTS: prefs.appRoots ?? "",
        JB_PROJECT_ROOTS: prefs.projectRoots ?? "",
        JB_TOOLBOX_DIR: prefs.toolboxDirs ?? "",
      },
    });
    // The exec succeeded, so chmod/xattr prep is proven good — skip it next time.
    backendPrepared = true;
    if (!options.silent) {
      await showToast({ style: Toast.Style.Success, title: "Done" });
    }
    return stdout;
  } catch (error) {
    if (!options.silent && !options.suppressFailureToast) {
      await showFailure("Command failed", error);
    }
    throw error;
  }
}

function replaceTaskSpecKind(spec: string, kind: string): string {
  const sep = spec.indexOf(taskSpecSeparator);
  if (sep === -1) {
    return spec;
  }
  return kind + spec.slice(sep);
}

async function execFileWithTimeout(
  file: string,
  args: string[],
  options: ExecFileOptions & { signal?: AbortSignal },
): Promise<{ stdout: string; stderr: string }> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 20_000);
  const relay = () => controller.abort();
  options.signal?.addEventListener("abort", relay, { once: true });
  try {
    const result = await execFileAsync(file, args, {
      ...options,
      signal: controller.signal,
    });
    return {
      stdout: String(result.stdout),
      stderr: String(result.stderr),
    };
  } finally {
    clearTimeout(timeout);
    options.signal?.removeEventListener("abort", relay);
  }
}

function backendPath(): string {
  return path.join(environment.assetsPath, "bin", "jb");
}

async function prepareBackendExecutable(backend: string): Promise<void> {
  if (backendPrepared) {
    return;
  }
  try {
    chmodSync(backend, 0o755);
  } catch {
    // The backend exec below will report the actionable failure if chmod was not possible.
  }
  try {
    await execFileAsync("/usr/bin/xattr", [
      "-d",
      "com.apple.quarantine",
      backend,
    ]);
  } catch {
    // Missing xattr is the normal local/dev case and should not block execution.
  }
  // Deliberately not marking backendPrepared here: chmod/xattr failures above are
  // swallowed, so if the binary is still non-executable we want to retry prep on
  // the next call. runBackend sets backendPrepared only after an exec succeeds.
}

export function imageFor(icon: APIIcon | undefined): List.Item.Props["icon"] {
  if (!icon?.path) {
    return Icon.Code;
  }
  if (icon.type === "fileicon") {
    return { fileIcon: icon.path } as List.Item.Props["icon"];
  }
  return icon.path;
}

export async function showFailure(title: string, error: unknown) {
  const message = errorMessage(error);
  await showToast({ style: Toast.Style.Failure, title, message });
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    const details = error as Error & { stderr?: string; stdout?: string };
    return (
      details.stderr?.trim() ||
      details.stdout?.trim() ||
      details.message ||
      String(error)
    );
  }
  return String(error);
}

export function isAbortError(error: unknown): boolean {
  return (
    error instanceof Error &&
    (error.name === "AbortError" ||
      error.message.toLowerCase().includes("aborted"))
  );
}

export function useDebouncedValue<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

export function TaskActions(props: { task: Task; onRefresh: () => void }) {
  const { task, onRefresh } = props;
  if (task.kind === "info") {
    return <ActionPanel />;
  }
  if (task.kind === "refresh") {
    return (
      <ActionPanel>
        <Action
          title="Refresh Tasks"
          icon={Icon.RotateClockwise}
          onAction={async () => {
            if (!task.spec) {
              return;
            }
            // Spawn the background re-enumeration but keep the window open: bumping
            // refreshToken re-fetches, which shows the "Refreshing…" row and starts
            // the rerunSeconds poll that swaps in the fresh list. (runTaskSpec is not
            // used here because it closes the main window, which would kill the poll.)
            try {
              await runBackend(["runtask", "--spec", task.spec], {
                silent: true,
              });
            } catch (error) {
              await showFailure("Could not refresh tasks", error);
              return;
            }
            onRefresh();
          }}
        />
      </ActionPanel>
    );
  }
  return (
    <ActionPanel>
      {task.runnable && task.spec ? (
        <Action
          title="Run Task"
          icon={Icon.Play}
          onAction={async () =>
            runTaskSpec(task.spec, { fallbackSpec: task.windowSpec })
          }
        />
      ) : null}
      {task.runnable && task.windowSpec ? (
        <Action
          title="Run in New Window"
          icon={Icon.AppWindow}
          onAction={async () => runTaskSpec(task.windowSpec)}
        />
      ) : null}
      {task.runnable && task.tabSpec ? (
        <Action
          title="Run in New Tab"
          icon={Icon.Terminal}
          onAction={async () => runTaskSpec(task.tabSpec)}
        />
      ) : null}
      {task.runnable && task.backgroundSpec ? (
        <Action
          title="Run in Background"
          icon={Icon.Hourglass}
          onAction={async () => runTaskSpec(task.backgroundSpec)}
        />
      ) : null}
      {task.copySpec ? (
        <Action
          title="Copy Command"
          icon={Icon.Clipboard}
          onAction={async () => runTaskSpec(task.copySpec)}
        />
      ) : null}
      {task.runnable && task.resetSpec ? (
        <Action
          title="Run and Reset Project"
          icon={Icon.ArrowClockwise}
          onAction={async () =>
            runTaskSpec(task.resetSpec, {
              fallbackSpec: task.resetSpec
                ? replaceTaskSpecKind(task.resetSpec, "windowreset")
                : undefined,
            })
          }
        />
      ) : null}
    </ActionPanel>
  );
}

// RerunTask lists the single most-recently-run task (a global record, not tied to
// any project) so it can be re-run without first finding a project. It backs the
// top-level "Rerun Last Task" command.
export function RerunTask() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const controller = new AbortController();
    let cancelled = false;
    api<Task>("rerun", [], controller.signal)
      .then((out) => {
        if (!cancelled) {
          setTasks(out.items);
        }
      })
      .catch((error) => {
        if (!isAbortError(error)) {
          showFailure("Could not load last task", error);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setIsLoading(false);
        }
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, []);

  return (
    <List isLoading={isLoading}>
      {tasks.length === 0 && !isLoading ? (
        <List.EmptyView icon={Icon.RotateClockwise} title="No task run yet" />
      ) : null}
      {tasks.map((task) => (
        <List.Item
          key={task.spec}
          title={task.title}
          subtitle={task.subtitle}
          icon={imageFor(task.icon)}
          actions={<TaskActions task={task} onRefresh={() => undefined} />}
        />
      ))}
    </List>
  );
}
