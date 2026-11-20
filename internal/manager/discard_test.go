package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscardWithoutBackupInvalidPackageAndBaseline(t *testing.T) {
	m, repo, out := workflowFixture(t)
	const id = "group-example"
	if err := m.Install(id); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(m.GlobalDir, id)
	mustWrite(t, filepath.Join(target, "SKILL.md"), "invalid locally edited frontmatter\n")
	mustWrite(t, filepath.Join(target, ".hidden"), "local hidden data")
	if err := os.Chmod(filepath.Join(target, ".hidden"), 0751); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(target, "empty"), 0710); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(id); err == nil {
		t.Fatal("ordinary update bypassed protection")
	}
	mustWrite(t, filepath.Join(repo, "skills", "group", "example", "new.txt"), "new source")
	out.Reset()
	plan, err := m.PrepareDiscard([]string{id})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "-invalid locally edited") || !strings.Contains(out.String(), "+new source") {
		t.Fatal(out.String())
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(m.stateDir(id)), "backups")); !os.IsNotExist(err) {
		t.Fatalf("unexpected backups: %v", err)
	}
	out.Reset()
	if err := m.Status(id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "clean") {
		t.Fatal(out.String())
	}
}

func TestDiscardUnknownAndCorruptBaseline(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "corrupt"}[corrupt], func(t *testing.T) {
			m, _, _ := workflowFixture(t)
			const id = "group-example"
			if err := m.Install(id); err != nil {
				t.Fatal(err)
			}
			if corrupt {
				mustWrite(t, filepath.Join(m.stateDir(id), "record.json"), "broken metadata")
			} else {
				if err := os.RemoveAll(m.stateDir(id)); err != nil {
					t.Fatal(err)
				}
			}
			mustWrite(t, filepath.Join(m.GlobalDir, id, "local.txt"), "local")
			plan, err := m.PrepareDiscard([]string{id})
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Apply(); err != nil {
				t.Fatal(err)
			}
			if _, _, err := m.readBaseline(id); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDiscardRecheck(t *testing.T) {
	for _, kind := range []string{"local", "source", "baseline", "symlink", "dependencies"} {
		t.Run(kind, func(t *testing.T) {
			m, repo, _ := workflowFixture(t)
			const id = "group-example"
			if err := m.Install(id); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(m.GlobalDir, id)
			mustWrite(t, filepath.Join(target, "local.txt"), "preserve")
			plan, err := m.PrepareDiscard([]string{id})
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "local":
				mustWrite(t, filepath.Join(target, "late.txt"), "late edit")
			case "source":
				mustWrite(t, filepath.Join(repo, "skills", "group", "example", "late.txt"), "late source")
			case "baseline":
				mustWrite(t, filepath.Join(m.stateDir(id), "record.json"), "changed")
			case "symlink":
				if err := os.Symlink(repo, filepath.Join(target, "link")); err != nil {
					t.Fatal(err)
				}
			case "dependencies":
				writeSkillNamed(t, filepath.Join(repo, "skills", "dependency"), "dependency", "Dependency.", "new")
				mustWrite(t, filepath.Join(repo, "skills.json"), `{"dependencies":{"group-example":["dependency"]}}`)
			}
			if err := plan.Check(); err == nil {
				t.Fatal("check accepted changed/unsafe selection")
			}
			if err := plan.Apply(); err == nil {
				t.Fatal("applied changed/unsafe selection")
			}
			if data, err := os.ReadFile(filepath.Join(target, "local.txt")); err != nil || string(data) != "preserve" {
				t.Fatal("local files changed")
			}
		})
	}
}

func TestDiscardPreservesExistingBackups(t *testing.T) {
	m, _, _ := workflowFixture(t)
	if err := m.Install("group-example"); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(m.GlobalDir, "group-example", "local.txt")
	mustWrite(t, local, "keep")
	plan, err := m.PrepareDiscard([]string{"group-example"})
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(filepath.Dir(m.stateDir("group-example")), "backups", "existing", "local.txt")
	if err := os.MkdirAll(filepath.Dir(backup), 0700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, backup, "keep old backup")
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(backup); string(data) != "keep old backup" {
		t.Fatal("existing backup changed")
	}
}

func TestDiscardInstallsNewDependency(t *testing.T) {
	m, repo, out := workflowFixture(t)
	if err := m.Install("group-example"); err != nil {
		t.Fatal(err)
	}
	writeSkillNamed(t, filepath.Join(repo, "skills", "dependency"), "dependency", "Dependency.", "new")
	mustWrite(t, filepath.Join(repo, "skills.json"), `{"dependencies":{"group-example":["dependency"]}}`)
	plan, err := m.PrepareDiscard([]string{"group-example"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dependency: install dependency") {
		t.Fatal(out.String())
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.readBaseline("dependency"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(m.GlobalDir, "dependency", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestDiscardPreviewsDependencies(t *testing.T) {
	m, repo, out := workflowFixture(t)
	writeSkillNamed(t, filepath.Join(repo, "skills", "dependency"), "dependency", "Dependency.", "original")
	mustWrite(t, filepath.Join(repo, "skills.json"), `{"dependencies":{"group-example":["dependency"]}}`)
	if err := m.Install("group-example"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(m.GlobalDir, "dependency", "local.txt"), "dependency edit")
	if err := m.Update("group-example"); err == nil {
		t.Fatal("ordinary update discarded dependency")
	}
	out.Reset()
	plan, err := m.PrepareDiscard([]string{"group-example"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dependency: DISCARD") || !strings.Contains(out.String(), "-dependency edit") {
		t.Fatal(out.String())
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(m.GlobalDir, "dependency", "local.txt")); !os.IsNotExist(err) {
		t.Fatal("dependency not updated", err)
	}
}
