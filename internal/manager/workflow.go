package manager

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adwise/developer-skills-manager/internal/diff"
	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

// Baselines live outside installed packages, so editing or copying a skill does
// not accidentally modify its provenance. Missing/corrupt state fails closed.
type baseline struct {
	Source   string `json:"source"`
	Path     string `json:"path"`
	Hash     string `json:"hash"`
	External bool   `json:"external"`
}

func (m Manager) stateDir(name string) string {
	dir := m.StateDir
	if dir == "" {
		dir = m.GlobalDir + ".state"
	}
	return filepath.Join(dir, fmt.Sprintf("%x", sha256.Sum256([]byte(name))))
}

func (m Manager) readBaseline(name string) (baseline, string, error) {
	var b baseline
	dir := m.stateDir(name)
	data, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if os.IsNotExist(err) {
		return b, "", nil
	}
	if err != nil {
		return b, "", err
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, "", fmt.Errorf("invalid baseline for %s: %w", name, err)
	}
	if len(b.Hash) != 64 || strings.Trim(b.Hash, "0123456789abcdef") != "" || b.Source == "" {
		return b, "", fmt.Errorf("invalid baseline for %s", name)
	}
	snapshot := filepath.Join(dir, b.Hash, filepath.Base(name))
	hash, err := skill.Checksum(snapshot)
	if err != nil {
		return b, "", err
	}
	if hash != b.Hash {
		return b, "", fmt.Errorf("baseline integrity check failed for %s", name)
	}
	return b, snapshot, nil
}

func (m Manager) saveBaseline(item skill.Metadata, root string) error {
	// Snapshot what was actually installed, not a potentially changing checkout.
	installed, err := m.globalTarget(item.ID)
	if err != nil {
		return err
	}
	hash, err := skill.Checksum(installed)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, item.Path)
	if err != nil {
		return err
	}
	// External manifest packages are deliberately not mapped into the catalog.
	external := item.External || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || !strings.HasPrefix(filepath.ToSlash(relative), "skills/")
	identity, err := source.Identity(m.Repository.Source)
	if err != nil {
		return err
	}
	b := baseline{Source: identity, Path: filepath.ToSlash(relative), Hash: hash, External: external}
	dir := m.stateDir(item.ID)
	snapshot := filepath.Join(dir, hash, filepath.Base(item.ID))
	if _, err := os.Stat(snapshot); os.IsNotExist(err) {
		if err := replacePackage(installed, snapshot); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	actual, err := skill.Checksum(snapshot)
	if err != nil {
		return err
	}
	if actual != hash {
		return fmt.Errorf("baseline integrity check failed for %s", item.ID)
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".record-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), filepath.Join(dir, "record.json"))
}

// protect also covers legacy installations: only equality with the current
// source can establish that discarding an unknown baseline is safe.
func (m Manager) protect(name, target, upstream string) error {
	_, base, err := m.readBaseline(name)
	if err != nil {
		return err
	}
	comparison := base
	if comparison == "" {
		comparison = upstream
	}
	if comparison != "" {
		same, err := samePackage(target, comparison)
		if err != nil {
			return err
		}
		if same {
			return nil
		}
	}
	if base == "" {
		return fmt.Errorf("refusing to overwrite/remove %s: unknown baseline and no matching upstream package; use diff, back up and reconcile your files manually (local diff/contribution need a known baseline)", name)
	}
	return fmt.Errorf("refusing to overwrite/remove %s: local changes; back up your edits and review diff --local or contribute before retrying", name)
}

func (m Manager) Status(name string) error {
	names, err := m.selectedInstalled(name)
	if err != nil {
		return err
	}
	catalog, _, err := m.Repository.Registry(true)
	if err != nil {
		return err
	}
	items := indexCatalog(catalog.Skills)
	identity, err := source.Identity(m.Repository.Source)
	if err != nil {
		return err
	}
	width := 0
	for _, id := range names {
		if len(id)+1 > width {
			width = len(id) + 1
		}
	}
	for _, id := range names {
		target, err := m.globalTarget(id)
		if err != nil {
			return err
		}
		item, exists := items[id]
		state, err := m.packageStatus(id, target, item.Path, exists, identity)
		if err != nil {
			return err
		}
		fmt.Fprintf(m.Out, "%-*s %s\n", width, id+":", state)
	}
	return nil
}

// packageStatus compares both sides to the verified baseline when provenance matches.
func (m Manager) packageStatus(id, target, upstream string, exists bool, identity string) (string, error) {
	b, base, err := m.readBaseline(id)
	if err != nil {
		return "", err
	}
	if base != "" && b.Source == identity {
		local, err := samePackage(target, base)
		if err != nil {
			return "", err
		}
		upstreamSame := false
		if exists {
			upstreamSame, err = samePackage(upstream, base)
			if err != nil {
				return "", err
			}
		}
		switch {
		case local && upstreamSame:
			return "clean", nil
		case !local && upstreamSame:
			return "local", nil
		case local && !upstreamSame:
			return "upstream", nil
		default:
			return "both", nil
		}
	}
	if exists {
		same, err := samePackage(target, upstream)
		if err != nil {
			return "", err
		}
		if same {
			return "unknown (matches upstream)", nil
		}
		return "unknown (differs from upstream)", nil
	}
	return "unknown", nil
}

func (m Manager) DiffLocal(name string) error {
	names, err := m.selectedInstalled(name)
	if err != nil {
		return err
	}
	for _, id := range names {
		_, base, err := m.readBaseline(id)
		if err != nil {
			return err
		}
		if base == "" {
			fmt.Fprintf(m.Out, "%s: unknown baseline; local diff unavailable (use diff for upstream comparison).\n", id)
			continue
		}
		target, err := m.globalTarget(id)
		if err != nil {
			return err
		}
		if _, err := skill.Validate(target); err != nil {
			return err
		}
		text, err := diff.Directories(base, target, id)
		if err != nil {
			return err
		}
		fmt.Fprintf(m.Out, "%s (baseline → installed):\n%s", id, text)
	}
	return nil
}

func noSymlinkPath(root, target string) error {
	root, target = filepath.Clean(root), filepath.Clean(target)
	for p := target; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", p)
		}
		if p == root {
			return nil
		}
		if filepath.Dir(p) == p {
			return fmt.Errorf("path outside checkout")
		}
	}
}
