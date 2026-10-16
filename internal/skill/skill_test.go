package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAndChecksum(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "pull-request")
	write(t, filepath.Join(dir, "SKILL.md"), "---\nname: pull-request\ndescription: Review pull requests consistently.\n---\n\nRead [the checklist](references/checklist.md).\n")
	write(t, filepath.Join(dir, "references", "checklist.md"), "# Checklist\n")

	metadata, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "pull-request" {
		t.Fatalf("unexpected name %q", metadata.Name)
	}
	first, err := Checksum(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Checksum(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("checksum is not reproducible: %q / %q", first, second)
	}
}

func TestDiscoverUsesExpectedLocalNamesForNestedSkills(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "adwise", "agent-browser", "SKILL.md"), "---\nname: agent-browser\ndescription: Browse the web.\n---\n")
	write(t, filepath.Join(root, "code-review", "SKILL.md"), "---\nname: code-review\ndescription: Review code.\n---\n")
	write(t, filepath.Join(root, "noah", "test", "SKILL.md"), "---\nname: noah-test\ndescription: Noah's test skill.\n---\n")

	catalog, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 3 || catalog[0].ID != "agent-browser" || catalog[1].ID != "code-review" || catalog[2].ID != "noah-test" {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
}

func TestValidateAndChecksumAcceptInstalledAndSourceNames(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		path string
		name string
	}{
		{"git-pull-request", "git-pull-request"}, // Installed package uses its full catalog ID as its leaf.
		{"git/pull-request", "git-pull-request"}, // Source package uses its parent-prefixed catalog ID.
	} {
		t.Run(test.path, func(t *testing.T) {
			dir := filepath.Join(root, filepath.FromSlash(test.path))
			write(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+test.name+"\ndescription: Test skill.\n---\n")
			if _, err := Validate(dir); err != nil {
				t.Fatal(err)
			}
			if _, err := Checksum(dir); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDiscoverRejectsUnexpectedLocalNames(t *testing.T) {
	for name, test := range map[string]struct {
		skillName string
		want      string
	}{
		"wrong root name":       {"arbitrary-name", "must match directory"},
		"nested arbitrary name": {"unrelated-name", "must match directory"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "git", "pull-request")
			if name == "wrong root name" {
				dir = filepath.Join(root, "pull-request")
			}
			write(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+test.skillName+"\ndescription: Test skill.\n---\n")
			if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected name error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestDistinctNameUsesFrontmatterID(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "composer", "auth")
	write(t, filepath.Join(dir, "SKILL.md"), "---\nname: composer-auth\ndescription: Manage authentication.\n---\n")
	metadata, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "composer-auth" || metadata.ID != "composer-auth" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	catalog, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 1 || catalog[0].ID != "composer-auth" || catalog[0].Path != dir {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	if _, err := Checksum(dir); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverRejectsDuplicateCatalogNames(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"composer/auth", "composer-auth"} {
		write(t, filepath.Join(root, filepath.FromSlash(path), "SKILL.md"), "---\nname: composer-auth\ndescription: Manage authentication.\n---\n")
	}
	if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), "duplicate skill ID") {
		t.Fatalf("expected duplicate ID error, got %v", err)
	}
}

func TestIsValidID(t *testing.T) {
	for _, id := range []string{"test", "noah/test", "third-party/browser-use"} {
		if !IsValidID(id) {
			t.Errorf("expected %q to be valid", id)
		}
	}
	for _, id := range []string{"", "/test", "noah/../test", "noah//test", `noah\\test`} {
		if IsValidID(id) {
			t.Errorf("expected %q to be invalid", id)
		}
	}
}

func TestValidateAcceptsFoldedYamlDescription(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "folded-skill")
	write(t, filepath.Join(dir, "SKILL.md"), "---\nname: folded-skill\ndescription: >\n  A description spanning\n  multiple lines.\nmetadata:\n  owner: platform\n---\n")

	metadata, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Description != "A description spanning multiple lines." {
		t.Fatalf("unexpected description %q", metadata.Description)
	}
}

func TestValidateRejectsUnsafeOrMissingReferences(t *testing.T) {
	tests := map[string]string{
		"traversal": "[secret](../../secret.txt)",
		"missing":   "[missing](references/missing.md)",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "unsafe-skill")
			write(t, filepath.Join(dir, "SKILL.md"), "---\nname: unsafe-skill\ndescription: Invalid test package.\n---\n\n"+body+"\n")
			_, err := Validate(dir)
			if err == nil {
				t.Fatal("expected validation to fail")
			}
		})
	}
}

func TestValidateRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "linked-skill")
	write(t, filepath.Join(dir, "SKILL.md"), "---\nname: linked-skill\ndescription: Invalid linked package.\n---\n")
	outside := filepath.Join(root, "outside.txt")
	write(t, outside, "secret")
	if err := os.Symlink(outside, filepath.Join(dir, "outside.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := Validate(dir)
	if err == nil || !strings.Contains(err.Error(), "symlinks are not allowed") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
