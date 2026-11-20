package manager

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// The zero node denotes absence. Git records only the owner execute bit.
type node struct {
	kind       byte
	hash       [sha256.Size]byte
	executable bool
}
type tree map[string]node

func inspectNode(path string) (node, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return node{}, nil
	}
	if err != nil {
		return node{}, err
	}
	if info.IsDir() {
		return node{kind: 'd'}, nil
	}
	if !info.Mode().IsRegular() {
		return node{}, fmt.Errorf("unsupported file type: %q", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return node{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return node{}, err
	}
	n := node{kind: 'f', executable: info.Mode()&0100 != 0}
	copy(n.hash[:], h.Sum(nil))
	return n, nil
}
func contributionSnapshot(root string) (tree, error) {
	files := tree{}
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if strings.EqualFold(e.Name(), ".git") {
			return fmt.Errorf("unsafe package path %q", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		n, err := inspectNode(path)
		if err != nil {
			return err
		}
		if n.kind == 0 {
			return fmt.Errorf("path disappeared: %q", path)
		}
		files[filepath.ToSlash(rel)] = n
		return nil
	})
	return files, err
}
func unionPaths(trees ...tree) []string {
	seen := map[string]bool{}
	var paths []string
	for _, t := range trees {
		for path := range t {
			if !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
		}
	}
	sort.Strings(paths)
	return paths
}

// Query the checkout index separately: artifacts tracked there always participate,
// even when absent from the worktree or matched by a built-in cache exclusion.
func ignoredContributionPaths(repo, prefix string, paths []string) (map[string]bool, error) {
	data, err := exec.Command("git", "--literal-pathspecs", "-C", repo, "ls-files", "--cached", "-z", "--", prefix).Output()
	if err != nil {
		return nil, fmt.Errorf("query tracked paths: %w", err)
	}
	tracked := map[string]bool{}
	for _, p := range strings.Split(string(data), "\x00") {
		tracked[p] = true
	}
	ignored := map[string]bool{}
	var input strings.Builder
	for _, p := range paths {
		full := prefix + "/" + p
		if tracked[full] {
			continue
		}
		if strings.Contains("/"+p+"/", "/__pycache__/") || strings.Contains("/"+p+"/", "/.pytest_cache/") || strings.HasSuffix(p, ".pyc") || strings.HasSuffix(p, ".pyo") {
			ignored[p] = true
			continue
		}
		input.WriteString(full)
		input.WriteByte(0)
	}
	cmd := exec.Command("git", "-C", repo, "check-ignore", "--no-index", "-v", "-z", "--stdin")
	cmd.Stdin = strings.NewReader(input.String())
	data, err = cmd.Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return nil, fmt.Errorf("query ignore rules: %w", err)
	}
	// Verbose output contains four NUL-separated fields, including negations.
	fields := strings.Split(string(data), "\x00")
	if len(fields)%4 != 1 {
		return nil, fmt.Errorf("malformed git check-ignore output")
	}
	for i := 0; i+3 < len(fields); i += 4 {
		if !strings.HasPrefix(fields[i+2], "!") {
			ignored[strings.TrimPrefix(fields[i+3], prefix+"/")] = true
		}
	}
	return ignored, nil
}

func (p *packageContribution) classify(paths []string, out io.Writer) {
	// Directory nodes have no content hash: account for local edits below a
	// directory before deciding a checkout type change is checkout-only.
	changedBelow := map[string]bool{}
	for _, path := range paths {
		if p.ignored[path] || p.base[path] == p.local[path] {
			continue
		}
		for dir := filepath.ToSlash(filepath.Dir(path)); dir != "."; dir = filepath.ToSlash(filepath.Dir(dir)) {
			changedBelow[dir] = true
		}
	}
	blocked := map[string]bool{}
	for _, path := range paths {
		b, l, c := p.base[path], p.local[path], p.checkout[path]
		parent := filepath.ToSlash(filepath.Dir(path))
		if blocked[parent] {
			blocked[path] = true
			continue
		}
		if l == c {
			continue
		}
		// Never turn a directory into a file or remove a file to make room for a directory.
		structural := (b.kind == 'd' || l.kind == 'd' || c.kind == 'd') && (b.kind == 'f' || l.kind == 'f' || c.kind == 'f')
		if structural {
			blocked[path] = true
		}
		if p.ignored[path] && !(structural && changedBelow[path]) {
			continue
		}
		if l == b && !(structural && changedBelow[path]) {
			if c.kind != 'd' || structural {
				fmt.Fprintf(out, "  kept %q (checkout change)\n", path)
			}
			continue
		}
		if !structural && c.kind != 'f' && l.kind != 'f' && b.kind != 'f' {
			continue
		}
		// A checkout-deleted directory must not be recreated for a new local child.
		ancestorDeleted := false
		for dir := parent; dir != "."; dir = filepath.ToSlash(filepath.Dir(dir)) {
			if p.base[dir].kind == 'd' && p.checkout[dir].kind == 0 {
				ancestorDeleted = true
				break
			}
		}
		if structural || c != b || ancestorDeleted {
			p.partial = true
			fmt.Fprintf(out, "  conflict %q (kept checkout; reconcile manually)\n", path)
			continue
		}
		p.writes = append(p.writes, path)
		verb := "edited"
		if l.kind == 0 {
			verb = "deleted"
		} else if c.kind == 0 {
			verb = "added"
		}
		fmt.Fprintf(out, "  File %s: %q\n", verb, path)
	}
}
