package manager

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adwise/developer-skills-manager/internal/diff"
	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

type Manager struct {
	Repository source.Repository
	GlobalDir  string
	LocalDir   string
	Out        io.Writer
}

func (m Manager) Available(query string) error {
	catalog, _, err := m.Repository.Catalog(true)
	if err != nil {
		return err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	matched := 0
	for _, item := range catalog {
		if query != "" && !strings.Contains(strings.ToLower(item.ID+" "+item.Description), query) {
			continue
		}
		fmt.Fprintf(m.Out, "%s\n  %s\n", item.ID, item.Description)
		matched++
	}
	if matched == 0 {
		if query == "" {
			fmt.Fprintln(m.Out, "No skills are available.")
		} else {
			fmt.Fprintf(m.Out, "No skills match %q.\n", query)
		}
	}
	return nil
}

func (m Manager) List() error {
	global, err := discoverIfExists(m.GlobalDir)
	if err != nil {
		return err
	}
	local, err := discoverIfExists(m.LocalDir)
	if err != nil {
		return err
	}
	printList(m.Out, "Global", global)
	fmt.Fprintln(m.Out)
	printList(m.Out, "Local", local)
	return nil
}

func (m Manager) Install(name string) error {
	return m.InstallMany([]string{name})
}

func (m Manager) InstallMany(selectors []string) error {
	if len(selectors) == 0 {
		return fmt.Errorf("at least one skill name or @set is required")
	}
	for _, selector := range selectors {
		if strings.HasPrefix(selector, "@") {
			if !skill.IsValidName(strings.TrimPrefix(selector, "@")) {
				return fmt.Errorf("invalid skill set %q", selector)
			}
			continue
		}
		if err := requireName(selector); err != nil {
			return err
		}
	}
	catalog, _, err := m.Repository.Registry(true)
	if err != nil {
		return err
	}
	items := indexCatalog(catalog.Skills)
	names, err := expandSelectors(selectors, catalog.Sets, items)
	if err != nil {
		return err
	}
	names, err = catalog.ResolveDependencies(names)
	if err != nil {
		return err
	}

	// Check the complete selection before copying anything, so a conflicting
	// existing install does not leave a multi-skill command half-finished.
	for _, name := range names {
		item := items[name]
		target, err := m.globalTarget(name)
		if err != nil {
			return err
		}
		if err := m.ensureNamespaceAvailable(name); err != nil {
			return err
		}
		if _, err := os.Stat(target); err == nil {
			if _, err := os.Stat(filepath.Join(target, "SKILL.md")); os.IsNotExist(err) {
				return fmt.Errorf("cannot install %q: its target is an existing skill namespace", name)
			}
			same, err := samePackage(item.Path, target)
			if err != nil {
				return err
			}
			if !same {
				return fmt.Errorf("%s is already installed and differs from the source; run `skills-manager update %s`", name, name)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect installed skill %s: %w", name, err)
		}
	}

	for _, name := range names {
		item := items[name]
		target, _ := m.globalTarget(name)
		if _, err := os.Stat(target); err == nil {
			fmt.Fprintf(m.Out, "✓ %s is already installed.\n", name)
			continue
		}
		if err := replacePackage(item.Path, target); err != nil {
			return err
		}
		fmt.Fprintf(m.Out, "✓ Installed %s in %s\n", name, target)
	}
	return nil
}

func (m Manager) Uninstall(name string) error {
	return m.UninstallMany([]string{name})
}

func (m Manager) UninstallMany(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("at least one skill name is required")
	}
	type installation struct {
		name   string
		target string
	}
	installations := make([]installation, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		if err := requireName(name); err != nil {
			return err
		}
		target, err := m.globalTarget(name)
		if err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(target, "SKILL.md")); os.IsNotExist(err) {
			return fmt.Errorf("skill %q is not installed", name)
		} else if err != nil {
			return fmt.Errorf("inspect installed skill %s: %w", name, err)
		}
		installations = append(installations, installation{name: name, target: target})
		seen[name] = true
	}

	for _, item := range installations {
		if err := os.RemoveAll(item.target); err != nil {
			return fmt.Errorf("uninstall %s: %w", item.name, err)
		}
		removeEmptyParents(filepath.Dir(item.target), m.GlobalDir)
		fmt.Fprintf(m.Out, "✓ Uninstalled %s\n", item.name)
	}
	return nil
}

