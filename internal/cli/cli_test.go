package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsGlobalOptionsAfterCommand(t *testing.T) {
	isolateEnvironment(t)
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	dir := filepath.Join(repository, "skills", "pull-request")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := "---\nname: pull-request\ndescription: Pull request workflow.\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	status := Run([]string{"available", "pull", "--source", repository, "--home", filepath.Join(root, "home")}, &stdout, &stderr)
	if status != 0 {
		t.Fatalf("status %d; stderr: %s", status, stderr.String())
	}
	if !strings.Contains(stdout.String(), "pull-request") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunInstallsMultipleSkillsIncludingNamespace(t *testing.T) {
	isolateEnvironment(t)
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	for path, name := range map[string]string{
		filepath.Join(repository, "skills", "code-review"):  "code-review",
		filepath.Join(repository, "skills", "noah", "test"): "test",
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		contents := "---\nname: " + name + "\ndescription: Test skill.\n---\n"
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	home := filepath.Join(root, "home")
	var stdout, stderr bytes.Buffer
	status := Run([]string{"install", "code-review", "noah/test", "--source", repository, "--home", home}, &stdout, &stderr)
	if status != 0 {
		t.Fatalf("status %d; stderr: %s", status, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "skills-manager", "noah", "test", "SKILL.md")); err != nil {
		t.Fatalf("namespaced skill was not installed in the managed namespace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "code-review", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("skill was unexpectedly installed outside the managed namespace: %v", err)
	}
}
