package cli

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/adwise/developer-skills-manager/internal/manager"
	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
	"github.com/adwise/developer-skills-manager/internal/tui"
)

// Never repurpose or reset the legacy cache. Source identity also isolates a
// removed/re-added name from its former checkout and external package cache.
func sourceCache(base, name, value string) string {
	return filepath.Join(base+"-sources", name, fmt.Sprintf("%x", sha256.Sum256([]byte(value))))
}

type sourceManager struct {
	name string
	repo string
	manager.Manager
}

func configuredManagers(opts options, cwd string, out io.Writer) ([]sourceManager, error) {
	if len(opts.sources) > 1 && opts.from == "" && (opts.sourceOverride || opts.namespaceOverride || opts.repoOverride != "") {
		return nil, fmt.Errorf("--source, --namespace and --repo overrides require --from NAME with multiple sources")
	}
	var result []sourceManager
	for _, name := range sourceNames(configuration{Sources: opts.sources}) {
		if opts.from != "" && opts.from != name {
			continue
		}
		entry := opts.sources[name]
		if opts.sourceOverride {
			entry.Source = opts.source
		}
		if opts.namespaceOverride && opts.namespace != entry.Namespace {
			return nil, fmt.Errorf("--namespace cannot redirect a configured source; use source add with a unique namespace")
		}
		if opts.repoOverride != "" {
			entry.Repo = opts.repoOverride
		}
		result = append(result, sourceManager{name: name, repo: entry.Repo, Manager: manager.Manager{
			Repository: source.Repository{Source: entry.Source, CacheDir: sourceCache(opts.cache, name, entry.Source)},
			GlobalDir:  filepath.Join(opts.home, ".agents", "skills", entry.Namespace),
			StateDir:   filepath.Join(opts.home, ".agents", ".skills-manager", entry.Namespace),
			LocalDir:   filepath.Join(cwd, ".agents", "skills"), Out: out,
		}})
	}
	return result, nil
}

// Match whole catalog IDs, not slash components: nested IDs can themselves
// begin with a configured source name. Ambiguous spellings fail closed.
func resolveSelector(selector string, scopes []sourceManager, ids []map[string]bool, filtered bool) (int, string, error) {
	type match struct {
		index int
		id    string
	}
	var matches []match
	for i, scope := range scopes {
		for id := range ids[i] {
			if selector == id || selector == scope.name+"/"+id {
				matches = append(matches, match{i, id})
			}
		}
	}
	if filtered {
		for _, candidate := range matches {
			if candidate.id == selector {
				return candidate.index, candidate.id, nil
			}
		}
	}
	if len(matches) == 1 {
		return matches[0].index, matches[0].id, nil
	}
	if len(matches) > 1 {
		return 0, "", fmt.Errorf("ambiguous ID %q; use --from NAME and the source-local ID", selector)
	}
	return 0, "", fmt.Errorf("unknown skill or set %q", selector)
}

func selectionIDs(scopes []sourceManager, available bool) ([]map[string]bool, error) {
	ids := make([]map[string]bool, len(scopes))
	for i, scope := range scopes {
		ids[i] = make(map[string]bool)
		var items []skill.Metadata
		if available {
			catalog, _, err := scope.Repository.Registry(true)
			if err != nil {
				return nil, fmt.Errorf("source %s: %w", scope.name, err)
			}
			items = catalog.Skills
			for _, set := range catalog.Sets {
				ids[i]["@"+set.Name] = true
			}
		} else {
			if _, err := os.Stat(scope.GlobalDir); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return nil, err
			}
			var err error
			items, err = skill.DiscoverInstalled(scope.GlobalDir)
			if err != nil {
				return nil, fmt.Errorf("source %s: %w", scope.name, err)
			}
		}
		for _, item := range items {
			ids[i][item.ID] = true
		}
	}
	return ids, nil
}

