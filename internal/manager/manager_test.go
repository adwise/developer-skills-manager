package manager

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

func TestInstallDiffUpdateListAndUninstall(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	home := filepath.Join(root, "home")
	local := filepath.Join(root, "project", ".agents", "skills")
	writeSkill(t, filepath.Join(repository, "skills", "pull-request"), "Pull request workflow.", "# Pull requests\n\nReview CI.\n")
	writeSkill(t, filepath.Join(repository, "skills", "code-review"), "Code review workflow.", "# Code review\n\nCheck tests.\n")
	writeSkill(t, filepath.Join(local, "project-conventions"), "Repository conventions.", "# Project conventions\n")

	var output bytes.Buffer
	mgr := Manager{
		Repository: source.Repository{Source: repository, CacheDir: filepath.Join(root, "cache")},
		GlobalDir:  filepath.Join(home, ".agents", "skills"),
		LocalDir:   local,
		Out:        &output,
	}

	if err := mgr.Available("pull"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "pull-request") || strings.Contains(output.String(), "code-review") {
		t.Fatalf("unexpected available output: %s", output.String())
	}
	output.Reset()

	if err := mgr.Install("pull-request"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Install("code-review"); err != nil {
		t.Fatal(err)
	}
	codeHash, err := skill.Checksum(filepath.Join(mgr.GlobalDir, "code-review"))
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := mgr.List(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Global:", "pull-request", "code-review", "Local:", "project-conventions"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("list output is missing %q: %s", expected, output.String())
		}
	}

	writeSkill(t, filepath.Join(repository, "skills", "pull-request"), "Pull request workflow.", "# Pull requests\n\nReview CI first.\n")
	output.Reset()
	if err := mgr.Diff("pull-request"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "+Review CI first.") || !strings.Contains(output.String(), "-Review CI.") {
		t.Fatalf("unexpected diff output: %s", output.String())
	}

	output.Reset()
	if err := mgr.Update("pull-request"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Updated pull-request") {
		t.Fatalf("unexpected update output: %s", output.String())
	}
	contents, err := os.ReadFile(filepath.Join(mgr.GlobalDir, "pull-request", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "Review CI first.") {
		t.Fatalf("installed package was not updated: %s", contents)
	}
	unchangedHash, err := skill.Checksum(filepath.Join(mgr.GlobalDir, "code-review"))
	if err != nil {
		t.Fatal(err)
	}
	if unchangedHash != codeHash {
		t.Fatal("updating one skill modified an unrelated skill")
	}

	if err := mgr.Uninstall("pull-request"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mgr.GlobalDir, "pull-request")); !os.IsNotExist(err) {
		t.Fatalf("skill still exists after uninstall: %v", err)
	}
}

