package manager

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/source"
)

func TestInstallAndUpdateDependencies(t *testing.T) {
	for _, selector := range []string{"skill-a", "@team"} {
		t.Run(selector, func(t *testing.T) {
			root := t.TempDir()
			repository := filepath.Join(root, "repository")
			for _, id := range []string{"skill-a", "tools/mcporter", "base", "extra"} {
				writeSkill(t, filepath.Join(repository, "skills", filepath.FromSlash(id)), "Workflow.", "original\n")
			}
			manifest := filepath.Join(repository, "skills.json")
			configuration := `{"dependencies":{"skill-a":["tools/mcporter","base"],"tools/mcporter":["base"]},"skillSets":[{"name":"team","skills":["skill-a","base"]}]}`
			writeManifest := func(contents string) {
				t.Helper()
				if err := os.WriteFile(manifest, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			writeManifest(configuration)
			var output bytes.Buffer
			mgr := Manager{Repository: source.Repository{Source: repository}, GlobalDir: filepath.Join(root, "global"), Out: &output}
			if err := mgr.InstallMany([]string{selector, "base"}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"base", "tools/mcporter", "skill-a"} {
				if strings.Count(output.String(), "Installed "+id+" in") != 1 {
					t.Fatalf("expected one install of %s: %s", id, output.String())
				}
			}
			// A manifest-only change should show an update in the installer.
			writeManifest(strings.Replace(configuration, `"tools/mcporter":["base"]`, `"tools/mcporter":["base","extra"]`, 1))
			_, _, updates, err := mgr.InstallOptionsWithUpdates()
			if err != nil || !updates["skill-a"] {
				t.Fatalf("missing dependency not actionable: %v, %v", updates, err)
			}
			writeSkill(t, filepath.Join(repository, "skills", "tools", "mcporter"), "Workflow.", "updated\n")
			if err := mgr.Update("skill-a"); err != nil {
				t.Fatal(err)
			}
			contents, err := os.ReadFile(filepath.Join(mgr.GlobalDir, "tools", "mcporter", "SKILL.md"))
			if err != nil || !strings.Contains(string(contents), "updated") {
				t.Fatalf("dependency not updated: %s, %v", contents, err)
			}
			if _, err := os.Stat(filepath.Join(mgr.GlobalDir, "extra", "SKILL.md")); err != nil {
				t.Fatalf("new dependency not installed: %v", err)
			}
			if err := mgr.Uninstall("skill-a"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(mgr.GlobalDir, "base", "SKILL.md")); err != nil {
				t.Fatalf("uninstall removed shared dependency: %v", err)
			}
		})
	}
}

func TestInstallChecksDependencyConflictsBeforeCopying(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	global := filepath.Join(root, "global")
	writeSkill(t, filepath.Join(repository, "skills", "skill-a"), "Workflow.", "original\n")
	writeSkill(t, filepath.Join(repository, "skills", "mcporter"), "Workflow.", "original\n")
	writeSkill(t, filepath.Join(global, "mcporter"), "Workflow.", "locally modified\n")
	if err := os.WriteFile(filepath.Join(repository, "skills.json"), []byte(`{"dependencies":{"skill-a":["mcporter"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := Manager{Repository: source.Repository{Source: repository}, GlobalDir: global, Out: &bytes.Buffer{}}
	if err := mgr.Install("skill-a"); err == nil {
		t.Fatal("expected dependency conflict")
	}
	if _, err := os.Stat(filepath.Join(global, "skill-a")); !os.IsNotExist(err) {
		t.Fatalf("dependent copied before checking conflict: %v", err)
	}
}
