# Developer Skills Manager

Repository: `adwise/developer-skills-manager`. CLI command: `skills-manager`.

A Git-native CLI for installing agent skills as plain files. Bring your own public or private Git repository; there is no central registry, account service, or bundled company catalog.

## Install and configure

### Homebrew (Adwise tap)

The [Adwise tap](https://github.com/adwise/homebrew-brew) has a prepared HEAD-only formula. Once the new CLI repository and tap changes are published:

```bash
brew install --HEAD adwise/brew/skills-manager
skills-manager init --source git@github.com:example/team-skills.git
skills-manager install
```

This temporary HEAD installation builds from source and requires Go during the build. Upgrade with `brew upgrade --fetch-HEAD adwise/brew/skills-manager`. After the first stable release is pinned and bottles are published, use `brew install adwise/brew/skills-manager` and `brew upgrade adwise/brew/skills-manager` instead. The new formula is not published yet; the existing `developer-skills` formula still installs the old `skills` command. While the tap or bottles remain private, follow the tap's authentication instructions. A public CLI repository alone does not make private bottles publicly downloadable.

### Go / local build

Requires Go 1.23+ and Git:

```bash
go install github.com/adwise/developer-skills-manager/cmd/skills-manager@latest
```

Alternatively, build from a local checkout:

```bash
go build -o build/skills-manager ./cmd/skills-manager
./build/skills-manager init --source /path/to/team-skills
./build/skills-manager install
```

Private repositories use your existing Git SSH or credential-helper configuration. Do not embed access tokens in repository URLs; the source URL is saved in configuration and may appear in output.

`init` checks that the source has a `skills/` directory and atomically saves the source and namespace in `~/.config/skills-manager/config.json`. Local paths are saved as absolute paths. Full package and manifest validation happens when listing/installing skills or running `validate`.

Global installations live in `~/.agents/skills/skills-manager/`. To use another namespace, including an existing installation directory:

```bash
skills-manager init --source git@github.com:example/team-skills.git --namespace team
```

This uses `~/.agents/skills/team/`; it does not move or remove existing skills. One configured source is supported at a time. Use a different namespace and cache when managing a second source with `--source`; changing sources does not migrate installed skills.

Options work before or after commands. Precedence is **flags > environment > saved configuration > defaults**:

| Flag | Environment | Default |
| --- | --- | --- |
| `--source` | `SKILLS_MANAGER_SOURCE` | Saved source; no built-in repository |
| `--namespace` | `SKILLS_MANAGER_NAMESPACE` | Saved namespace or `skills-manager` |
| `--home` | `SKILLS_MANAGER_HOME` | User home (skills and configuration) |
| `--cache` | `SKILLS_MANAGER_CACHE` | OS user cache + `skills-manager/repository` |

Only source and namespace are persisted by `init`; home and cache overrides apply to that invocation.

## Commands

```text
skills-manager init --source <url-or-path>  Configure a source repository
skills-manager available [query]          List or search skills
skills-manager sets                       List skill sets
skills-manager list                       List global and repository-owned skills
skills-manager install                    Interactive multi-select installer
skills-manager install <name|@set>...      Install skills or sets
skills-manager tui                        Interactive installer
skills-manager uninstall <name>           Remove a global installation
skills-manager diff [name]                Review upstream changes
skills-manager update [name]              Update one or all global installations
skills-manager doctor                     Check source, installs, and local skills
skills-manager validate [path]            Validate a package or repository
```

The installer marks installed skills with `✓` and updates with `↑`. Use arrows to move, space to select, `d` to mark a removal, enter to apply, and `q` to cancel. Repository-owned skills in `.agents/skills/` are never removed or updated by the installer.

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
name: test
description: A useful test workflow.
---
```

The name matches the final directory name. Catalog IDs preserve namespaces (`personal/test`); path segments use lowercase letters, digits, and hyphens. Additional package files can be referenced using relative paths inside the package.

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

Install with `skills-manager install @review personal/test`. External refs must be full 40- or 64-character commit hashes. External checkouts are shallow, partial, and sparse: only the requested package is materialized.

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

The CLI validates frontmatter, paths, relative references, package boundaries, file types, external entries, and set membership. It rejects package symlinks and traversal. Copies are verified by SHA-256 before an atomic install/update swap. Updates use Git history; review them with `diff` first.

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
