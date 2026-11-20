package manager

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/source"
)

func workflowFixture(t *testing.T) (Manager, string, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := filepath.Join(root, "repo")
	writeSkillNamed(t, filepath.Join(repo, "skills", "group", "example"), "group-example", "Example.", "original\n")
	out := &bytes.Buffer{}
	return Manager{Repository: source.Repository{Source: repo}, GlobalDir: filepath.Join(root, "installed"), Out: out}, repo, out
}

func workflowGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git: %s: %v", output, err)
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatusAlignsNames(t *testing.T) {
	m, repo, out := workflowFixture(t)
	writeSkillNamed(t, filepath.Join(repo, "skills", "short"), "short", "Short.", "original\n")
	if err := m.Install("group-example"); err != nil {
		t.Fatal(err)
	}
	if err := m.Install("short"); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := m.Status(""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "group-example: clean\n") || !strings.Contains(out.String(), "short:         clean\n") {
		t.Fatal(out.String())
	}
}

func TestWorkflowStatusProtectionAndLocalDiff(t *testing.T) {
	m, repo, out := workflowFixture(t)
	id := "group-example"
	if err := m.Install(id); err != nil {
		t.Fatal(err)
	}
	status := func(want string) {
		t.Helper()
		out.Reset()
		if err := m.Status(id); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), id+": "+want+"\n") {
			t.Fatalf("want %s, got %s", want, out)
		}
	}
	status("clean")
	installed := filepath.Join(m.GlobalDir, id)
	original := filepath.Join(repo, "skills", "group", "example")
	mustWrite(t, filepath.Join(installed, "added.txt"), "local addition\n")
	status("local")
	out.Reset()
	if err := m.DiffLocal(id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "+local addition") {
		t.Fatal(out.String())
	}
	if err := m.Update(id); err == nil {
		t.Fatal("update discarded local changes")
	}
	if err := m.Uninstall(id); err == nil {
		t.Fatal("uninstall discarded local changes")
	}
	mustWrite(t, filepath.Join(original, "upstream.txt"), "upstream addition\n")
	status("both")
	if err := os.Remove(filepath.Join(installed, "added.txt")); err != nil {
		t.Fatal(err)
	}
	status("upstream")
	if err := m.Update(id); err != nil {
		t.Fatal(err)
	}
	status("clean")
	if err := m.Uninstall(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.stateDir(id)); !os.IsNotExist(err) {
		t.Fatalf("state not removed: %v", err)
	}
}

func TestWorkflowLegacyUnknown(t *testing.T) {
	m, repo, out := workflowFixture(t)
	id := "group-example"
	target := filepath.Join(m.GlobalDir, id)
	writeSkillNamed(t, target, id, "Example.", "legacy edit\n")
	if err := m.Status(id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unknown (differs from upstream)") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := m.DiffLocal(id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unknown baseline") {
		t.Fatal(out.String())
	}
	if err := m.Update(id); err == nil {
		t.Fatal("updated unknown-different install")
	}
	if err := m.Uninstall(id); err == nil {
		t.Fatal("removed unknown-different install")
	}
	if err := m.Contribute(id, repo, func() (bool, error) { return true, nil }); err == nil {
		t.Fatal("contributed without provenance")
	}
	writeSkillNamed(t, target, id, "Example.", "original\n")
	out.Reset()
	if err := m.Status(id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unknown (matches upstream)") {
		t.Fatal(out.String())
	}
	if err := m.Update(id); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := m.Status(id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), ": clean") {
		t.Fatal(out.String())
	}
}

