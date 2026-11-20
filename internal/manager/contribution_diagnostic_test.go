package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/skill"
)

func TestContributionPartialDiagnostics(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "all"}[batch], func(t *testing.T) {
			m, repo, out := workflowFixture(t)
			workflowGit(t, repo, "init", "-q")
			mustWrite(t, filepath.Join(repo, ".gitignore"), "generated.log\n")
			target := filepath.Join(repo, "skills", "group", "example")
			for _, name := range []string{"deleted-checkout", "deleted-install", "overlap", "identical", "mode"} {
				mustWrite(t, filepath.Join(target, name), "baseline secret")
			}
			if err := m.Install("group-example"); err != nil {
				t.Fatal(err)
			}
			installed := filepath.Join(m.GlobalDir, "group-example")
			mustWrite(t, filepath.Join(target, "checkout-only"), "checkout secret")
			mustWrite(t, filepath.Join(installed, "install-only"), "installation secret")
			mustWrite(t, filepath.Join(target, "overlap"), "checkout secret")
			mustWrite(t, filepath.Join(installed, "overlap"), "installation secret")
			mustWrite(t, filepath.Join(target, "identical"), "same secret")
			mustWrite(t, filepath.Join(installed, "identical"), "same secret")
			if err := os.Remove(filepath.Join(target, "deleted-checkout")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(installed, "deleted-install")); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(target, "mode"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(installed, "__pycache__"), 0755); err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(installed, "__pycache__", "cache.pyc"), "cache secret")
			checkoutCache := filepath.Join("tests", "__pycache__", "test_skill.pyc")
			if err := os.MkdirAll(filepath.Join(target, filepath.Dir(checkoutCache)), 0755); err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(target, checkoutCache), "checkout cache secret")
			mustWrite(t, filepath.Join(target, "generated.log"), "checkout log secret")
			mustWrite(t, filepath.Join(installed, "generated.log"), "installation log secret")
			mustWrite(t, filepath.Join(target, "escape\nname"), "secret")
			beforeInstalled, err := skill.Checksum(installed)
			if err != nil {
				t.Fatal(err)
			}
			_, base, err := m.readBaseline("group-example")
			if err != nil {
				t.Fatal(err)
			}
			beforeBase, err := skill.Checksum(base)
			if err != nil {
				t.Fatal(err)
			}
			confirm := func() (bool, error) { return true, nil }
			if batch {
				err = m.ContributeAll(repo, confirm)
			} else {
				err = m.Contribute("group-example", repo, confirm)
			}
			if err != ErrContributionPartial {
				t.Fatal("expected partial contribution", err)
			}
			text := out.String()
			for _, want := range []string{target, `kept "checkout-only"`, `applied "install-only"`, `conflict "overlap"`, `kept "deleted-checkout"`, `applied "deleted-install"`, `kept "mode"`, `"escape\nname"`} {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in %s", want, text)
				}
			}
			if strings.Contains(text, "secret") || strings.Contains(text, ".pyc") || strings.Contains(text, "generated.log") || strings.Contains(text, "escape\nname") {
				t.Errorf("unsafe or misleading diagnostic: %s", text)
			}
			for _, path := range []string{"deleted-checkout", "deleted-install", "__pycache__/cache.pyc"} {
				if _, err := os.Stat(filepath.Join(target, path)); !os.IsNotExist(err) {
					t.Fatal(path, err)
				}
			}
			for path, want := range map[string]string{
				"install-only": "installation secret", "overlap": "checkout secret", "identical": "same secret",
				checkoutCache: "checkout cache secret", "generated.log": "checkout log secret",
			} {
				data, err := os.ReadFile(filepath.Join(target, path))
				if err != nil || string(data) != want {
					t.Fatal(path, err)
				}
			}
			for path, before := range map[string]string{installed: beforeInstalled, base: beforeBase} {
				after, err := skill.Checksum(path)
				if err != nil || after != before {
					t.Errorf("changed %s: %v", path, err)
				}
			}
		})
	}
}

func TestContributionIdenticalChangesRemainNoop(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "all"}[batch], func(t *testing.T) {
			m, repo, _ := workflowFixture(t)
			workflowGit(t, repo, "init", "-q")
			if err := m.Install("group-example"); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(repo, "skills", "group", "example")
			installed := filepath.Join(m.GlobalDir, "group-example")
			mustWrite(t, filepath.Join(target, "same"), "same")
			mustWrite(t, filepath.Join(installed, "same"), "same")
			confirm := func() (bool, error) { t.Fatal("identical prompted"); return false, nil }
			var err error
			if batch {
				err = m.ContributeAll(repo, confirm)
			} else {
				err = m.Contribute("group-example", repo, confirm)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
