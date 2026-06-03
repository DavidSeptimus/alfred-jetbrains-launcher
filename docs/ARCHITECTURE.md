# Architecture & key flows

Diagrams of the non-obvious control flows behind the workflow. Each renders on
GitHub directly from the ```mermaid blocks below.

- [Project discovery → merge → display](#project-discovery--merge--display)
- [IDE resolution](#ide-resolution)
- [Update check (cached banner vs live `jbup`)](#update-check-cached-banner-vs-live-jbup)
- [First-run quarantine self-heal](#first-run-quarantine-self-heal)

---

## Project discovery → merge → display

Every `jb search` builds (or reuses) the merged project list. The cache key is an
mtime fingerprint, so a newly-created IDE version dir invalidates it — that's what
prevents the "only the newest version is read" bug this workflow exists to fix.

```mermaid
flowchart LR
    classDef cache fill:#AB47BC,stroke:#8E24AA,color:#fff

    A["Read recent-project lists from every
       IDE and every version dir, plus Android
       Studio and Fleet/Air workspaces"]
    A --> B{"Sources changed since
             last run? (mtime fingerprint)"}
    B -->|no| C[("Reuse cached list")]:::cache
    B -->|yes| D["Merge into one list:
                  dedupe by path, keep
                  the most-recently-used"]
    D --> E[("Save to cache")]:::cache
    C --> F["Filter out what you should not see:
             missing folders, stubs, worktrees,
             forgotten, ignore-listed"]
    E --> F
    F --> G["Pin favourites on top,
             then apply the sort order"]
    G --> H["Show results in Alfred"]
```

Code: `internal/discover`, `internal/recent` (merge/dedupe), `internal/cache`
(mtime fingerprint), `cmd/jb` `loadProjects` / `emitSearch`.

---

## IDE resolution

Which IDE opens a project, then which *instance*. The chain falls back from the
exact recorded version down to "any IDE that fits", and finally reuses an
already-running instance of the resolved product rather than spawning another.

```mermaid
flowchart LR
    classDef ok fill:#26A69A,stroke:#00897B,color:#fff

    A["project: recorded productionCode + version"] --> B{"recorded version installed?"}
    B -->|yes| R["resolved IDE"]:::ok
    B -->|no| C{"newest installed of the same product?"}
    C -->|yes| R
    C -->|no| D{"project type first-class in IDEA? (Java/Kotlin/web/Py/Go/PHP/DB/Ruby)"}
    D -->|yes| E["IntelliJ IDEA Ultimate (latest)"]
    E --> R
    D -->|no| G{"any newest installed IDE that fits?"}
    G -->|yes| R
    G -->|no| X["no IDE, reveal / copy still work"]
    R --> H{"a different version of that product already running?"}
    H -->|yes| I["reuse the running instance"]
    H -->|no| J["launch the resolved target"]
```

Code: `internal/ide` (`Resolve`, `NewestByFamily`, `PreferRunning`), `cmd/jb`
`cmdOpen`.

---

## Update check (cached banner vs live `jbup`)

Two surfaces, deliberately different. The passive "update available" banner on
`jb` is driven entirely by a **cached** check (no network on the hot path), with a
background refresh at most once a day; the banner is **selectable** — pressing ↩
runs the update in place. Typing **`jbup`** instead hits GitHub **live**, so it's
always current. Self-update downloads via curl — not a browser — so the upgrade is
never quarantined.

```mermaid
flowchart LR
    classDef cache fill:#AB47BC,stroke:#8E24AA,color:#fff
    classDef net fill:#5C6BC0,stroke:#3949AB,color:#fff

    A["jb search (runs on every keystroke)"] --> B{"release channel and unified jb keyword?"}
    B -->|no| Z["no update UI"]
    B -->|yes| C[("update-cache.json")]:::cache
    C --> D{"last check over 24h ago?"}
    D -->|fresh| G{"cached latestTag newer than current?"}
    D -->|stale| E["TouchChecked: stamp checkedAt = now"]:::cache
    E --> F["spawn detached: jb update --refresh-cache"]
    F --> G
    F -. background .-> J["RefreshCache: GitHub latest API"]:::net
    J --> K["write latestTag + checkedAt (even on failure)"]:::cache
    K -. updates .-> C
    G -->|yes| H["prepend Update available banner (↩ selectable)"]
    G -->|no| I["no banner"]
    H -. "↩ routes through jb open sentinel" .-> Q

    JB["jbup keyword"] --> L["jb update --check"]
    L --> M["GitHub latest API — LIVE, bypasses cache"]:::net
    M --> N{"newer?"}
    N -->|yes| O["Install update row, then jb update --apply"]
    N -->|no| P["Up to date"]
    O --> Q["Download via curl (no quarantine)"]
    Q --> R["open .alfredworkflow, Alfred imports in place"]
```

The `TouchChecked` stamp is the debounce: Alfred re-runs the Script Filter on
every keystroke, so stamping `checkedAt = now` *before* spawning means only one
background refresh fires per 24h window instead of one per keystroke. A failed
refresh still records `checkedAt`, so a transient outage doesn't cause constant
retries (it waits the full window). The banner has no downstream wiring of its own
— Alfred routes one connection per Script Filter — so its ↩ reuses the `jb open`
action via a sentinel `arg` that `cmdOpen` recognises and turns into an update.
Code: `internal/update`, `cmd/jb` `updateBanner` / `spawnBackgroundRefresh` /
`cmdUpdate` / `cmdOpen`.

---

## First-run quarantine self-heal

A browser-downloaded release is quarantined, and macOS Gatekeeper blocks the
ad-hoc-signed binary on first launch. The fix lives in the Script Filter shell,
not the binary (see the note below).

```mermaid
flowchart LR
    classDef gk fill:#EF5350,stroke:#E53935,color:#fff
    classDef ok fill:#26A69A,stroke:#00897B,color:#fff

    A["Download .alfredworkflow in a browser"] --> B["macOS sets com.apple.quarantine on the file"]:::gk
    B --> C["Double-click, Alfred imports into user.workflow.UUID"]
    C --> D["Quarantine propagates to all extracted files"]:::gk
    D --> E["User triggers a keyword (e.g. jb)"]
    E --> F["Alfred runs the Script Filter via /bin/bash, cwd = bundle"]
    F --> G{".dequarantined marker present?"}
    G -->|yes| K["skip sweep"]
    G -->|no| H["/usr/bin/xattr -dr com.apple.quarantine $PWD"]:::ok
    H --> I["touch .dequarantined"]
    I --> K
    K --> L["exec ./jb, binary no longer quarantined so Gatekeeper allows it"]:::ok
    L --> M["results shown"]
```

**Why the shell, not the binary:** a direct exec of the quarantined binary is
killed by Gatekeeper *before* `main()` runs, so the binary can never clear its own
flag — a chicken-and-egg. Alfred's inline Script Filter runs under `/bin/bash` (a
system binary, not gated) using `/usr/bin/xattr` (also system), so it strips the
flag first, then launches the now-clean binary. The sweep is scoped to `"$PWD"`
(our own bundle, never a sibling workflow) and guarded by a `.dequarantined`
marker so it runs once per install; Alfred wipes the marker when it re-imports an
upgrade, so a re-downloaded (and thus re-quarantined) build is cleaned again.
Code: `dequarantinePrefix` in `cmd/genplist/main.go`.
