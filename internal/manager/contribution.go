package manager

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

type packageContribution struct {
	record                baseline
	target, installed     string
	base, local, checkout tree
	ignored               map[string]bool
	writes                []string
	partial               bool
}

func (m Manager) inspectContribution(name, repo string, out io.Writer) (*packageContribution, error) {
	if err := requireName(name); err != nil {
		return nil, err
	}
	if repo == "" {
		return nil, fmt.Errorf("no checkout configured; use `skills-manager config set repo PATH` or --repo PATH")
	}
	b, base, err := m.readBaseline(name)
	if err != nil {
		return nil, err
	}
	if base == "" {
		return nil, fmt.Errorf("cannot contribute %s: unknown installation baseline/provenance", name)
	}
	identity, err := source.Identity(m.Repository.Source)
	if err != nil {
		return nil, err
	}
	if b.Source != identity {
		return nil, fmt.Errorf("cannot contribute %s: baseline belongs to another source", name)
	}
	if b.External {
		return nil, fmt.Errorf("cannot contribute %s: external packages are not supported; edit their original repository instead", name)
	}
	if strings.Contains("/"+strings.ToLower(b.Path)+"/", "/.git/") || !strings.HasPrefix(b.Path, "skills/") || filepath.IsAbs(b.Path) || filepath.ToSlash(filepath.Clean(b.Path)) != b.Path {
		return nil, fmt.Errorf("unsafe original catalog path %q", b.Path)
	}
	target := filepath.Join(repo, filepath.FromSlash(b.Path))
	if err := ensureChild(filepath.Join(repo, "skills"), target); err != nil {
		return nil, err
	}
	installed, err := m.globalTarget(name)
	if err != nil {
		return nil, err
	}
	if err := source.VerifyCheckout(b.Source, repo); err != nil {
		return nil, err
	}
	if err := noSymlinkPath(repo, target); err != nil {
		return nil, err
	}
	// A misconfigured destination must not turn contribution into an edit
	// of an installation or its saved provenance.
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, err
	}
	for _, protected := range []string{m.GlobalDir, filepath.Dir(m.stateDir(name))} {
		root, err := filepath.Abs(protected)
		if err != nil {
			return nil, err
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		if root == realTarget || ensureChild(root, realTarget) == nil || ensureChild(realTarget, root) == nil {
			return nil, fmt.Errorf("checkout package overlaps managed installation or state: %q", target)
		}
	}
	p := &packageContribution{record: b, target: target, installed: installed}
	if p.base, err = contributionSnapshot(base); err != nil {
		return nil, err
	}
	if p.local, err = contributionSnapshot(installed); err != nil {
		return nil, err
	}
	if p.checkout, err = contributionSnapshot(target); err != nil {
		return nil, err
	}
	for _, root := range []string{installed, target} {
		if _, err := skill.Validate(root); err != nil {
			var ioErr *os.PathError
			if errors.As(err, &ioErr) {
				return nil, err
			}
			// Validation errors may quote text from an ignored Markdown file.
			return nil, fmt.Errorf("invalid contribution input %q; validate the package before retrying", root)
		}
	}
	paths := unionPaths(p.base, p.local, p.checkout)
	p.ignored, err = ignoredContributionPaths(repo, b.Path, paths)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "Contribute %s → %q:\n", name, target)
	p.classify(paths, out)
	if len(p.writes) > 0 {
		// Apply the previewed checkout rules. Install rule files last, deepest
		// first, so our own writes cannot hide later approved paths.
		sort.SliceStable(p.writes, func(i, j int) bool {
			a, b := p.writes[i], p.writes[j]
			if filepath.Base(a) != ".gitignore" {
				return filepath.Base(b) == ".gitignore"
			}
			return filepath.Base(b) == ".gitignore" && strings.Count(a, "/") > strings.Count(b, "/")
		})
		if err := p.validateProjection(out); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *packageContribution) validateProjection(out io.Writer) error {
	tmp, err := os.MkdirTemp("", "skills-contribution-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	stage := filepath.Join(tmp, filepath.Base(filepath.Dir(p.target)), filepath.Base(p.target))
	if err := copyPackage(p.target, stage); err != nil {
		return err
	}
	for _, path := range p.writes {
		if err := p.applyFile(stage, path); err != nil {
			return err
		}
	}
	if _, err := skill.Validate(stage); err != nil {
		var ioErr *os.PathError
		if errors.As(err, &ioErr) {
			return err
		}
		fmt.Fprintln(out, "  skipped entire package: projected package fails validation; reconcile checkout and installed files")
		p.partial = true
		p.writes = nil
	}
	return nil
}

// Compare again immediately before unlink/rename. Never truncate a checkout
// inode (it might be hard-linked), recursively delete, or replace a directory.
func (p *packageContribution) applyFile(root, path string) error {
	before := p.checkout[path]
	if before != p.base[path] || before.kind == 'd' || (p.local[path].kind == 0 && before.kind != 'f') {
		return fmt.Errorf("unsafe contribution operation at %q", path)
	}
	dst := filepath.Join(root, filepath.FromSlash(path))
	src := filepath.Join(p.installed, filepath.FromSlash(path))
	if err := noSymlinkPath(p.installed, src); err != nil {
		return err
	}
	check := func() error {
		if err := noSymlinkPath(root, dst); err != nil {
			return err
		}
		n, err := inspectNode(dst)
		if err != nil {
			return err
		}
		if n != p.checkout[path] {
			return fmt.Errorf("checkout changed at %q; retry", path)
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	local := p.local[path]
	if local.kind == 0 {
		n, err := inspectNode(src)
		if err != nil {
			return err
		}
		if n.kind != 0 {
			return fmt.Errorf("installation changed at %q; retry", path)
		}
		if err := check(); err != nil {
			return err
		}
		return os.Remove(dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if p.checkout[path].kind != 0 {
		old, err := os.Lstat(dst)
		if err != nil {
			return err
		}
		mode = old.Mode().Perm()&^0100 | mode&0100
	}
	f, err := os.CreateTemp(filepath.Dir(dst), ".contribute-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = io.Copy(f, in)
	if err == nil {
		err = f.Chmod(mode)
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	got, err := inspectNode(f.Name())
	if err != nil {
		return err
	}
	if got != local {
		return fmt.Errorf("installed file changed at %q; retry", path)
	}
	if err := check(); err != nil {
		return err
	}
	if p.checkout[path].kind == 0 {
		return os.Link(f.Name(), dst)
	}
	return os.Rename(f.Name(), dst)
}

var ErrContributionPartial = errors.New("contribution incomplete: reconcile reported conflicts or invalid packages and retry")

type ContributionPlan struct {
	manager  Manager
	repo     string
	names    []string
	packages []*packageContribution
}

func (m Manager) prepare(repo string, names []string, batch bool) (*ContributionPlan, error) {
	if repo != "" {
		abs, err := filepath.Abs(repo)
		if err != nil {
			return nil, err
		}
		repo = abs
	}
	plan := &ContributionPlan{manager: m, repo: repo}
	for _, name := range names {
		if batch {
			_, base, err := m.readBaseline(name)
			if err != nil {
				return nil, err
			}
			if base == "" {
				fmt.Fprintf(m.Out, "%s: skipped (unknown baseline/provenance).\n", name)
				continue
			}
			installed, err := m.globalTarget(name)
			if err != nil {
				return nil, err
			}
			a, err := contributionSnapshot(base)
			if err != nil {
				return nil, err
			}
			b, err := contributionSnapshot(installed)
			if err != nil {
				return nil, err
			}
			if reflect.DeepEqual(a, b) {
				continue
			}
		}
		p, err := m.inspectContribution(name, repo, m.Out)
		if err != nil {
			return nil, err
		}
		plan.names = append(plan.names, name)
		plan.packages = append(plan.packages, p)
	}
	return plan, nil
}
func (m Manager) PrepareContributions(repo string) (*ContributionPlan, error) {
	names, err := m.selectedInstalled("")
	if err != nil {
		return nil, err
	}
	return m.prepare(repo, names, true)
}
func (p *ContributionPlan) recheck(i int) error {
	next, err := p.manager.inspectContribution(p.names[i], p.repo, io.Discard)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(next, p.packages[i]) {
		return fmt.Errorf("%s: contribution changed during confirmation; retry", p.names[i])
	}
	return nil
}

// ExecuteContributions confirms once and checks all sources before writing any.
func ExecuteContributions(plans []*ContributionPlan, confirm func() (bool, error), out io.Writer) error {
	pending, partial := false, false
	for _, plan := range plans {
		for _, p := range plan.packages {
			pending = pending || len(p.writes) > 0
			partial = partial || p.partial
		}
	}
	if pending {
		ok, err := confirm()
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
		for _, plan := range plans {
			for i := range plan.packages {
				if err := plan.recheck(i); err != nil {
					return err
				}
			}
		}
		for _, plan := range plans {
			for i, p := range plan.packages {
				if len(p.writes) == 0 {
					continue
				}
				if err := plan.recheck(i); err != nil {
					return err
				}
				for _, path := range p.writes {
					if err := noSymlinkPath(plan.repo, p.target); err != nil {
						return err
					}
					ignored, err := ignoredContributionPaths(plan.repo, p.record.Path, []string{path})
					if err != nil {
						return err
					}
					if ignored[path] {
						return fmt.Errorf("checkout ignore rules changed at %q; retry", path)
					}
					if err := p.applyFile(p.target, path); err != nil {
						return err
					}
					fmt.Fprintf(out, "%s: applied %q\n", plan.names[i], path)
				}
			}
		}
	} else {
		fmt.Fprintln(out, "No eligible changes to contribute.")
	}
	if partial {
		return ErrContributionPartial
	}
	return nil
}
func (m Manager) ContributeAll(repo string, confirm func() (bool, error)) error {
	p, err := m.PrepareContributions(repo)
	if err != nil {
		return err
	}
	return ExecuteContributions([]*ContributionPlan{p}, confirm, m.Out)
}
func (m Manager) Contribute(name, repo string, confirm func() (bool, error)) error {
	p, err := m.prepare(repo, []string{name}, false)
	if err != nil {
		return err
	}
	return ExecuteContributions([]*ContributionPlan{p}, confirm, m.Out)
}
