package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkflowConfigAndContributeCLI(t *testing.T) {
	isolateEnvironment(t)
	home := os.Getenv("SKILLS_MANAGER_HOME")
	repo := filepath.Join(t.TempDir(), "repo")
	skillDir := filepath.Join(repo, "skills", "example")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := "---\nname: example\ndescription: Example.\n---\noriginal\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", output, err)
	}
	var out, stderr bytes.Buffer
	run := func(input string, args ...string) int {
		t.Helper()
		out.Reset()
		stderr.Reset()
		return RunWithIO(args, strings.NewReader(input), &out, &stderr)
	}
	if run("", "config", "set", "repo", repo) != 0 {
		t.Fatal(stderr.String())
	}
	if run("", "init", "--source", repo, "--namespace", "team") != 0 {
		t.Fatal(stderr.String())
	}
	saved, err := readConfiguration(home)
	if err != nil || saved.Repo != repo || saved.Namespace != "team" || saved.Source != repo {
		t.Fatalf("config: %+v, %v", saved, err)
	}
	data, err := os.ReadFile(configurationPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"repo":`)) {
		t.Fatal("missing exact repo field")
	}
	if run("", "install", "example") != 0 {
		t.Fatal(stderr.String())
	}
	installed := filepath.Join(home, ".agents", "skills", "team", "example", "SKILL.md")
	if err := os.WriteFile(installed, []byte(original+"local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if run("", "status", "example") != 0 || !strings.Contains(out.String(), "example: local") {
		t.Fatalf("%s %s", &out, &stderr)
	}
	if run("", "diff", "--local", "example") != 0 || !strings.Contains(out.String(), "+local edit") {
		t.Fatalf("%s %s", &out, &stderr)
	}
	// EOF and an explicit no both cancel rather than accepting implicitly.
	for _, input := range []string{"", "no\n"} {
		if run(input, "contribute", "example") != 0 {
			t.Fatal(stderr.String())
		}
		data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
		if err != nil || string(data) != original {
			t.Fatal("cancel modified checkout")
		}
	}
	if run("", "contribute", "--all", "example") == 0 {
		t.Fatal("accepted --all with a name")
	}
	if run("", "contribute", "--all", "--repo", t.TempDir(), "--yes") == 0 {
		t.Fatal("batch ignored invalid override")
	}
	if run("no\n", "contribute", "--all") != 0 || !strings.Contains(out.String(), "Cancelled.") {
		t.Fatalf("%s %s", &out, &stderr)
	}
	// The explicit override must be used, not silently ignored in favor of config.
	if run("", "contribute", "example", "--repo", t.TempDir(), "--yes") == 0 {
		t.Fatal("ignored invalid override")
	}
	if run("", "contribute", "--all", "--repo="+repo, "--yes") != 0 {
		t.Fatal(stderr.String())
	}
	data, err = os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil || !bytes.Contains(data, []byte("local edit")) {
		t.Fatalf("contribution: %s %v", data, err)
	}
	after, err := readConfiguration(home)
	if err != nil || !reflect.DeepEqual(after, saved) {
		t.Fatal("override altered saved config")
	}
}