func (m Manager) Update(name string) error {
	names, err := m.selectedInstalled(name)
	if err != nil {
		return err
	}
	return m.updateSelected(names)
}

func (m Manager) UpdateMany(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("at least one skill name is required")
	}
	selected := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		installed, err := m.selectedInstalled(name)
		if err != nil {
			return err
		}
		selected = append(selected, installed...)
		seen[name] = true
	}
	return m.updateSelected(selected)
}

func (m Manager) updateSelected(names []string) error {
	if len(names) == 0 {
		fmt.Fprintln(m.Out, "No global skills are installed.")
		return nil
	}
	catalog, _, err := m.Repository.Registry(true)
	if err != nil {
		return err
	}
	available := indexCatalog(catalog.Skills)
	names, err = catalog.ResolveDependencies(names)
	if err != nil {
		return err
	}
	// Check every dependency target before installing or updating any package.
	for _, id := range names {
		target, err := m.globalTarget(id)
		if err != nil {
			return err
		}
		if err := m.ensureNamespaceAvailable(id); err != nil {
			return err
		}
		if _, err := os.Stat(target); err == nil {
			if _, err := skill.Validate(target); err != nil {
				return fmt.Errorf("inspect installed skill %s: %w", id, err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect installed skill %s: %w", id, err)
		}
	}

	updated := 0
	for _, skillName := range names {
		item, exists := available[skillName]
		if !exists {
			return fmt.Errorf("installed skill %q no longer exists in the source repository", skillName)
		}
		sourceDir := item.Path
		target, err := m.globalTarget(skillName)
		if err != nil {
			return err
		}
		if _, err := os.Stat(target); os.IsNotExist(err) {
			if err := replacePackage(sourceDir, target); err != nil {
				return err
			}
			fmt.Fprintf(m.Out, "✓ Installed %s in %s\n", skillName, target)
			updated++
			continue
		}
		same, err := samePackage(sourceDir, target)
		if err != nil {
			return err
		}
		if same {
			fmt.Fprintf(m.Out, "✓ %s is up to date.\n", skillName)
			continue
		}
		if err := replacePackage(sourceDir, target); err != nil {
			return err
		}
		fmt.Fprintf(m.Out, "↑ Updated %s\n", skillName)
		updated++
	}
	if updated == 0 && len(names) > 1 {
		fmt.Fprintln(m.Out, "All global skills are up to date.")
	}
	return nil
}

func (m Manager) Diff(name string) error {
	catalog, _, err := m.Repository.Registry(true)
	if err != nil {
		return err
	}
	availableByID := indexCatalog(catalog.Skills)
	names, err := m.selectedInstalled(name)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintln(m.Out, "No global skills are installed.")
		return nil
	}

	differences := 0
	for _, skillName := range names {
		item, exists := availableByID[skillName]
		if !exists {
			return fmt.Errorf("installed skill %q no longer exists in the source repository", skillName)
		}
		installed, err := m.globalTarget(skillName)
		if err != nil {
			return err
		}
		available := item.Path
		same, err := samePackage(installed, available)
		if err != nil {
			return err
		}
		if same {
			fmt.Fprintf(m.Out, "✓ %s is up to date.\n", skillName)
			continue
		}
		contents, err := diff.Directories(installed, available, skillName)
		if err != nil {
			return err
		}
		fmt.Fprintf(m.Out, "\n%s\n%s", skillName, contents)
		differences++
	}
	if differences == 0 && len(names) > 1 {
		fmt.Fprintln(m.Out, "All global skills are up to date.")
	}
	return nil
}

func (m Manager) Doctor() error {
	failures := 0
	if info, err := os.Stat(m.GlobalDir); err == nil && info.IsDir() {
		fmt.Fprintf(m.Out, "✓ Global skill directory: %s\n", m.GlobalDir)
	} else if os.IsNotExist(err) {
		fmt.Fprintf(m.Out, "! Global skill directory does not exist yet: %s\n", m.GlobalDir)
	} else {
		fmt.Fprintf(m.Out, "✗ Global skill directory: %v\n", err)
		failures++
	}

	catalog, _, err := m.Repository.Registry(true)
	if err != nil {
		fmt.Fprintf(m.Out, "✗ Source repository: %v\n", err)
		return fmt.Errorf("doctor found %d problem(s)", failures+1)
	}
	availableByID := indexCatalog(catalog.Skills)
	fmt.Fprintf(m.Out, "✓ Source repository: %s\n", m.Repository.Source)

	installed, err := discoverIfExists(m.GlobalDir)
	if err != nil {
		fmt.Fprintf(m.Out, "✗ Global skills: %v\n", err)
		failures++
	} else {
		for _, item := range installed {
			available, exists := availableByID[item.ID]
			if !exists {
				fmt.Fprintf(m.Out, "✗ %s is installed but unavailable in the source\n", item.ID)
				failures++
				continue
			}
			same, err := samePackage(item.Path, available.Path)
			if err != nil {
				fmt.Fprintf(m.Out, "✗ %s: %v\n", item.ID, err)
				failures++
			} else if same {
				fmt.Fprintf(m.Out, "✓ %s is valid and up to date\n", item.ID)
			} else {
				fmt.Fprintf(m.Out, "↑ %s has an update available\n", item.ID)
			}
		}
	}

	local, err := discoverIfExists(m.LocalDir)
	if err != nil {
		fmt.Fprintf(m.Out, "✗ Repository skills: %v\n", err)
		failures++
	} else {
		for _, item := range local {
			fmt.Fprintf(m.Out, "✓ Local skill %s is valid\n", item.ID)
		}
	}
	if failures > 0 {
		return fmt.Errorf("doctor found %d problem(s)", failures)
	}
	return nil
}

func (m Manager) Validate(path string) error {
	if path != "" {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", path)
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
			metadata, err := skill.Validate(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(m.Out, "✓ %s\n", metadata.ID)
			return nil
		}
		roots := []string{filepath.Join(path, "skills"), filepath.Join(path, ".agents", "skills")}
		foundRoot := false
		if _, err := os.Stat(filepath.Join(path, "skills.json")); err == nil {
			catalog, _, err := (source.Repository{Source: path, CacheDir: m.Repository.CacheDir}).Registry(false)
			if err != nil {
				return err
			}
			for _, item := range catalog.Skills {
				fmt.Fprintf(m.Out, "✓ %s\n", item.ID)
			}
			foundRoot = true
			roots = roots[1:]
		}
		for _, root := range roots {
			if _, err := os.Stat(root); os.IsNotExist(err) {
				continue
			}
			foundRoot = true
			catalog, err := skill.Discover(root)
			if err != nil {
				return err
			}
			for _, item := range catalog {
				fmt.Fprintf(m.Out, "✓ %s\n", item.ID)
			}
		}
		if !foundRoot {
			catalog, err := skill.Discover(path)
			if err != nil {
				return err
			}
			for _, item := range catalog {
				fmt.Fprintf(m.Out, "✓ %s\n", item.ID)
			}
		}
		return nil
	}

	validated := 0
	roots := []string{"skills", m.LocalDir}
	if _, err := os.Stat("skills.json"); err == nil {
		catalog, _, err := (source.Repository{Source: ".", CacheDir: m.Repository.CacheDir}).Registry(false)
		if err != nil {
			return err
		}
		for _, item := range catalog.Skills {
			fmt.Fprintf(m.Out, "✓ %s\n", item.ID)
			validated++
		}
		roots = roots[1:]
	}
	for _, root := range roots {
		catalog, err := discoverIfExists(root)
		if err != nil {
			return err
		}
		for _, item := range catalog {
			fmt.Fprintf(m.Out, "✓ %s\n", item.ID)
			validated++
		}
	}
	if validated == 0 {
		fmt.Fprintln(m.Out, "No skills found to validate.")
	}
	return nil
}

func (m Manager) InstallOptions() (source.CatalogData, map[string]bool, error) {
	catalog, installed, _, err := m.InstallOptionsWithUpdates()
	return catalog, installed, err
}

func (m Manager) InstallOptionsWithUpdates() (source.CatalogData, map[string]bool, map[string]bool, error) {
	catalog, _, err := m.Repository.Registry(true)
	if err != nil {
		return source.CatalogData{}, nil, nil, err
	}
	items, err := discoverIfExists(m.GlobalDir)
	if err != nil {
		return source.CatalogData{}, nil, nil, err
	}
	available := indexCatalog(catalog.Skills)
	installed := make(map[string]bool, len(items))
	updates := make(map[string]bool)
	for _, item := range items {
		installed[item.ID] = true
		candidate, exists := available[item.ID]
		if !exists {
			catalog.Skills = append(catalog.Skills, item)
			continue
		}
		same, err := samePackage(item.Path, candidate.Path)
		if err != nil {
			return source.CatalogData{}, nil, nil, err
		}
		if !same {
			updates[item.ID] = true
		}
	}
	// Manifest-only dependency changes also need to be actionable in the TUI.
	for _, item := range items {
		if _, exists := available[item.ID]; !exists {
			continue
		}
		dependencies, err := catalog.ResolveDependencies([]string{item.ID})
		if err != nil {
			return source.CatalogData{}, nil, nil, err
		}
		for _, id := range dependencies {
			if !installed[id] {
				updates[item.ID] = true
			}
		}
	}
	sort.Slice(catalog.Skills, func(left, right int) bool {
		return catalog.Skills[left].ID < catalog.Skills[right].ID
	})
	return catalog, installed, updates, nil
}

func (m Manager) Sets() error {
	catalog, _, err := m.Repository.Registry(true)
	if err != nil {
		return err
	}
	if len(catalog.Sets) == 0 {
		fmt.Fprintln(m.Out, "No skill sets are defined.")
		return nil
	}
	for _, set := range catalog.Sets {
		fmt.Fprintf(m.Out, "@%s\n  %s\n  %s\n", set.Name, set.Description, strings.Join(set.Skills, ", "))
	}
	return nil
}

func (m Manager) selectedInstalled(name string) ([]string, error) {
	if name != "" {
		if err := requireName(name); err != nil {
			return nil, err
		}
		target, err := m.globalTarget(name)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(filepath.Join(target, "SKILL.md")); os.IsNotExist(err) {
			return nil, fmt.Errorf("skill %q is not installed", name)
		} else if err != nil {
			return nil, fmt.Errorf("inspect installed skill %s: %w", name, err)
		}
		return []string{name}, nil
	}
	items, err := discoverIfExists(m.GlobalDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.ID)
	}
	return names, nil
}

