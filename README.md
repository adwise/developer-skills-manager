# Developer Skills Manager

Repository: `adwise/developer-skills-manager`. CLI command: `skills-manager`.

A Git-native CLI for installing agent skills as plain files. Bring your own public or private Git repository; there is no central registry, account service, or bundled company catalog.

## Install and configure

### Homebrew (Adwise tap)

Install the stable release from the [Adwise tap](https://github.com/adwise/homebrew-brew):

```bash
brew install adwise/brew/skills-manager
skills-manager init --source git@github.com:example/team-skills.git
skills-manager
```

Upgrade with `brew update && brew upgrade --formula adwise/brew/skills-manager`. The stable formula pins a published release and its SHA-256 checksum. Until bottles are published for your platform, installation builds from source with Go. While the tap or bottles remain private, follow the tap's authentication instructions; the public Go installation below does not require tap access.

For an installation made with `--HEAD`, explicitly fetch the latest commit when upgrading; `brew update` only refreshes Homebrew and its taps:

```bash
brew upgrade --formula --fetch-HEAD adwise/brew/skills-manager
```

Always use the full formula name: the unqualified name `skills-manager` can also refer to an unrelated Homebrew cask.

To switch from `--HEAD` to stable, uninstall and reinstall the CLI. This does not remove your skills or configuration:

```bash
brew uninstall --formula adwise/brew/skills-manager
brew install adwise/brew/skills-manager
```

### Automatic releases

Every push to `main` that passes tests, vet, and the installation smoke test builds and publishes a GitHub release tagged `v0.1.<workflow run number>`. Releases include Linux binaries for `amd64` (x86-64) and `arm64`, and macOS (`darwin`) binaries for `arm64` (Apple Silicon), packaged with the README and license, plus `checksums.txt` with SHA-256 checksums. Pull requests do not publish releases. Re-running a successful workflow reuses its release and replaces its assets rather than creating another version.

The Adwise tap's `Update skills-manager release` workflow checks for new releases every 15 minutes (GitHub scheduling can be delayed), updates the stable formula and checksum, and commits to the tap's `main` branch. It also supports manual dispatch. Both repositories' workflows must be merged into `main` and Actions must be allowed to write repository contents; tap branch protection must permit its updater to push. No cross-repository token is needed because the CLI releases are public. Bottles are not automatically published by this updater; Homebrew can build from source.

A stable install gets the newest release already synchronized into the tap, not necessarily the latest `main` commit. Use `--HEAD` if you explicitly want unreleased code. Existing HEAD installations are not switched to stable automatically.

### Go install (public, no Homebrew required)

Requires Go 1.23+:

```bash
go install github.com/adwise/developer-skills-manager/cmd/skills-manager@latest
export PATH="$(go env GOPATH)/bin:$PATH"
skills-manager --help
skills-manager init --source git@github.com:example/team-skills.git
skills-manager
```

Go puts the executable in `$(go env GOPATH)/bin` by default. Add the PATH line to your shell profile (`~/.zshrc` or `~/.bashrc`) to keep it available in new terminals. If you set `GOBIN`, add that directory to PATH instead. Repeat `go install ...@latest` to upgrade. Git and access to your source repository are required to fetch skills; Go installation itself does not require access to the private Adwise tap or catalog.

### Local build

From a checkout of this repository:

```bash
go build -o build/skills-manager ./cmd/skills-manager
./build/skills-manager init --source /path/to/team-skills
./build/skills-manager
```

Private repositories use your existing Git SSH or credential-helper configuration. Do not embed access tokens in repository URLs; the source URL is saved in configuration and may appear in output.

`init` checks that the source has a `skills/` directory and atomically saves the source and namespace in `~/.config/skills-manager/config.json`. Local paths are saved as absolute paths. Full package and manifest validation happens when listing/installing skills or running `validate`.

Global installations live in `~/.agents/skills/skills-manager/`. To use another namespace, including an existing installation directory:

```bash
skills-manager init --source git@github.com:example/team-skills.git --namespace team
```

This uses `~/.agents/skills/team/`; it does not move or remove existing skills. A namespace is a single directory name, not a free installation path. Source names are independent of namespaces, and namespaces must be unique.

### Multiple sources

```bash
skills-manager source add adwise --source git@github.com:example/team-skills.git --namespace skills-manager --repo /local/team-skills
skills-manager source add personal --source /local/personal-skills --repo /local/personal-skills
skills-manager source list
skills-manager install adwise/code-review personal/code-review
skills-manager status                         # All configured sources
skills-manager update --from personal        # One source
skills-manager config set repo /local/team-skills --from adwise
skills-manager contribute --all              # Route to each source's repo
skills-manager source remove personal        # Configuration ONLY
```

`source add` defaults its namespace to NAME. It refuses existing source names or reused namespaces. Installations are directly in `~/.agents/skills/<namespace>/<skill-ID>`: no extra source directory. Removal never deletes installations, baselines, or caches. Re-adding the same source and namespace reconnects those files; do not reuse an old namespace for unrelated data.

The saved configuration is:

```json
{
  "sources": {
    "adwise": {
      "source": "git@github.com:example/team-skills.git",
      "namespace": "skills-manager",
      "repo": "/local/team-skills"
    },
    "personal": {
      "source": "/local/personal-skills",
      "namespace": "personal",
      "repo": "/local/personal-skills"
    }
  }
}
```

Use qualified IDs `source/skill`, `source/nested/skill`, and `source/@set`. An unqualified ID or `@set` works only if unique among the selected sources. IDs are matched in full, not split naively at `/`. If a qualified spelling also matches another source's nested ID, use `--from NAME` with the exact source-local ID. Sets and dependencies stay within their source; manifests never use cross-source qualifiers. The TUI opens on **All sources**, with combined sets and skills sections sorted by qualified ID. Each configured source also has a tab, including empty sources; source tabs show local names without changing action routing. `--from` still limits which sources are loaded.

**Migration:** on first config read, a legacy `{source, namespace, repo}` config is atomically rewritten under `sources.default`. The deterministic name is always `default`: URLs and directory names are not reliable source labels. The existing namespace is preserved (or `skills-manager` when absent). Installed files, baseline snapshots, source identities, and original catalog paths are neither moved nor rewritten. Missing baselines remain unknown. Legacy caches are left untouched, not adopted or reset; configured sources use separate caches under `<cache>-sources/<source-name>/<source-hash>`, including isolated external caches. The first remote access may fetch a fresh checkout. Renaming a source in config does not change its namespace-based installations or baseline paths.

**Compatibility:** `init` creates `default` when unconfigured and updates the sole source otherwise; with multiple sources it requires `--from NAME`. It preserves `repo` and cannot change an existing source's namespace. `config set repo` likewise requires `--from` when multiple sources exist (configuring a repo before first init remains supported). `--source` / `SKILLS_MANAGER_SOURCE` still mean a URL/path, never a source name; they override a single selected source for that invocation. With multiple configured sources, source/namespace/repo overrides require explicit `--from`. A configured namespace cannot be redirected by a flag or environment variable; use `source add` instead. Without configured sources, the original ad-hoc `--source`/`--namespace` workflow and legacy cache location still work. `--repo` is invocation-only; use `config set repo` to persist it.

Options work before or after commands. Precedence is **flags > environment > saved configuration > defaults**:

| Flag | Environment | Default |
| --- | --- | --- |
| `--source` | `SKILLS_MANAGER_SOURCE` | Saved source; no built-in repository |
| `--namespace` | `SKILLS_MANAGER_NAMESPACE` | Saved namespace or `skills-manager` |
| `--home` | `SKILLS_MANAGER_HOME` | User home (skills and configuration) |
| `--cache` | `SKILLS_MANAGER_CACHE` | OS user cache + `skills-manager/repository` (base for per-source caches) |
| `--from` | — | All configured sources |

`init` persists the source and namespace and preserves the saved contribution repo (unless explicitly supplied with `--repo`). Home and cache overrides apply only to that invocation.

## Commands

```text
skills-manager                            Interactive installer (default)
skills-manager --help                     Show help
skills-manager init --source <url-or-path>  Configure a source repository
skills-manager source add NAME --source URL/path [--namespace NS] [--repo PATH]
skills-manager source list                List configured sources
skills-manager source remove NAME         Remove configuration only
skills-manager available [query]          List or search skills
skills-manager sets                       List skill sets
skills-manager list                       List global and repository-owned skills
skills-manager install                    Interactive multi-select installer
skills-manager install <name|@set>...      Install skills or sets
skills-manager tui                        Interactive installer
skills-manager uninstall <name>           Remove a global installation
skills-manager status [name]              Show clean/local/upstream/both/unknown
skills-manager diff [name]                Compare installed files to current upstream
skills-manager diff --local [name]        Compare installation baseline to local files
skills-manager config set repo PATH       Save your contribution checkout
skills-manager contribute <name|--all> [--repo PATH] [--yes]
                                         Preview and copy local files to the checkout
skills-manager update [name|--all]              Update one or all global installations
skills-manager update <name|--all> --discard-local [--yes]
                                         Preview, discard edits without backup and update
skills-manager doctor                     Check source, installs, and local skills
skills-manager validate [path]            Validate a package or repository
```

### Interactive installer

The installer uses an aligned catalog, a focused skill/set detail panel, and a queue summary counting installs, updates, and removals. At 80×24 the details sit below the list; at widths of 110 columns or more (and at least 14 rows) they sit beside it. Short terminals prioritize the list; very small terminals show only focus and essential keys. Long text is ellipsized using terminal-cell widths.

Use Tab/Shift+Tab to cycle source tabs in either direction (wrapping), arrows or `j`/`k` to move, home/end or `g`/`G` to jump, space to toggle an install/update for a skill or set, `d` (or Delete) to toggle removal of an **installed skill row** (not a set or available skill), `r` to deliberately queue **discard local changes and update** (a set queues its installed members), enter to review, and `q`/escape to cancel. Tabs remember focus; selections persist across tabs. **Queue (all)** totals every source, and enter reviews the entire queue—even from an empty tab. In review, scroll with arrows, press `c` to confirm and proceed, Esc to go back without losing the queue, or `q` to cancel. An empty queue is a no-op; go back to select skills. The tabstrip scrolls/clips to keep the active tab visible; chevrons indicate hidden tabs. Markers remain readable without color: `[x]` selected, `[-]` removal, `[R]` queued discard, `[✓]` installed, `[↑]` upstream difference, `[!]` difference needing review. The detail panel shows the focused item's description, full status, and available action; selecting an update does not bypass manager-side protection. Repository-owned skills in `.agents/skills/` are never removed or updated by the installer. Actions only change the queue until confirmation in review; pressing the same action again unqueues it. Unavailable actions explain why nothing was queued. Removal still refuses protected local edits or unknown-different content; `d` is not a force-delete shortcut. Sets report installed and queued member counts; remove individual members from skill rows. The footer wraps controls to the terminal width, with feedback on separate lines so it cannot replace navigation or action help. Ctrl+C cancels the installer.

Sources contain separate sets and skills section headers (with source/namespace labels); status colors distinguish local changes, upstream changes, conflicts, removals, clean installs, and available skills. Set `NO_COLOR` (even empty) or `TERM=dumb` for unstyled text; focus and status still have textual indicators. Interactive screen control requires a terminal output; redirected output is rejected without escape sequences, with guidance to use the ordinary CLI commands instead.

Actual renderer snapshots: [80×24](docs/tui/80x24.txt), [120×32](docs/tui/120x32.txt), and [review](docs/tui/review.txt). Regenerate them with `UPDATE_TUI_SNAPSHOTS=1 go test ./internal/tui -run 'TestInstallerSnapshots|TestReviewSnapshotAndScroll'`.

Press `b` in the installer for the “Blackjack and hookers” easter egg (just blackjack, no money or saved scores). Use `h` to hit, `s` to stand, and `n` for a new round after the result. The dealer stands on soft 17. Press `q` or escape to return without changing your skill selections; Ctrl+C cancels the installer.

## Editing and contributing installed skills

Edit the plain files in `~/.agents/skills/<namespace>/<skill>`, then review:

```bash
skills-manager status
skills-manager diff --local <skill>
skills-manager config set repo /path/to/team-skills
skills-manager contribute <skill>
# One-off checkout override; --yes explicitly accepts the printed preview:
skills-manager contribute <skill> --repo /path/to/other-checkout --yes
```

Each source's saved checkout field is exactly `repo`; `init` preserves it. With multiple sources, qualify the skill ID and use `--from NAME` for `config set repo` or a `--repo` override. Contribution verifies that the target is the source checkout itself or has a matching Git origin (SSH/HTTPS forms are accepted). For a local source, another checkout can point its origin at that source or share its origin. An unrelated checkout is rejected. This is a repository-identity check, not a guarantee that its contents are trustworthy.

Install/update records a verified package snapshot and original source/catalog path in `~/.agents/.skills-manager/<namespace>/`, **outside** the installed skill directories. Keep this state when moving/backing up your installations. `status` compares both installed files and current upstream against that baseline: `clean`, `local`, `upstream`, or `both`. An unavailable upstream package counts as an upstream change. Older installations without a baseline are `unknown` (with a current-upstream equality hint), not assumed to contain local edits. Status output aligns names and classifications. The interactive installer shows compact status labels and focused details, and uses color on supported terminals (set `NO_COLOR` to disable it). The installer and `doctor` use the same classifications; selectable differences are not necessarily safe updates. `diff --local` is unavailable for them. A matching install/update safely establishes their baseline; differing unknown installs must be reconciled manually or explicitly discarded with the destructive workflow below.

Update and uninstall refuse to discard local changes or unknown-different packages, including dependencies and operations selected in the TUI. CLI install/update and TUI selections preflight all involved sources before copying/removing selected packages. Source failures abort rather than being treated as empty catalogs; use `--from` to work explicitly with an available source. Ordinary updates never bypass these protections. Use the explicit discard workflow below only when you want to replace your edits. An update whose files are already identical to upstream is safe and accepts that content as the new baseline.

Contribution previews paths at the **original catalog location** and asks `[y/N]` before writing; EOF cancels. Use `--yes` for noninteractive acceptance. Each regular file is compared to its installation baseline (contents and Git’s owner-executable bit). Installed-only additions, edits and deletions are applied; checkout-only changes stay put. Identical changes need no action. Different changes to the same file, including delete/edit conflicts, are left for manual reconciliation. File/directory collisions are not recursively replaced. Empty directories are not synchronized.

Untracked paths ignored by the checkout’s Git rules, and untracked Python caches (`__pycache__/`, `.pytest_cache/`, `*.pyc`, `*.pyo`), are never copied or removed. Files tracked in the checkout index remain eligible. These exclusions affect contribution, not update/status protection. Both input packages must still pass package/path validation, including symlink rejection. The composed result is validated before writing; if invalid (e.g. a missing linked document), that entire skill is skipped. External packages and unknown provenance cannot be contributed.

`contribute --all` selects installed managed packages with local changes, including packages also changed upstream; it skips unknown baselines with a message. It routes each candidate to its configured source's `repo`, preflights all sources, previews source names and destination paths, and asks once. After confirmation the candidate plans, including checkout identity and effective ignores, are checked again before the first write. A hard safety, provenance or I/O error stops the batch. A file conflict does not block other eligible files or skills. Exit **3** means conflicts or invalid projections remain: inspect the listed paths, even if other changes were applied. A successful copy, no-op or cancellation exits 0. `--repo` across sources is rejected; select one with `--from NAME`. I/O failures or concurrent changes during copying can still leave a partial batch.

Contribution never stages, commits or pushes; it does not change the installed package or its baseline. Review the checkout and commit/push yourself. Once upstream matches your installed edits, `update <skill>` can establish a clean baseline. The tools do not provide cross-process locking; avoid concurrent edits/CLI runs during writes. TUI removals, installs, and updates are separate batches, not one transaction.

### Discard local changes and update

```sh
skills-manager update adwise/review --discard-local
# All installed skills across all configured sources (or filter with --from):
skills-manager update --all --discard-local
skills-manager update --all --discard-local --from adwise
# Non-interactive explicit acceptance (still previews; NO backup):
skills-manager update --all --discard-local --yes
```

Requires one explicit qualified or uniquely installed skill ID, or `--all`; combining a name with `--all` is rejected. No-name discard and set selectors are not supported. `--all` seeds only installed skills, defaults to all configured sources, and supports `--from NAME`. Newly required dependencies are installed through the usual dependency expansion. `--discard-local` is rejected for other commands; `--all` is valid only for update and contribute. The preview compares **actual installed files → latest source**, listing every affected skill and dependency, including locally edited dependencies and newly required packages. Confirmation authorizes the entire displayed dependency selection, not just the named skill. Cancellation makes no installation or baseline changes. Ordinary `update` still refuses local changes.

In the TUI, `r` queues a discard separately from an ordinary update, with `[R]`, a compact magenta `discard` status (still explicit with `NO_COLOR`), and a discard count across tabs. The longer destructive warning stays in details/review. Enter opens a review screen, then exits to the full diff/dependency preview and a separate yes/no confirmation. Cancelling that confirmation cancels the whole queue. All source selections are preflighted and rechecked before replacement; an ordinary queued operation that conflicts with protected local edits still refuses (use a separate discard selection rather than expecting implicit authorization).

**Local modifications will be permanently overwritten without backup.** This applies to single-skill, all-skills and TUI discard updates, including dependencies. No recovery backup is created; existing backups are left untouched. `--yes` explicitly accepts this destructive action for the entire preview. All involved sources and targets are preflighted, and source content, dependency selection, installed files and baseline state are rechecked after confirmation before replacement. Successful updates use atomic per-package replacement and refresh the baseline.

Unknown or corrupt baseline metadata and invalid edited `SKILL.md` contents can be recovered through this CLI workflow without parsing the edited package; symlinks and special files are rejected. The TUI still requires valid installed packages to load its catalog/status view. An unavailable or unresolvable source aborts without replacing anything. Avoid concurrent CLI runs or edits: rechecks are not filesystem locks, and the whole multi-package operation is not transactional. A replacement or baseline write failure after earlier packages succeeded can leave a partial batch, with no automatic recovery backup.

## Skill repository format

```text
team-skills/
├── skills.json                       # Optional sets, dependencies, and external entries
└── skills/
    ├── code-review/SKILL.md
    └── personal/test/SKILL.md
```

Each `SKILL.md` starts with frontmatter:

```yaml
---
name: personal-test
description: A useful test workflow.
---
```

The frontmatter `name` is always the catalog ID. A package directly under `skills/` must use its directory name (`skills/code-review/SKILL.md` uses `name: code-review`). A nested package can use either its directory name (`skills/adwise/agent-browser/SKILL.md` uses `name: agent-browser`) or its immediate parent and directory name joined by a hyphen (`skills/personal/test/SKILL.md` uses `name: personal-test`; `skills/git/pull-request/SKILL.md` uses `name: git-pull-request`). Use that ID in commands, skill sets, and dependencies.

Standalone validation also accepts a package whose name matches its leaf directory, so installed packages can be validated and checksummed under their full catalog ID. Local discovery accepts both forms for nested packages; frontmatter names must remain unique across the catalog. Explicit external IDs keep their namespace semantics and must end with the external package's frontmatter name. Names and path segments use lowercase letters, digits, and hyphens. Additional package files can be referenced using relative paths inside the package.

An optional `skills.json` defines sets, skill dependencies, and pinned external skills:

```json
{
  "externalSkills": [
    {
      "id": "vercel/agent-browser",
      "repository": "https://github.com/vercel-labs/agent-browser.git",
      "ref": "aff6125c023b810ea3f2e5deec5379e9a4270bdc",
      "path": "skills/agent-browser"
    }
  ],
  "skillSets": [
    {
      "name": "review",
      "description": "Review workflows.",
      "skills": ["code-review"]
    }
  ]
}
```

Install with `skills-manager install @review personal-test`. External refs must be full 40- or 64-character commit hashes. External checkouts are shallow, partial, and sparse: only the requested package is materialized.

### Skill dependencies

Declare dependencies by their exact catalog IDs in `skills.json`:

```json
{
  "dependencies": {
    "skill-a": ["tools/mcporter"]
  }
}
```

Both skills must exist in the catalog (under `skills/` or in `externalSkills`). Dependencies can have dependencies of their own. Install commands, sets, and the interactive installer automatically install the complete dependency chain, once per skill, dependencies first. Missing skills, duplicate dependencies, and cycles are rejected before installation.

Updating a skill also updates its dependencies and installs newly required ones. Existing differing packages are never overwritten by `install`; use `update` explicitly. Dependencies are not automatically removed when uninstalling a skill, and manual uninstall can still remove a dependency needed by another skill. This declares dependencies on **skills**, not on binaries: installing a mcporter skill does not install the mcporter executable.

## Safety

The CLI validates frontmatter, paths, relative references, package boundaries, file types, external entries, and set membership. It rejects package symlinks and traversal. Copies are verified by SHA-256 before an atomic install/update swap. Updates compare current package contents with the recorded baseline; review upstream with `diff` and your edits with `diff --local` first.

There are no post-install hooks, automatic script execution, or symlink farms. Installing a skill does not make its instructions trustworthy: review sources and referenced scripts before using them with an agent.

## Development

```bash
go test ./...
go vet ./...
go run ./cmd/skills-manager --help
# Validate your separate skill repository:
go run ./cmd/skills-manager validate /path/to/team-skills
```

## License

MIT; see [LICENSE](LICENSE).
