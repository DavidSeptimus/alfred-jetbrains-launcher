# Contributing

## Commit messages — Conventional Commits

This repo uses [Conventional Commits](https://www.conventionalcommits.org). The
format is enforced both locally and in CI, and it drives the changelog and the
release version bump.

```
<type>[optional scope][!]: <subject>
```

| Type       | Use it for                              | Changelog   |
|------------|-----------------------------------------|-------------|
| `feat`     | a user-facing feature                   | **Added**   |
| `fix`      | a bug fix                               | **Fixed**   |
| `perf`     | a performance improvement               | **Changed** |
| `refactor` | code change that isn't a feature or fix | —           |
| `docs`     | documentation only                      | —           |
| `test`     | tests only                              | —           |
| `build`    | build system, Makefile, packaging       | —           |
| `ci`       | CI/workflows                            | —           |
| `chore`    | housekeeping (deps, release commits)    | —           |
| `style`    | formatting, whitespace                  | —           |
| `revert`   | reverts a previous commit               | —           |

A `!` (or a `BREAKING CHANGE:` footer) marks a breaking change — it forces a
**major** version bump and is flagged in the changelog. Only `feat`, `fix`, and
`perf` (and any breaking change) appear in the changelog; the rest are omitted to
keep it user-focused.

Examples:

```
feat: add a configurable custom open command
fix(search): honour a renamed keyword after pin/forget
refactor!: drop the legacy jbup keyword
```

### Enable the local hook

```sh
make hooks      # sets core.hooksPath=.githooks
```

After this, `git commit` rejects any message that isn't a Conventional Commit
(`scripts/commit-msg-lint.sh` is the single source of truth; the `commit-lint` CI
workflow runs the identical check on every push and PR).

## Changelog

[`CHANGELOG.md`](CHANGELOG.md) follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and is **generated** from the commit history by [git-cliff](https://git-cliff.org)
(config in [`cliff.toml`](cliff.toml)) — don't edit it by hand. Preview the
unreleased section locally:

```sh
brew install git-cliff   # once
make changelog           # regenerates CHANGELOG.md
```

## Releases

Releases are cut from **GitHub → Actions → "release" → Run workflow**. Pick a bump:

- **`auto`** (default) — the next version is derived from the commits since the
  last tag: `fix` → patch, `feat` → minor, a breaking change → major.
- **`patch` / `minor` / `major`** — force that level (e.g. to cut a `minor` for a
  milestone, or jump to `1.0.0`).

The workflow regenerates `CHANGELOG.md`, builds the universal `.alfredworkflow`,
commits the version + changelog as `chore(release): vX.Y.Z`, tags it, and
publishes a GitHub Release whose notes are that version's changelog section.
