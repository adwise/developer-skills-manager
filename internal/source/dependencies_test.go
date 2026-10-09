package source

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRegistryDependencies(t *testing.T) {
	for _, test := range []struct {
		name, manifest, wantError string
	}{
		{"valid", `{"dependencies":{"skill-a":["tools/mcporter"],"tools/mcporter":["base"]}}`, ""},
		{"missing owner", `{"dependencies":{"missing":["base"]}}`, "unavailable skill"},
		{"missing dependency", `{"dependencies":{"skill-a":["missing"]}}`, "not available"},
		{"unsafe ID", `{"dependencies":{"skill-a":["../base"]}}`, "not available"},
		{"self cycle", `{"dependencies":{"skill-a":["skill-a"]}}`, "cycle"},
		{"cycle", `{"dependencies":{"skill-a":["base"],"base":["skill-a"]}}`, "cycle"},
		{"duplicate", `{"dependencies":{"skill-a":["base","base"]}}`, "duplicate dependency"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeSkillFile(t, filepath.Join(root, "skills", "skill-a", "SKILL.md"), "skill-a", "Workflow.")
			writeSkillFile(t, filepath.Join(root, "skills", "base", "SKILL.md"), "base", "Base.")
			writeSkillFile(t, filepath.Join(root, "vendor", "mcporter", "SKILL.md"), "mcporter", "MCP tools.")
			manifest := strings.TrimSuffix(test.manifest, "}") + `,"externalSkills":[{"id":"tools/mcporter","path":"vendor/mcporter"}]}`
			if err := os.WriteFile(filepath.Join(root, "skills.json"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			catalog, _, err := (Repository{Source: root}).Registry(false)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("got %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ids, err := catalog.ResolveDependencies([]string{"skill-a", "base", "skill-a"})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ids, []string{"base", "tools/mcporter", "skill-a"}) {
				t.Fatalf("unexpected order or duplicates: %v", ids)
			}
		})
	}
}
