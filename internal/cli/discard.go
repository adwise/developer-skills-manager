package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adwise/developer-skills-manager/internal/manager"
	"github.com/adwise/developer-skills-manager/internal/tui"
)

func discardArguments(args []string) (string, bool, error) {
	var names []string
	yes, all := false, false
	for _, arg := range args {
		switch arg {
		case "--discard-local":
		case "--yes":
			yes = true
		case "--all":
			all = true
		default:
			if strings.HasPrefix(arg, "--") {
				return "", false, fmt.Errorf("unknown update option %s", arg)
			}
			names = append(names, arg)
		}
	}
	if all {
		if len(names) != 0 {
			return "", false, fmt.Errorf("--all cannot be combined with a skill name")
		}
		return "", yes, nil
	}
	if len(names) != 1 {
		return "", false, fmt.Errorf("update --discard-local requires one explicit installed skill or --all")
	}
	return names[0], yes, nil
}

func runDiscard(scopes []sourceManager, filtered bool, args []string, in io.Reader, out io.Writer) error {
	name, yes, err := discardArguments(args)
	if err != nil {
		return err
	}
	// Resolve against upstream IDs rather than parsing potentially invalid local
	// frontmatter. PrepareDiscard then requires the explicit target to exist.
	ids, err := selectionIDs(scopes, true)
	if err != nil {
		return err
	}
	for i, scope := range scopes {
		for id := range ids[i] {
			if strings.HasPrefix(id, "@") {
				delete(ids[i], id)
				continue
			}
			_, err := os.Lstat(filepath.Join(scope.GlobalDir, filepath.FromSlash(id)))
			if os.IsNotExist(err) {
				delete(ids[i], id)
			} else if err != nil {
				return err
			}
		}
	}
	selected := make([][]string, len(scopes))
	if name == "" {
		for i, scope := range scopes {
			// Catalog targets also recover installed packages with a deleted SKILL.md.
			// Walk package boundaries to catch installed IDs removed from upstream.
			err := filepath.WalkDir(scope.GlobalDir, func(path string, entry fs.DirEntry, err error) error {
				if os.IsNotExist(err) && path == scope.GlobalDir {
					return nil
				}
				if err != nil {
					return err
				}
				if !entry.IsDir() || path == scope.GlobalDir {
					return nil
				}
				rel, err := filepath.Rel(scope.GlobalDir, path)
				if err != nil {
					return err
				}
				id := filepath.ToSlash(rel)
				_, err = os.Lstat(filepath.Join(path, "SKILL.md"))
				if ids[i][id] || err == nil {
					ids[i][id] = true
					return filepath.SkipDir
				}
				if !os.IsNotExist(err) {
					return err
				}
				return nil
			})
			if err != nil {
				return err
			}
			for id := range ids[i] {
				selected[i] = append(selected[i], id)
			}
			slices.Sort(selected[i])
		}
	} else {
		i, id, err := resolveSelector(name, scopes, ids, filtered)
		if err != nil {
			return err
		}
		selected[i] = []string{id}
	}
	var plans []*manager.DiscardPlan
	for i, scope := range scopes {
		if len(selected[i]) == 0 {
			continue
		}
		fmt.Fprintf(out, "Source %s: discard local changes and update\n", scope.name)
		plan, err := scope.PrepareDiscard(selected[i])
		if err != nil {
			return err
		}
		plans = append(plans, plan)
	}
	if len(plans) == 0 {
		fmt.Fprintln(out, "No installed skills to update.")
		return nil
	}
	return confirmDiscard(plans, func() error { return nil }, in, out, yes)
}

func applyDiscardSelection(scopes []sourceManager, selections []tui.InstallerSelection, in io.Reader, out io.Writer) error {
	preflight := func() error {
		for i, scope := range scopes {
			s := selections[i]
			if err := scope.Preflight(s.Installs, s.Updates, s.Removals); err != nil {
				return err
			}
		}
		return nil
	}
	if err := preflight(); err != nil {
		return err
	}
	var plans []*manager.DiscardPlan
	for i, scope := range scopes {
		if len(selections[i].Discards) == 0 {
			continue
		}
		fmt.Fprintf(out, "Source %s: discard local changes and update\n", scope.name)
		plan, err := scope.PrepareDiscard(selections[i].Discards)
		if err != nil {
			return err
		}
		plans = append(plans, plan)
	}
	// Ordinary operations remain protected, and cancellation cancels the whole queue.
	plans = append(plans, &manager.DiscardPlan{Check: preflight, Apply: func() error {
		for i, scope := range scopes {
			s := selections[i]
			if len(s.Removals) > 0 {
				if err := scope.UninstallMany(s.Removals); err != nil {
					return err
				}
			}
			if len(s.Installs) > 0 {
				if err := scope.InstallMany(s.Installs); err != nil {
					return err
				}
			}
			if len(s.Updates) > 0 {
				if err := scope.UpdateMany(s.Updates); err != nil {
					return err
				}
			}
		}
		return nil
	}})
	return confirmDiscard(plans, preflight, in, out, false)
}

// Recheck the entire selection before the first replacement, across sources.
func confirmDiscard(plans []*manager.DiscardPlan, preflight func() error, in io.Reader, out io.Writer, yes bool) error {
	fmt.Fprintln(out, "DISCARD LOCAL CHANGES AND UPDATE\nAll packages listed above, including dependencies, will be replaced with source versions.\nWARNING: Local modifications will be permanently overwritten without backup. No merge will be attempted.")
	ok, err := confirmation(in, out, yes)()
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, "Cancelled.")
		return nil
	}
	if err := preflight(); err != nil {
		return err
	}
	for _, plan := range plans {
		if err := plan.Check(); err != nil {
			return err
		}
	}
	for _, plan := range plans {
		if err := plan.Apply(); err != nil {
			return err
		}
	}
	return nil
}
