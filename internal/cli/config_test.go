package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolateEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"SKILLS_MANAGER_SOURCE", "SKILLS_MANAGER_NAMESPACE", "SKILLS_MANAGER_CACHE"} {
		t.Setenv(name, "")
	}
	t.Setenv("SKILLS_MANAGER_HOME", t.TempDir())
}

func TestInitPersistsSourceAndNamespace(t *testing.T) {
	isolateEnvironment(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(root, "repository")
	dir := filepath.Join(repository, "skills", "test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\ndescription: Test skill.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	var stdout, stderr bytes.Buffer
	// Saving a relative path must not tie later installs to the current directory.
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Error(err)
		}
	})
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if status := Run([]string{"init", "--source", "repository", "--namespace", "team", "--home", home}, &stdout, &stderr); status != 0 {
		t.Fatalf("init status %d: %s", status, &stderr)
	}
	saved, err := readConfiguration(home)
	if err != nil || saved.Source != repository || saved.Namespace != "team" {
		t.Fatalf("saved configuration: %+v, %v", saved, err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatal(err)
	}
	if status := Run([]string{"install", "test", "--home", home}, &stdout, &stderr); status != 0 {
		t.Fatalf("install status %d: %s", status, &stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "team", "test", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	// Failed initialization must preserve the previous configuration.
	if status := Run([]string{"init", "--source", home, "--home", home}, &stdout, &stderr); status == 0 {
		t.Fatal("accepted a source without a skills directory")
	}
	after, err := readConfiguration(home)
	if err != nil || after != saved {
		t.Fatalf("failed init changed configuration: %+v, %v", after, err)
	}

	t.Setenv("SKILLS_MANAGER_SOURCE", "environment-source")
	t.Setenv("SKILLS_MANAGER_NAMESPACE", "environment-team")
	opts, _, err := parseArguments([]string{"list", "--home", home}, &stderr)
	if err != nil || opts.source != "environment-source" || opts.namespace != "environment-team" {
		t.Fatalf("environment did not override config: %+v, %v", opts, err)
	}
	opts, _, err = parseArguments([]string{"list", "--home", home, "--source=flag-source", "--namespace=flag-team"}, &stderr)
	if err != nil || opts.source != "flag-source" || opts.namespace != "flag-team" {
		t.Fatalf("flags did not override environment: %+v, %v", opts, err)
	}
}

func TestUnconfiguredCommandsAndInvalidConfiguration(t *testing.T) {
	isolateEnvironment(t)
	for _, command := range []string{"help", "version", "list"} {
		var stdout, stderr bytes.Buffer
		if status := Run([]string{command}, &stdout, &stderr); status != 0 {
			t.Fatalf("%s requires a source: %s", command, &stderr)
		}
	}
	for _, command := range []string{"init", "available"} {
		var stdout, stderr bytes.Buffer
		if status := Run([]string{command}, &stdout, &stderr); status == 0 || !strings.Contains(stderr.String(), "--source") {
			t.Fatalf("%s did not explain missing source: %s", command, &stderr)
		}
	}
	for _, namespace := range []string{"../escape", "team/sub", "Team"} {
		var stdout, stderr bytes.Buffer
		if status := Run([]string{"list", "--namespace", namespace}, &stdout, &stderr); status != 2 {
			t.Fatalf("accepted unsafe namespace %q", namespace)
		}
	}
	path := configurationPath(os.Getenv("SKILLS_MANAGER_HOME"))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if status := Run([]string{"install", "test"}, &stdout, &stderr); status != 2 || !strings.Contains(stderr.String(), "parse configuration") {
		t.Fatalf("invalid configuration was ignored: %s", &stderr)
	}
}
