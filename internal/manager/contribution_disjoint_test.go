package manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContributionAllDisjointKeepsBoth(t *testing.T) {
	m, repo, _ := workflowFixture(t)
	workflowGit(t, repo, "init", "-q")
	writeSkill(t, filepath.Join(repo, "skills", "aaa"), "First.", "original\n")
	for _, name := range []string{"aaa", "group-example"} {
		if err := m.Install(name); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(m.GlobalDir, "aaa", "local"), "local")
	target := filepath.Join(repo, "skills", "group", "example")
	mustWrite(t, filepath.Join(target, "upstream"), "upstream")
	mustWrite(t, filepath.Join(m.GlobalDir, "group-example", "local"), "local")
	err := m.ContributeAll(repo, func() (bool, error) { return true, nil })
	if err != nil {
		t.Fatalf("unexpected diagnostic: %v", err)
	}
	for _, path := range []string{filepath.Join(repo, "skills", "aaa", "local"), filepath.Join(target, "upstream"), filepath.Join(target, "local")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
}
