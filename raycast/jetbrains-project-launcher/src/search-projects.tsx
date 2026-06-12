import { Action, ActionPanel, closeMainWindow, Icon, List } from "@raycast/api";
import { Dispatch, SetStateAction, useEffect, useState } from "react";
import {
  api,
  APIIcon,
  getPrefs,
  imageFor,
  isAbortError,
  runBackend,
  showFailure,
  Task,
  TaskActions,
  useDebouncedValue,
} from "./lib/backend";

type Project = {
  title: string;
  subtitle: string;
  path: string;
  match: string;
  family?: string;
  ide?: string;
  valid: boolean;
  pinned: boolean;
  worktree: boolean;
  projectRoot: boolean;
  branch?: string;
  icon?: APIIcon;
  taskPickSpec: string;
};

type IDEChoice = {
  title: string;
  subtitle: string;
  spec: string;
  family: string;
  version: string;
  icon?: APIIcon;
};

type Scope = {
  product: string;
  variant: "recent" | "roots" | "worktrees";
};

const products = [
  ["", "All IDEs"],
  ["idea", "IntelliJ IDEA"],
  ["pycharm", "PyCharm"],
  ["webstorm", "WebStorm"],
  ["goland", "GoLand"],
  ["clion", "CLion"],
  ["rubymine", "RubyMine"],
  ["datagrip", "DataGrip"],
  ["phpstorm", "PhpStorm"],
  ["rider", "Rider"],
  ["rustrover", "RustRover"],
  ["studio", "Android Studio"],
  ["dataspell", "DataSpell"],
  ["aqua", "Aqua"],
  ["writerside", "Writerside"],
  ["fleet", "Fleet"],
  ["air", "Air"],
] as const;

const variants = [
  ["recent", "Recent"],
  ["roots", "Projects"],
  ["worktrees", "Worktrees"],
] as const;

function scopeTitle(variantTitle: string, productTitle: string): string {
  return `${variantTitle} - ${productTitle}`;
}

function scopeValue(scope: Scope): string {
  return `${scope.variant}:${scope.product || "all"}`;
}

function parseScope(value: string): Scope {
  const [variant, product = "all"] = value.split(":", 2);
  return {
    variant:
      variant === "roots" || variant === "worktrees" ? variant : "recent",
    product: product === "all" ? "" : product,
  };
}

function searchFromQuery(
  scope: Scope,
  query: string,
): { scope: Scope; query: string } {
  if (query.startsWith("~")) {
    return {
      scope: { ...scope, variant: "worktrees" },
      query: query.slice(1).trimStart(),
    };
  }
  if (query.startsWith("+")) {
    return {
      scope: { ...scope, variant: "roots" },
      query: query.slice(1).trimStart(),
    };
  }
  return { scope, query };
}

function ScopeDropdown(props: {
  scope: Scope;
  setScope: Dispatch<SetStateAction<Scope>>;
}) {
  return (
    <List.Dropdown
      tooltip="Scope"
      value={scopeValue(props.scope)}
      onChange={(value) => props.setScope(parseScope(value))}
    >
      {variants.map(([variant, title]) => (
        <List.Dropdown.Section title={title} key={variant}>
          {products.map(([product, productTitle]) => (
            <List.Dropdown.Item
              key={`${variant}:${product || "all"}`}
              value={`${variant}:${product || "all"}`}
              title={scopeTitle(title, productTitle)}
            />
          ))}
        </List.Dropdown.Section>
      ))}
    </List.Dropdown>
  );
}