func discoverIfExists(root string) ([]skill.Metadata, error) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}
	return skill.Discover(root)
}

func printList(out io.Writer, heading string, items []skill.Metadata) {
	fmt.Fprintf(out, "%s:\n", heading)
	if len(items) == 0 {
		fmt.Fprintln(out, "  (none)")
		return
	}
	for _, item := range items {
		fmt.Fprintf(out, "  %s\n", item.ID)
	}
}

func requireName(name string) error {
	if name == "" {
		return fmt.Errorf("a skill name is required")
	}
	if !skill.IsValidID(name) {
		return fmt.Errorf("invalid skill name %q; use slash-separated lowercase letters, digits, and hyphens", name)
	}
	return nil
}

func indexCatalog(items []skill.Metadata) map[string]skill.Metadata {
	indexed := make(map[string]skill.Metadata, len(items))
	for _, item := range items {
		indexed[item.ID] = item
	}
	return indexed
}

func expandSelectors(selectors []string, sets []source.SkillSet, available map[string]skill.Metadata) ([]string, error) {
	setsByName := make(map[string]source.SkillSet, len(sets))
	for _, set := range sets {
		setsByName[set.Name] = set
	}
	seen := make(map[string]struct{})
	var names []string
	add := func(name string) error {
		if err := requireName(name); err != nil {
			return err
		}
		if _, exists := available[name]; !exists {
			return fmt.Errorf("skill %q is not available", name)
		}
		if _, duplicate := seen[name]; !duplicate {
			seen[name] = struct{}{}
			names = append(names, name)
		}
		return nil
	}
	for _, selector := range selectors {
		if strings.HasPrefix(selector, "@") {
			setName := strings.TrimPrefix(selector, "@")
			set, exists := setsByName[setName]
			if !exists {
				return nil, fmt.Errorf("skill set %q is not available", setName)
			}
			for _, name := range set.Skills {
				if err := add(name); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := add(selector); err != nil {
			return nil, err
		}
	}
	return names, nil
}

func (m Manager) globalTarget(name string) (string, error) {
	if err := requireName(name); err != nil {
		return "", err
	}
	target := filepath.Join(m.GlobalDir, filepath.FromSlash(name))
	if err := ensureChild(m.GlobalDir, target); err != nil {
		return "", err
	}
	return target, nil
}

func (m Manager) ensureNamespaceAvailable(name string) error {
	parts := strings.Split(name, "/")
	for index := 1; index < len(parts); index++ {
		ancestor := filepath.Join(m.GlobalDir, filepath.Join(parts[:index]...))
		if _, err := os.Stat(filepath.Join(ancestor, "SKILL.md")); err == nil {
			return fmt.Errorf("cannot install %q: parent skill %q is already installed", name, strings.Join(parts[:index], "/"))
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect skill namespace %s: %w", ancestor, err)
		}
	}
	return nil
}

func removeEmptyParents(dir, stop string) {
	stopAbs, err := filepath.Abs(stop)
	if err != nil {
		return
	}
	for {
		dirAbs, err := filepath.Abs(dir)
		if err != nil || dirAbs == stopAbs {
			return
		}
		rel, err := filepath.Rel(stopAbs, dirAbs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return
		}
		if err := os.Remove(dirAbs); err != nil {
			return
		}
		dir = filepath.Dir(dirAbs)
	}
}

func samePackage(left, right string) (bool, error) {
	leftHash, err := skill.Checksum(left)
	if err != nil {
		return false, err
	}
	rightHash, err := skill.Checksum(right)
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

func replacePackage(sourceDir, target string) error {
	if _, err := skill.Validate(sourceDir); err != nil {
		return err
	}
	root := filepath.Dir(target)
	if err := ensureChild(root, target); err != nil {
		return err
	}
	if err := mkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create global skills directory: %w", err)
	}
	tempRoot, err := os.MkdirTemp(root, ".install-")
	if err != nil {
		return fmt.Errorf("create temporary install directory: %w", err)
	}
	defer os.RemoveAll(tempRoot)
	temp := filepath.Join(tempRoot, filepath.Base(target))

	if err := copyPackage(sourceDir, temp); err != nil {
		return err
	}
	sourceHash, err := skill.Checksum(sourceDir)
	if err != nil {
		return err
	}
	tempHash, err := skill.Checksum(temp)
	if err != nil {
		return err
	}
	if sourceHash != tempHash {
		return fmt.Errorf("integrity verification failed while installing %s", filepath.Base(target))
	}

	backupRoot, err := os.MkdirTemp(root, ".backup-")
	if err != nil {
		return fmt.Errorf("create temporary backup directory: %w", err)
	}
	defer os.RemoveAll(backupRoot)
	backup := filepath.Join(backupRoot, filepath.Base(target))
	hadTarget := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("prepare update for %s: %w", filepath.Base(target), err)
		}
		hadTarget = true
	}
	if err := os.Rename(temp, target); err != nil {
		if hadTarget {
			_ = os.Rename(backup, target)
		}
		return fmt.Errorf("install %s: %w", filepath.Base(target), err)
	}
	if hadTarget {
		if err := os.RemoveAll(backupRoot); err != nil {
			return fmt.Errorf("remove update backup for %s: %w", filepath.Base(target), err)
		}
	}
	return nil
}

func mkdirAll(path string, perm fs.FileMode) error {
	return mkdirAllFollowingSymlinks(filepath.Clean(path), perm, make(map[string]bool))
}

func mkdirAllFollowingSymlinks(path string, perm fs.FileMode, followed map[string]bool) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s exists and is not a directory", path)
		}
		if followed[path] {
			return fmt.Errorf("symbolic link cycle at %s", path)
		}
		followed[path] = true
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		return mkdirAllFollowingSymlinks(filepath.Clean(target), perm, followed)
	}
	if !os.IsNotExist(err) {
		return err
	}

	parent := filepath.Dir(path)
	if parent == path {
		return err
	}
	if err := mkdirAllFollowingSymlinks(parent, perm, followed); err != nil {
		return err
	}
	if err := os.Mkdir(path, perm); err != nil {
		if os.IsExist(err) {
			info, statErr := os.Stat(path)
			if statErr == nil && info.IsDir() {
				return nil
			}
		}
		return err
	}
	return nil
}

func copyPackage(sourceDir, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		destination := filepath.Join(target, rel)
		if err := ensureChild(target, destination); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func ensureChild(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing path outside skill directory: %s", target)
	}
	return nil
}