func TestWorkflowDependencyAndBatchProtection(t *testing.T) {
	m, repo, _ := workflowFixture(t)
	writeSkill(t, filepath.Join(repo, "skills", "dependent"), "Dependent.", "old\n")
	mustWrite(t, filepath.Join(repo, "skills.json"), `{"dependencies":{"dependent":["group-example"]}}`)
	if err := m.Install("dependent"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(m.GlobalDir, "group-example", "local.txt"), "edit")
	writeSkill(t, filepath.Join(repo, "skills", "dependent"), "Dependent.", "new\n")
	if err := m.UpdateMany([]string{"dependent"}); err == nil {
		t.Fatal("dependency edits overwritten")
	}
	body, err := os.ReadFile(filepath.Join(m.GlobalDir, "dependent", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "old") {
		t.Fatal("partial batch update")
	}
	if err := m.UninstallMany([]string{"dependent", "group-example"}); err == nil {
		t.Fatal("dependency edits removed")
	}
	if _, err := os.Stat(filepath.Join(m.GlobalDir, "dependent")); err != nil {
		t.Fatal("partial batch removal", err)
	}
}

func TestWorkflowContributeNestedAddsDeletesAndConfirmation(t *testing.T) {
	m, repo, out := workflowFixture(t)
	workflowGit(t, repo, "init", "-q")
	id := "group-example"
	original := filepath.Join(repo, "skills", "group", "example")
	mustWrite(t, filepath.Join(original, "delete.txt"), "remove me\n")
	if err := m.Install(id); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(m.GlobalDir, id)
	if err := os.Remove(filepath.Join(installed, "delete.txt")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(installed, "new.txt"), "new file\n")
	calls := 0
	if err := m.Contribute(id, repo, func() (bool, error) { calls++; return false, nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(out.String(), "File deleted") || !strings.Contains(out.String(), "File added") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(filepath.Join(original, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("cancel changed checkout")
	}
	if err := m.Contribute(id, repo, func() (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(original, "new.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(original, "delete.txt")); !os.IsNotExist(err) {
		t.Fatal("deleted file retained")
	}
	if err := m.Contribute(id, repo, func() (bool, error) { t.Fatal("identical target prompted"); return false, nil }); err != nil {
		t.Fatal(err)
	}
	// Contribution preserves local history; explicit identical update accepts it.
	out.Reset()
	if err := m.Status(id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), ": both") {
		t.Fatal(out.String())
	}
	if err := m.Update(id); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(id); err != nil {
		t.Fatal(err)
	}
}

func TestContributeAllBatch(t *testing.T) {
	m, repo, out := workflowFixture(t)
	workflowGit(t, repo, "init", "-q")
	writeSkill(t, filepath.Join(repo, "skills", "second"), "Second.", "original\n")
	writeSkill(t, filepath.Join(repo, "skills", "clean"), "Clean.", "original\n")
	mustWrite(t, filepath.Join(repo, "skills", "group", "example", "deleted.txt"), "delete\n")
	for _, name := range []string{"group-example", "second", "clean"} {
		if err := m.Install(name); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill(t, filepath.Join(m.GlobalDir, "unknown"), "Unknown.", "legacy\n")
	out.Reset()
	if err := m.ContributeAll(repo, func() (bool, error) { t.Fatal("no changes prompted"); return false, nil }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unknown baseline") {
		t.Fatal(out.String())
	}
	mustWrite(t, filepath.Join(m.GlobalDir, "group-example", "added.txt"), "added\n")
	if err := os.Remove(filepath.Join(m.GlobalDir, "group-example", "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(m.GlobalDir, "second", "added.txt"), "second\n")
	calls := 0
	out.Reset()
	if err := m.ContributeAll(repo, func() (bool, error) { calls++; return false, nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(out.String(), "File deleted") || !strings.Contains(out.String(), "File added") || strings.Contains(out.String(), "Contribute clean ") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(filepath.Join(repo, "skills", "second", "added.txt")); !os.IsNotExist(err) {
		t.Fatal("cancellation wrote changes")
	}
	mustWrite(t, filepath.Join(repo, "skills", "second", "conflict.txt"), "upstream\n")
	if err := m.ContributeAll(repo, func() (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "skills", "group", "example", "added.txt")); !os.IsNotExist(err) {
		t.Fatal("partial batch")
	}
	if err := os.Remove(filepath.Join(repo, "skills", "second", "conflict.txt")); err != nil {
		t.Fatal(err)
	}
	if err := m.ContributeAll(repo, func() (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "skills", "group", "example", "deleted.txt")); !os.IsNotExist(err) {
		t.Fatal("deletion not copied")
	}
	if _, err := os.Stat(filepath.Join(repo, "skills", "second", "added.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowContributeRejectsUnsafeTargets(t *testing.T) {
	for _, mode := range []string{"conflict", "unrelated", "symlink", "invalid", "changed-during-confirm", "target-changed-during-confirm"} {
		t.Run(mode, func(t *testing.T) {
			m, repo, _ := workflowFixture(t)
			workflowGit(t, repo, "init", "-q")
			id := "group-example"
			if err := m.Install(id); err != nil {
				t.Fatal(err)
			}
			installed := filepath.Join(m.GlobalDir, id)
			original := filepath.Join(repo, "skills", "group", "example")
			mustWrite(t, filepath.Join(installed, "edit.txt"), "edit")
			targetRepo := repo
			switch mode {
			case "conflict":
				mustWrite(t, filepath.Join(original, "edit.txt"), "upstream")
			case "unrelated":
				targetRepo = t.TempDir()
				workflowGit(t, targetRepo, "init", "-q")
				workflowGit(t, targetRepo, "remote", "add", "origin", "https://example.org/wrong.git")
			case "symlink":
				destination := filepath.Join(repo, "moved")
				if err := os.Rename(original, destination); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(destination, original); err != nil {
					t.Fatal(err)
				}
			case "invalid":
				mustWrite(t, filepath.Join(installed, "SKILL.md"), "invalid")
			}
			err := m.Contribute(id, targetRepo, func() (bool, error) {
				if mode == "changed-during-confirm" {
					mustWrite(t, filepath.Join(installed, "later.txt"), "later")
				}
				if mode == "target-changed-during-confirm" {
					mustWrite(t, filepath.Join(original, "later.txt"), "later")
				}
				return true, nil
			})
			if err == nil {
				t.Fatal("unsafe contribution accepted")
			}
			if mode == "conflict" {
				data, err := os.ReadFile(filepath.Join(original, "edit.txt"))
				if err != nil || string(data) != "upstream" {
					t.Fatal("changed conflict")
				}
				return
			}
			if _, err := os.Stat(filepath.Join(original, "edit.txt")); !os.IsNotExist(err) {
				t.Fatalf("unsafe copy occurred: %v", err)
			}
		})
	}
}

func TestWorkflowExternalContributionRejected(t *testing.T) {
	m, repo, _ := workflowFixture(t)
	writeSkill(t, filepath.Join(repo, "external", "example"), "External.", "external\n")
	mustWrite(t, filepath.Join(repo, "skills.json"), `{"externalSkills":[{"id":"vendor/example","path":"external/example"}]}`)
	if err := m.Install("vendor/example"); err != nil {
		t.Fatal(err)
	}
	err := m.Contribute("vendor/example", repo, func() (bool, error) { return true, nil })
	if err == nil || !strings.Contains(err.Error(), "external packages") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkflowBaselineIntegrity(t *testing.T) {
	m, _, _ := workflowFixture(t)
	if err := m.Install("group-example"); err != nil {
		t.Fatal(err)
	}
	_, base, err := m.readBaseline("group-example")
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(base, "tamper.txt"), "tampered")
	if err := m.Uninstall("group-example"); err == nil {
		t.Fatal("corrupt baseline accepted")
	}
}

func TestInstallerAndDoctorUseBaselineStatus(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		local, upstream, legacy bool
		want                    string
	}{
		{"clean", false, false, false, "clean"},
		{"local", true, false, false, "local"},
		{"upstream", false, true, false, "upstream"},
		{"both", true, true, false, "both"},
		{"unknown equal", false, false, true, "unknown (matches upstream)"},
		{"unknown different", true, false, true, "unknown (differs from upstream)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, repo, out := workflowFixture(t)
			id := "group-example"
			if err := m.Install(id); err != nil {
				t.Fatal(err)
			}
			if tc.legacy {
				if err := os.RemoveAll(m.stateDir(id)); err != nil {
					t.Fatal(err)
				}
			}
			if tc.local {
				mustWrite(t, filepath.Join(m.GlobalDir, id, "local.txt"), "local")
			}
			if tc.upstream {
				mustWrite(t, filepath.Join(repo, "skills", "group", "example", "upstream.txt"), "upstream")
			}
			_, _, updates, states, err := m.InstallOptionsWithStatus()
			if err != nil {
				t.Fatal(err)
			}
			if states[id] != tc.want {
				t.Fatalf("status = %q, want %q", states[id], tc.want)
			}
			if updates[id] != (tc.local || tc.upstream) {
				t.Fatalf("actionability changed: %v", updates)
			}
			out.Reset()
			if err := m.Doctor(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), id+": "+tc.want) {
				t.Fatal(out.String())
			}
		})
	}
}