func TestInstallNamespacedSkillAndSkillSet(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	writeSkill(t, filepath.Join(repository, "skills", "noah", "test"), "Personal test workflow.", "# Test\n")
	writeSkill(t, filepath.Join(repository, "skills", "code-review"), "Code review workflow.", "# Review\n")
	configuration := `{
  "skillSets": [
    {"name": "team", "description": "Team defaults.", "skills": ["code-review", "noah/test"]}
  ]
}`
	if err := os.WriteFile(filepath.Join(repository, "skills.json"), []byte(configuration), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	mgr := Manager{
		Repository: source.Repository{Source: repository},
		GlobalDir:  filepath.Join(root, "home", ".agents", "skills"),
		LocalDir:   filepath.Join(root, "local"),
		Out:        &output,
	}
	if err := mgr.InstallMany([]string{"@team"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(mgr.GlobalDir, "code-review", "SKILL.md"),
		filepath.Join(mgr.GlobalDir, "noah", "test", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected installed skill at %s: %v", path, err)
		}
	}
	output.Reset()
	if err := mgr.List(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "noah/test") {
		t.Fatalf("namespaced skill missing from list: %s", output.String())
	}
	if err := mgr.Uninstall("noah"); err == nil {
		t.Fatal("expected namespace-only directory not to be uninstallable as a skill")
	}
	if _, err := os.Stat(filepath.Join(mgr.GlobalDir, "noah", "test", "SKILL.md")); err != nil {
		t.Fatalf("uninstalling a namespace removed its child skill: %v", err)
	}

	writeSkill(t, filepath.Join(repository, "skills", "noah", "test"), "Personal test workflow.", "# Updated test\n")
	output.Reset()
	if err := mgr.Update("noah/test"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(mgr.GlobalDir, "noah", "test", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "Updated test") {
		t.Fatalf("namespaced skill was not updated: %s", contents)
	}

	if err := mgr.Uninstall("noah/test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mgr.GlobalDir, "noah")); !os.IsNotExist(err) {
		t.Fatalf("empty namespace was not removed: %v", err)
	}
}

func TestUninstallManyValidatesAllSkillsBeforeRemoving(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	global := filepath.Join(root, "home", ".agents", "skills")
	writeSkill(t, filepath.Join(repository, "skills", "code-review"), "Code review workflow.", "# Review\n")
	mgr := Manager{Repository: source.Repository{Source: repository}, GlobalDir: global, Out: &bytes.Buffer{}}
	if err := mgr.Install("code-review"); err != nil {
		t.Fatal(err)
	}

	if err := mgr.UninstallMany([]string{"code-review", "missing"}); err == nil {
		t.Fatal("expected uninstall to reject a missing skill")
	}
	if _, err := os.Stat(filepath.Join(global, "code-review", "SKILL.md")); err != nil {
		t.Fatalf("valid skill was removed before the full selection was checked: %v", err)
	}
	if err := mgr.UninstallMany([]string{"code-review"}); err != nil {
		t.Fatal(err)
	}
}

func TestInstallOptionsIncludesInstalledSkillMissingFromSource(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	sourceDir := filepath.Join(repository, "skills", "code-review")
	writeSkill(t, sourceDir, "Code review workflow.", "# Review\n")
	mgr := Manager{
		Repository: source.Repository{Source: repository},
		GlobalDir:  filepath.Join(root, "home", ".agents", "skills"),
		Out:        &bytes.Buffer{},
	}
	if err := mgr.Install("code-review"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(sourceDir); err != nil {
		t.Fatal(err)
	}

	catalog, installed, _, err := mgr.InstallOptionsWithUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if !installed["code-review"] {
		t.Fatal("installed skill was not reported")
	}
	found := false
	for _, item := range catalog.Skills {
		if item.ID == "code-review" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("installed skill missing from source was not included in TUI options")
	}
}

func TestInstallRejectsTraversalName(t *testing.T) {
	mgr := Manager{GlobalDir: filepath.Join(t.TempDir(), ".agents", "skills"), Out: &bytes.Buffer{}}
	if err := mgr.Install("../../escape"); err == nil {
		t.Fatal("expected traversal name to be rejected")
	}
}

func TestInstallCreatesTargetOfDanglingAgentsSymlink(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	home := filepath.Join(root, "home")
	linkedAgents := filepath.Join(root, "config", "agents")
	writeSkill(t, filepath.Join(repository, "skills", "code-review"), "Code review workflow.", "# Code review\n")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkedAgents, filepath.Join(home, ".agents")); err != nil {
		t.Fatal(err)
	}

	mgr := Manager{
		Repository: source.Repository{Source: repository},
		GlobalDir:  filepath.Join(home, ".agents", "skills"),
		Out:        &bytes.Buffer{},
	}
	if err := mgr.Install("code-review"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(linkedAgents, "skills", "code-review", "SKILL.md")); err != nil {
		t.Fatalf("skill was not installed through the symlink: %v", err)
	}
}

func TestInstallOptionsReportsAvailableUpdates(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	global := filepath.Join(root, "home", ".agents", "skills")
	writeSkill(t, filepath.Join(repository, "skills", "pull-request"), "Pull request workflow.", "old\n")
	mgr := Manager{Repository: source.Repository{Source: repository}, GlobalDir: global, Out: &bytes.Buffer{}}
	if err := mgr.Install("pull-request"); err != nil {
		t.Fatal(err)
	}

	_, installed, updates, err := mgr.InstallOptionsWithUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if !installed["pull-request"] || updates["pull-request"] {
		t.Fatalf("unexpected initial state: installed=%v updates=%v", installed, updates)
	}

	writeSkill(t, filepath.Join(repository, "skills", "pull-request"), "Pull request workflow.", "new\n")
	_, installed, updates, err = mgr.InstallOptionsWithUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if !installed["pull-request"] || !updates["pull-request"] {
		t.Fatalf("update was not reported: installed=%v updates=%v", installed, updates)
	}

	if err := mgr.UpdateMany([]string{"pull-request"}); err != nil {
		t.Fatal(err)
	}
	_, _, updates, err = mgr.InstallOptionsWithUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if updates["pull-request"] {
		t.Fatalf("updated skill is still reported as outdated: %v", updates)
	}
}

func TestDoctorReportsAvailableUpdateWithoutChangingFiles(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	global := filepath.Join(root, "home", ".agents", "skills")
	writeSkill(t, filepath.Join(repository, "skills", "pull-request"), "Pull request workflow.", "old\n")
	var output bytes.Buffer
	mgr := Manager{Repository: source.Repository{Source: repository}, GlobalDir: global, LocalDir: filepath.Join(root, "local"), Out: &output}
	if err := mgr.Install("pull-request"); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(repository, "skills", "pull-request"), "Pull request workflow.", "new\n")
	output.Reset()
	if err := mgr.Doctor(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "has an update available") {
		t.Fatalf("doctor did not report update: %s", output.String())
	}
	contents, err := os.ReadFile(filepath.Join(global, "pull-request", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "new") {
		t.Fatal("doctor unexpectedly modified the installed skill")
	}
}

func writeSkill(t *testing.T, dir, description, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(dir)
	contents := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
