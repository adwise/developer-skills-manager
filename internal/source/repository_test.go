package source

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSameRepository(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{
			name:  "HTTPS and SSH URLs",
			left:  "ssh://git@github.com/example/team-skills.git",
			right: "https://github.com/example/team-skills.git",
			want:  true,
		},
		{
			name:  "SCP-like and HTTPS URLs",
			left:  "git@github.com:example/team-skills.git",
			right: "https://github.com/example/team-skills",
			want:  true,
		},
		{
			name:  "different repositories",
			left:  "ssh://git@github.com/example/team-skills.git",
			right: "https://github.com/example/another-repository.git",
			want:  false,
		},
		{
			name:  "different hosts",
			left:  "ssh://git@github.com/example/team-skills.git",
			right: "https://gitlab.com/example/team-skills.git",
			want:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sameRepository(test.left, test.right); got != test.want {
				t.Fatalf("sameRepository(%q, %q) = %v, want %v", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestRemoteRepositoryCloneAndRefresh(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	if err := os.MkdirAll(filepath.Join(origin, "skills", "pull-request"), 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, origin, "init")
	git(t, origin, "config", "user.email", "skills-test@example.com")
	git(t, origin, "config", "user.name", "Skills Test")
	writeRemoteSkill(t, origin, "First version.")
	git(t, origin, "add", ".")
	git(t, origin, "commit", "-m", "initial skill")

	repository := Repository{
		Source:   "file://" + filepath.ToSlash(origin),
		CacheDir: filepath.Join(root, "cache"),
	}
	catalog, _, err := repository.Catalog(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 1 || catalog[0].Description != "First version." {
		t.Fatalf("unexpected initial catalog: %#v", catalog)
	}

	writeRemoteSkill(t, origin, "Second version.")
	git(t, origin, "add", ".")
	git(t, origin, "commit", "-m", "update skill")
	catalog, _, err = repository.Catalog(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 1 || catalog[0].Description != "Second version." {
		t.Fatalf("repository cache was not refreshed: %#v", catalog)
	}
}

func TestRegistrySupportsExternalSkillsAndSets(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, filepath.Join(root, "skills", "noah", "test", "SKILL.md"), "test", "A personal skill.")
	writeSkillFile(t, filepath.Join(root, "third_party", "agent-browser", "skills", "agent-browser", "SKILL.md"), "agent-browser", "Browser automation.")
	configuration := `{
  "externalSkills": [
    {"id": "vercel/agent-browser", "path": "third_party/agent-browser/skills/agent-browser"}
  ],
  "skillSets": [
    {"name": "web", "description": "Web workflow.", "skills": ["noah/test", "vercel/agent-browser"]}
  ]
}`
	if err := os.WriteFile(filepath.Join(root, "skills.json"), []byte(configuration), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, _, err := (Repository{Source: root}).Registry(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Skills) != 2 || catalog.Skills[0].ID != "noah/test" || catalog.Skills[1].ID != "vercel/agent-browser" {
		t.Fatalf("unexpected skills: %#v", catalog.Skills)
	}
	if len(catalog.Sets) != 1 || catalog.Sets[0].Name != "web" {
		t.Fatalf("unexpected sets: %#v", catalog.Sets)
	}
}

func TestRegistryFetchesOnlyExternalSkillPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	root := t.TempDir()
	central := filepath.Join(root, "central")
	if err := os.MkdirAll(filepath.Join(central, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	origin := filepath.Join(root, "external-origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, origin, "init")
	git(t, origin, "config", "user.email", "skills-test@example.com")
	git(t, origin, "config", "user.name", "Skills Test")
	writeSkillFile(t, filepath.Join(origin, "skills", "agent-browser", "SKILL.md"), "agent-browser", "Browser automation.")
	if err := os.WriteFile(filepath.Join(origin, "unrelated.txt"), []byte("must not be checked out"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, origin, "add", ".")
	git(t, origin, "commit", "-m", "external skill")
	ref, err := gitOutput("-C", origin, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	configuration := fmt.Sprintf(`{
  "externalSkills": [
    {
      "id": "vercel/agent-browser",
      "repository": %q,
      "ref": %q,
      "path": "skills/agent-browser"
    }
  ]
}`, "file://"+filepath.ToSlash(origin), strings.TrimSpace(ref))
	if err := os.WriteFile(filepath.Join(central, "skills.json"), []byte(configuration), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := filepath.Join(root, "cache", "repository")
	repository := Repository{Source: central, CacheDir: cache}
	catalog, _, err := repository.Registry(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Skills) != 1 || catalog.Skills[0].ID != "vercel/agent-browser" {
		t.Fatalf("unexpected catalog: %#v", catalog.Skills)
	}
	if _, err := os.Stat(filepath.Join(catalog.Skills[0].Path, "SKILL.md")); err != nil {
		t.Fatalf("external skill was not checked out: %v", err)
	}
	unrelated, err := filepath.Glob(filepath.Join(cache+"-external", "*", "unrelated.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(unrelated) != 0 {
		t.Fatalf("unrelated external files were checked out: %v", unrelated)
	}

	// A pinned skill remains usable from its cache without contacting upstream.
	if err := os.RemoveAll(origin); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Registry(false); err != nil {
		t.Fatalf("reuse external cache: %v", err)
	}
}

func TestRegistryRejectsExternalSkillThroughSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	writeSkillFile(t, filepath.Join(outside, "agent-browser", "SKILL.md"), "agent-browser", "Browser automation.")
	if err := os.Symlink(outside, filepath.Join(root, "vendor")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	configuration := `{"externalSkills":[{"id":"vercel/agent-browser","path":"vendor/agent-browser"}]}`
	if err := os.WriteFile(filepath.Join(root, "skills.json"), []byte(configuration), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := (Repository{Source: root}).Registry(false); err == nil {
		t.Fatal("expected external path through symlink to be rejected")
	}
}

func writeSkillFile(t *testing.T, path, name, description string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	contents := "---\nname: " + name + "\ndescription: " + description + "\n---\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRemoteSkill(t *testing.T, repository, description string) {
	t.Helper()
	path := filepath.Join(repository, "skills", "pull-request", "SKILL.md")
	writeSkillFile(t, path, "pull-request", description)
}

func git(t *testing.T, repository string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repository}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
