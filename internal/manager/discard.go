package manager

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/adwise/developer-skills-manager/internal/diff"
	"github.com/adwise/developer-skills-manager/internal/skill"
)

// DiscardPlan is intentionally separate from ordinary update protections.
// All plans must be checked after confirmation before applying any of them.
type DiscardPlan struct {
	Check func() error
	Apply func() error
}

// PrepareDiscard previews every resolved dependency, including packages with an
// unknown/corrupt baseline or invalid edited frontmatter. No installed files change.
func (m Manager) PrepareDiscard(names []string) (*DiscardPlan, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("discard-local requires an explicit installed skill")
	}
	for _, name := range names {
		target, err := m.globalTarget(name)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(target)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("not an installed directory: %s", target)
		}
	}
	catalog, root, err := m.Repository.Registry(true)
	if err != nil {
		return nil, err
	}
	requested := append([]string(nil), names...)
	names, err = catalog.ResolveDependencies(names)
	if err != nil {
		return nil, err
	}
	items := indexCatalog(catalog.Skills)
	type entry struct{ id, target, state, localHash, sourceHash, stateHash string }
	var entries []entry
	for _, id := range names {
		target, err := m.globalTarget(id)
		if err != nil {
			return nil, err
		}
		if err := m.ensureNamespaceAvailable(id); err != nil {
			return nil, err
		}
		item, exists := items[id]
		if !exists {
			return nil, fmt.Errorf("skill %s is missing from source", id)
		}
		if _, err := skill.Validate(item.Path); err != nil {
			return nil, err
		}
		e := entry{id: id, target: target, state: m.stateDir(id)}
		for _, pair := range []struct {
			path string
			hash *string
		}{{target, &e.localHash}, {item.Path, &e.sourceHash}, {e.state, &e.stateHash}} {
			*pair.hash, err = discardTreeHash(pair.path)
			if err != nil {
				return nil, err
			}
		}
		text, err := diff.Directories(target, item.Path, id)
		if err != nil {
			return nil, err
		}
		action := "DISCARD installed files and update"
		if e.localHash == "missing" {
			action = "install dependency"
		}
		fmt.Fprintf(m.Out, "%s: %s (%s)\n%s", id, action, target, text)
		if err := previewTreeModes(m.Out, target, item.Path); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	check := func() error {
		current, currentRoot, err := m.Repository.Registry(false)
		if err != nil {
			return err
		}
		resolved, err := current.ResolveDependencies(requested)
		if err != nil {
			return err
		}
		if currentRoot != root || !slices.Equal(resolved, names) {
			return fmt.Errorf("source dependency selection changed since preview; retry")
		}
		currentItems := indexCatalog(current.Skills)
		for _, e := range entries {
			if currentItems[e.id].Path != items[e.id].Path || currentItems[e.id].External != items[e.id].External {
				return fmt.Errorf("source provenance changed since preview: %s", e.id)
			}
			if _, err := m.globalTarget(e.id); err != nil {
				return err
			}
			if err := m.ensureNamespaceAvailable(e.id); err != nil {
				return err
			}
			for _, pair := range []struct{ path, hash string }{{e.target, e.localHash}, {items[e.id].Path, e.sourceHash}, {e.state, e.stateHash}} {
				hash, err := discardTreeHash(pair.path)
				if err != nil {
					return err
				}
				if hash != pair.hash {
					return fmt.Errorf("%s changed since preview; retry: %s", e.id, pair.path)
				}
			}
		}
		return nil
	}
	apply := func() error {
		if err := check(); err != nil {
			return err
		}
		for _, e := range entries {
			item := items[e.id]
			if err := replacePackage(item.Path, e.target); err != nil {
				return err
			}
			if err := m.saveBaseline(item, root); err != nil {
				return err
			}
			fmt.Fprintf(m.Out, "↑ Updated %s (local changes discarded)\n", e.id)
		}
		return nil
	}
	return &DiscardPlan{Check: check, Apply: apply}, nil
}

// Include permission-only edits and empty directories, which the text diff
// intentionally does not represent.
func previewTreeModes(out io.Writer, before, after string) error {
	modes := func(root string) (map[string]fs.FileMode, error) {
		result := make(map[string]fs.FileMode)
		if _, err := os.Lstat(root); os.IsNotExist(err) {
			return result, nil
		}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			result[rel] = info.Mode()
			return nil
		})
		return result, err
	}
	oldModes, err := modes(before)
	if err != nil {
		return err
	}
	newModes, err := modes(after)
	if err != nil {
		return err
	}
	var paths []string
	for path := range oldModes {
		paths = append(paths, path)
	}
	for path := range newModes {
		if _, exists := oldModes[path]; !exists {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	for _, path := range paths {
		old, had := oldModes[path]
		next, has := newModes[path]
		if had && has && old != next {
			fmt.Fprintf(out, "Mode %s: %s → %s\n", path, old, next)
		}
		if !had && next.IsDir() {
			fmt.Fprintf(out, "Directory added: %s (%s)\n", path, next)
		}
		if !has && old.IsDir() {
			fmt.Fprintf(out, "Directory deleted: %s (%s)\n", path, old)
		}
	}
	return nil
}

// discardTreeHash operates on bytes, not SKILL.md semantics. It includes hidden
// files, empty directories and exact permission modes, and rejects links/devices.
// A missing tree is recorded explicitly (not confused with an empty directory).
func discardTreeHash(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if err := noSymlinkPath(string(filepath.Separator), abs); err != nil {
		return "", err
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return "missing", nil
	} else if err != nil {
		return "", err
	}
	h := sha256.New()
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsafe discard file: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%q %o\n", rel, info.Mode())
		if info.IsDir() {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		content := sha256.New()
		_, copyErr := io.Copy(content, input)
		if copyErr != nil {
			return copyErr
		}
		fmt.Fprintf(h, "%x\n", content.Sum(nil))
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
