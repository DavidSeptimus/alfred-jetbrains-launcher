# Third-Party Notices

This file lists third-party material distributed with, or used by, this project.

## Source code

The `jb` binary is written in Go and uses **only the Go standard library** — it
has no third-party module dependencies. (`go.mod` requires one module,
`alfred-taskrunner`, which is this repository's own task-detection core,
developed in `./taskrunner` and resolved locally via a `replace` directive — not
an external dependency.) No third-party source code is bundled.

## Bundled logos and product icons

The workflow bundles a small set of JetBrains product logos (from the official
brand assets) as a fallback icon for IDEs you don't have installed. For IDEs you
*do* have installed — including Android Studio, Fleet, and Air — the result icon
is drawn by macOS from the application itself (via Alfred's `fileicon`) and is
not bundled or redistributed. The bundled images are **trademarks and property
of their respective owners**, included for identification (nominative) purposes
only, and are **not** covered by this project's MIT license.

**JetBrains s.r.o.** — IntelliJ IDEA, PyCharm, WebStorm, GoLand, CLion,
RubyMine, DataGrip, PhpStorm, Rider, RustRover, DataSpell, Aqua, and JetBrains
Toolbox logos. JetBrains and the above product names and logos are trademarks of
JetBrains s.r.o. Used in accordance with the JetBrains Website Terms of Use and
Brand Guidelines.

- Brand assets: <https://www.jetbrains.com/company/brand/>
- Website Terms of Use: <https://www.jetbrains.com/legal/docs/company/useterms/>

### Task-runner icons

The task runner's icons are rasterized from JetBrains' IntelliJ icon set:

- The run/execute arrow (the `runtask` keyword + fallback for runners without a
  dedicated icon) and the npm, Gradle, and Maven marks are extracted from the
  locally installed **IntelliJ IDEA**, distributed under the **Apache License
  2.0**, and reproduced here under that license.
- The Go, Cargo (Rust), Composer (PHP), Rake (Ruby), and .NET marks are pulled
  from the public **IntelliJ Icons catalog**
  (<https://intellij-icons.jetbrains.design>), JetBrains' resource for plugin
  authors, since those product IDEs aren't installed locally.

The underlying language/tool marks (npm, Gradle, Maven, Go, Rust, PHP, Ruby,
.NET) are trademarks of their respective projects, shown for identification
only. Regenerate with `scripts/gen-task-icons.sh`.

- Apache License 2.0: <https://www.apache.org/licenses/LICENSE-2.0>
- IntelliJ icon guidelines: <https://jetbrains.design/intellij/principles/icons/>

## No affiliation

This project is an independent, community-built Alfred workflow. It is **not
affiliated with, sponsored by, or endorsed by JetBrains s.r.o.** All trademarks
are the property of their respective owners.

If you are a rights holder and have any concern about the use of a mark here,
please open an issue and it will be addressed promptly.