func runSources(opts options, command string, args []string, cwd string, in io.Reader, out io.Writer) error {
	scopes, err := configuredManagers(opts, cwd, out)
	if err != nil {
		return err
	}
	if command == "tui" || command == "install" && len(args) == 0 {
		return noExtraArgs(args, func() error { return runSourcesInstaller(scopes, in, out) })
	}
	if command == "contribute" {
		return contributeSources(scopes, opts, args, in, out)
	}
	local := false
	if command == "diff" {
		var names []string
		for _, arg := range args {
			if arg == "--local" {
				local = true
			} else {
				names = append(names, arg)
			}
		}
		args = names
	}
	switch command {
	case "available", "search", "list", "sets", "doctor":
		if command != "available" && command != "search" && len(args) > 0 {
			return fmt.Errorf("this command does not accept arguments")
		}
		for _, scope := range scopes {
			fmt.Fprintf(out, "Source %s (%s):\n", scope.name, filepath.Base(scope.GlobalDir))
			var err error
			switch command {
			case "available", "search":
				err = scope.Available(strings.Join(args, " "))
			case "list":
				err = scope.List()
			case "sets":
				err = scope.Sets()
			case "doctor":
				err = scope.Doctor()
			}
			if err != nil {
				return fmt.Errorf("source %s: %w", scope.name, err)
			}
		}
		return nil
	case "install", "update", "status", "diff", "uninstall", "remove":
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	if command != "install" && len(args) > 1 {
		return fmt.Errorf("this command accepts at most one argument")
	}
	if (command == "uninstall" || command == "remove") && len(args) == 0 {
		return fmt.Errorf("a skill name is required")
	}
	selected := make([][]string, len(scopes))
	if len(args) > 0 {
		ids, err := selectionIDs(scopes, command == "install")
		if err != nil {
			return err
		}
		for _, arg := range args {
			i, id, err := resolveSelector(arg, scopes, ids, opts.from != "")
			if err != nil {
				return err
			}
			selected[i] = append(selected[i], id)
		}
	}
	// Ensure every involved upstream is reachable before any cross-source writes.
	if command == "update" || command == "install" {
		for i, scope := range scopes {
			if len(args) > 0 && len(selected[i]) == 0 {
				continue
			}
			var err error
			if command == "install" {
				err = scope.Preflight(selected[i], nil, nil)
			} else {
				id := ""
				if len(selected[i]) > 0 {
					id = selected[i][0]
				}
				err = scope.PreflightUpdate(id)
			}
			if err != nil {
				return fmt.Errorf("source %s: %w", scope.name, err)
			}
		}
	}
	for i, scope := range scopes {
		if len(args) > 0 && len(selected[i]) == 0 {
			continue
		}
		fmt.Fprintf(out, "Source %s (%s):\n", scope.name, filepath.Base(scope.GlobalDir))
		id := ""
		if len(selected[i]) > 0 {
			id = selected[i][0]
		}
		var err error
		switch command {
		case "install":
			err = scope.InstallMany(selected[i])
		case "update":
			err = scope.Update(id)
		case "status":
			err = scope.Status(id)
		case "diff":
			if local {
				err = scope.DiffLocal(id)
			} else {
				err = scope.Diff(id)
			}
		case "uninstall", "remove":
			err = scope.Uninstall(id)
		}
		if err != nil {
			return fmt.Errorf("source %s: %w", scope.name, err)
		}
	}
	return nil
}

func contributeSources(scopes []sourceManager, opts options, args []string, in io.Reader, out io.Writer) error {
	all, yes := false, false
	var names []string
	for _, arg := range args {
		switch arg {
		case "--all":
			all = true
		case "--yes":
			yes = true
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown contribute option %s", arg)
			}
			names = append(names, arg)
		}
	}
	if !all {
		if len(names) != 1 {
			return fmt.Errorf("a skill name or --all is required")
		}
		ids, err := selectionIDs(scopes, false)
		if err != nil {
			return err
		}
		index, id, err := resolveSelector(names[0], scopes, ids, opts.from != "")
		if err != nil {
			return err
		}
		scope := scopes[index]
		fmt.Fprintf(out, "Source %s → %s\n", scope.name, scope.repo)
		return scope.Contribute(id, scope.repo, confirmation(in, out, yes))
	}
	if len(names) > 0 {
		return fmt.Errorf("--all cannot be combined with a skill name")
	}
	var plans []*manager.ContributionPlan
	for _, scope := range scopes {
		fmt.Fprintf(out, "Source %s → %s\n", scope.name, scope.repo)
		plan, err := scope.PrepareContributions(scope.repo)
		if err != nil {
			return fmt.Errorf("source %s: %w", scope.name, err)
		}
		if plan != nil {
			plans = append(plans, plan)
		}
	}
	return manager.ExecuteContributions(plans, confirmation(in, out, yes), out)
}

func confirmation(in io.Reader, out io.Writer, yes bool) func() (bool, error) {
	return func() (bool, error) {
		if yes {
			return true, nil
		}
		fmt.Fprint(out, "Copy the displayed changes to these source checkouts (no commit/push)? [y/N] ")
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && err != io.EOF {
			return false, err
		}
		line = strings.ToLower(strings.TrimSpace(line))
		return line == "y" || line == "yes", nil
	}
}

func runSourcesInstaller(scopes []sourceManager, in io.Reader, out io.Writer) error {
	var groups []tui.SourceGroup
	for _, scope := range scopes {
		catalog, installed, updates, states, err := scope.InstallOptionsWithStatus()
		if err != nil {
			return fmt.Errorf("source %s: %w", scope.name, err)
		}
		groups = append(groups, tui.SourceGroup{Name: scope.name, Namespace: filepath.Base(scope.GlobalDir), Skills: catalog.Skills, Sets: catalog.Sets, Installed: installed, Updates: updates, States: states})
	}
	selected, err := tui.RunSourceInstaller(in, out, groups)
	if err != nil {
		return err
	}
	return applySourcesSelection(scopes, selected, in, out)
}

func applySourcesSelection(scopes []sourceManager, selected tui.InstallerSelection, in io.Reader, out io.Writer) error {
	// TUI IDs are constructed from known scopes; strip only the exact prefix.
	local := func(scope sourceManager, ids []string) []string {
		var result []string
		for _, id := range ids {
			if strings.HasPrefix(id, scope.name+"/") {
				result = append(result, strings.TrimPrefix(id, scope.name+"/"))
			}
		}
		return result
	}
	if len(selected.Discards) > 0 {
		var selections []tui.InstallerSelection
		for _, scope := range scopes {
			selections = append(selections, tui.InstallerSelection{Installs: local(scope, selected.Installs), Updates: local(scope, selected.Updates), Removals: local(scope, selected.Removals), Discards: local(scope, selected.Discards)})
		}
		return applyDiscardSelection(scopes, selections, in, out)
	}
	for _, scope := range scopes {
		if err := scope.Preflight(local(scope, selected.Installs), local(scope, selected.Updates), local(scope, selected.Removals)); err != nil {
			return fmt.Errorf("source %s: %w", scope.name, err)
		}
	}
	for _, scope := range scopes {
		if ids := local(scope, selected.Removals); len(ids) > 0 {
			if err := scope.UninstallMany(ids); err != nil {
				return err
			}
		}
		if ids := local(scope, selected.Installs); len(ids) > 0 {
			if err := scope.InstallMany(ids); err != nil {
				return err
			}
		}
		if ids := local(scope, selected.Updates); len(ids) > 0 {
			if err := scope.UpdateMany(ids); err != nil {
				return err
			}
		}
	}
	return nil
}