export default function SearchProjects() {
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query, 180);
  const [scope, setScope] = useState<Scope>({ product: "", variant: "recent" });
  const [projects, setProjects] = useState<Project[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [refreshToken, setRefreshToken] = useState(0);
  const effectiveSearch = searchFromQuery(scope, debouncedQuery);

  // Reflect a leading ~/+ prefix back into the scope dropdown so its variant
  // matches the results the prefix forces. The prefix wins over a manual dropdown
  // change while present (keying on scope re-forces it if the user switches), and
  // the guarded setter keeps the same reference when already in sync, so this
  // can't loop. Product (the other scope dimension) is preserved.
  useEffect(() => {
    let variant: Scope["variant"] | null = null;
    if (debouncedQuery.startsWith("~")) {
      variant = "worktrees";
    } else if (debouncedQuery.startsWith("+")) {
      variant = "roots";
    }
    if (variant) {
      const v = variant;
      setScope((s) => (s.variant === v ? s : { ...s, variant: v }));
    }
  }, [debouncedQuery, scope]);

  useEffect(() => {
    const controller = new AbortController();
    let cancelled = false;
    setIsLoading(true);
    api<Project>(
      "projects",
      [
        "--product",
        effectiveSearch.scope.product,
        "--variant",
        effectiveSearch.scope.variant,
        "--query",
        effectiveSearch.query,
      ],
      controller.signal,
    )
      .then((out) => {
        if (!cancelled) {
          setProjects(out.items);
        }
      })
      .catch((error) => {
        if (isAbortError(error)) {
          return;
        }
        if (!cancelled) {
          setProjects([]);
          showFailure("Could not load projects", error);
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
  }, [
    effectiveSearch.query,
    effectiveSearch.scope.product,
    effectiveSearch.scope.variant,
    refreshToken,
  ]);

  return (
    <List
      isLoading={isLoading}
      filtering={false}
      onSearchTextChange={(text) => setQuery(text ?? "")}
      searchBarPlaceholder="Search projects by name, path, IDE, or branch"
      searchBarAccessory={<ScopeDropdown scope={scope} setScope={setScope} />}
    >
      {projects.length === 0 && !isLoading ? (
        <List.EmptyView icon={Icon.MagnifyingGlass} title="No projects found" />
      ) : null}
      {projects.map((project) => (
        <List.Item
          key={project.path}
          title={project.title}
          subtitle={project.subtitle}
          icon={imageFor(project.icon)}
          keywords={project.match.split(/\s+/).filter(Boolean)}
          accessories={projectAccessories(project)}
          actions={
            <ProjectActions
              project={project}
              product={effectiveSearch.scope.product}
              onRefresh={() => setRefreshToken((n) => n + 1)}
            />
          }
        />
      ))}
    </List>
  );
}

function ProjectActions(props: {
  project: Project;
  product: string;
  onRefresh: () => void;
}) {
  const { project, product, onRefresh } = props;
  const prefs = getPrefs();
  const hasCustomOpen = Boolean(prefs.customOpenCommand?.trim());
  return (
    <ActionPanel>
      {/* Only offer the direct Open when an IDE actually resolves; otherwise it can
          only fail, so "Open in Different IDE" (the picker) becomes the primary action. */}
      {project.valid ? (
        <Action
          title="Open Project"
          icon={Icon.ArrowRight}
          onAction={async () => {
            await runBackend([
              "open",
              ...(product ? ["--product", product] : []),
              "--path",
              project.path,
            ]);
            await closeMainWindow();
          }}
        />
      ) : null}
      <Action.Push
        title="Open in Different IDE"
        icon={Icon.AppWindow}
        target={<IDEPicker path={project.path} />}
      />
      <Action.Push
        title="Run Task"
        icon={Icon.Terminal}
        target={<TaskList project={project} />}
      />
      <Action
        title="Open in Terminal"
        icon={Icon.Terminal}
        onAction={async () =>
          runBackend(["action", "--do", "terminal", "--path", project.path])
        }
      />
      {hasCustomOpen ? (
        <Action
          title="Open with Custom Command"
          icon={Icon.Code}
          onAction={async () =>
            runBackend(["action", "--do", "command", "--path", project.path])
          }
        />
      ) : null}
      <Action.ShowInFinder path={project.path} />
      <Action.CopyToClipboard title="Copy Path" content={project.path} />
      <Action
        title={project.pinned ? "Unpin Project" : "Pin Project"}
        icon={project.pinned ? Icon.StarDisabled : Icon.Star}
        shortcut={{ modifiers: ["cmd", "shift"], key: "p" }}
        onAction={async () => {
          await runBackend(["pin", "--path", project.path]);
          onRefresh();
        }}
      />
      <Action
        title="Forget Project"
        icon={Icon.Trash}
        style={Action.Style.Destructive}
        shortcut={{ modifiers: ["cmd", "shift"], key: "delete" }}
        onAction={async () => {
          await runBackend(["forget", "--path", project.path]);
          onRefresh();
        }}
      />
    </ActionPanel>
  );
}

function IDEPicker(props: { path: string }) {
  const [items, setItems] = useState<IDEChoice[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const controller = new AbortController();
    let cancelled = false;
    api<IDEChoice>("ides", ["--path", props.path], controller.signal)
      .then((out) => {
        if (!cancelled) {
          setItems(out.items);
        }
      })
      .catch((error) => {
        if (isAbortError(error)) {
          return;
        }
        if (!cancelled) {
          showFailure("Could not load IDEs", error);
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
  }, [props.path]);

  return (
    <List
      isLoading={isLoading}
      searchBarPlaceholder="Pick an installed JetBrains IDE"
    >
      {items.map((item) => (
        <List.Item
          key={item.spec}
          title={item.title}
          subtitle={item.subtitle}
          icon={imageFor(item.icon)}
          actions={
            <ActionPanel>
              <Action
                title="Open in IDE"
                icon={Icon.ArrowRight}
                onAction={async () => {
                  await runBackend(["open", "--spec", item.spec]);
                  await closeMainWindow();
                }}
              />
            </ActionPanel>
          }
        />
      ))}
    </List>
  );
}

function TaskList(props: { project: Project }) {
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query, 180);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [refreshToken, setRefreshToken] = useState(0);
  const [rerunSeconds, setRerunSeconds] = useState<number | undefined>();

  useEffect(() => {
    const controller = new AbortController();
    let cancelled = false;
    setIsLoading(true);
    api<Task>(
      "tasks",
      ["--path", props.project.path, "--query", debouncedQuery],
      controller.signal,
    )
      .then((out) => {
        if (!cancelled) {
          setTasks(out.items);
          setRerunSeconds(out.rerunSeconds);
        }
      })
      .catch((error) => {
        if (!isAbortError(error)) {
          showFailure("Could not load tasks", error);
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
  }, [props.project.path, debouncedQuery, refreshToken]);

  useEffect(() => {
    if (!rerunSeconds) {
      return;
    }
    const timer = setTimeout(
      () => setRefreshToken((n) => n + 1),
      rerunSeconds * 1000,
    );
    return () => clearTimeout(timer);
  }, [rerunSeconds, refreshToken]);

  return (
    <List
      isLoading={isLoading}
      filtering={false}
      onSearchTextChange={(text) => setQuery(text ?? "")}
      searchBarPlaceholder={`Search tasks in ${props.project.title}`}
    >
      {tasks.length === 0 && !isLoading ? (
        <List.EmptyView icon={Icon.Terminal} title="No tasks found" />
      ) : null}
      {tasks.map((task, index) => (
        <List.Item
          key={`${task.kind}:${task.spec ?? task.title}:${index}`}
          title={task.title}
          subtitle={task.subtitle}
          icon={imageFor(task.icon)}
          keywords={task.match?.split(/\s+/).filter(Boolean)}
          actions={
            <TaskActions
              task={task}
              onRefresh={() => setRefreshToken((n) => n + 1)}
            />
          }
        />
      ))}
    </List>
  );
}

function projectAccessories(project: Project): List.Item.Accessory[] {
  const accessories: List.Item.Accessory[] = [];
  if (project.pinned) {
    accessories.push({ icon: Icon.Star });
  }
  // The ⑂ title prefix already marks worktrees, so the tag space shows the git
  // branch instead (for both projects and worktrees that are on one).
  if (project.branch) {
    accessories.push({ tag: `⎇ ${project.branch}`, tooltip: "Git branch" });
  }
  if (project.ide) {
    accessories.push({ text: project.ide });
  }
  return accessories;
}
