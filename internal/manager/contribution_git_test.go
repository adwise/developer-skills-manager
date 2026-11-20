package manager

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContributionCheckoutIgnoresAndModes(t *testing.T) {
	m, repo, out := workflowFixture(t)
	workflowGit(t, repo, "init", "-q")
	id := "group-example"
	target := filepath.Join(repo, "skills/group/example")
	local := filepath.Join(m.GlobalDir, id)
	if err := os.Mkdir(filepath.Join(target, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(repo, ".gitignore"), "*.tmp\n")
	mustWrite(t, filepath.Join(target, "nested/.gitignore"), "!keep.tmp\n")
	for _, p := range []string{"cached.tmp", "tracked.pyc", "mode", "other-mode"} {
		mustWrite(t, filepath.Join(target, p), "original")
	}
	workflowGit(t, repo, "add", "-f", "--", "skills/group/example/tracked.pyc")
	index := filepath.Join(repo, ".git/index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Install(id); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"nested/private.tmp", "cache.pyo", "escape\nname.tmp"} {
		mustWrite(t, filepath.Join(local, p), "ignored secret")
	}
	if err := os.Remove(filepath.Join(local, "cached.tmp")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(local, ".pytest_cache"), 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(local, ".pytest_cache/state"), "private")
	if err := os.Chmod(filepath.Join(local, "other-mode"), 0655); err != nil {
		t.Fatal(err)
	}
	if err := m.ContributeAll(repo, func() (bool, error) { t.Fatal("ignored-only prompted"); return false, nil }); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"nested/keep.tmp", "tracked.pyc", ":odd*[\n"} {
		mustWrite(t, filepath.Join(local, p), "eligible")
	}
	if err := os.Chmod(filepath.Join(local, "mode"), 0744); err != nil {
		t.Fatal(err)
	}
	if err := m.ContributeAll(repo, func() (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"nested/keep.tmp", "tracked.pyc", ":odd*[\n"} {
		data, err := os.ReadFile(filepath.Join(target, p))
		if err != nil || string(data) != "eligible" {
			t.Fatal(p, err)
		}
	}
	for _, p := range []string{"nested/private.tmp", "cache.pyo", "escape\nname.tmp", ".pytest_cache"} {
		if _, err := os.Stat(filepath.Join(target, p)); !os.IsNotExist(err) {
			t.Fatal(p, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(target, "cached.tmp")); err != nil || string(data) != "original" {
		t.Fatal("ignored deletion")
	}
	for p, want := range map[string]os.FileMode{"mode": 0744, "other-mode": 0644} {
		info, err := os.Stat(filepath.Join(target, p))
		if err != nil || info.Mode().Perm() != want {
			t.Fatal(p, err)
		}
	}
	after, err := os.ReadFile(index)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatal("changed index")
	}
	if strings.Contains(out.String(), "secret") {
		t.Fatal(out)
	}
}

func TestContributionInvalidProjectionAndTypeConflict(t *testing.T) {
	m, repo, out := workflowFixture(t)
	workflowGit(t, repo, "init", "-q")
	target := filepath.Join(repo, "skills/group/example")
	mustWrite(t, filepath.Join(target, "ref.txt"), "base")
	id := "group-example"
	if err := m.Install(id); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(m.GlobalDir, id)
	if err := os.Remove(filepath.Join(local, "ref.txt")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(target, "checkout.md"), "[reference](ref.txt)")
	mustWrite(t, filepath.Join(local, "safe"), "local")
	if err := m.Contribute(id, repo, func() (bool, error) { t.Fatal("invalid prompted"); return true, nil }); err != ErrContributionPartial || !strings.Contains(out.String(), "projected package fails validation") {
		t.Fatal(err, out)
	}
	if _, err := os.Stat(filepath.Join(target, "safe")); !os.IsNotExist(err) {
		t.Fatal("wrote invalid projection")
	}
	if err := os.Remove(filepath.Join(target, "checkout.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(local, "ref.txt"), 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(local, "ref.txt", "child"), "new")
	if err := m.Contribute(id, repo, func() (bool, error) { return true, nil }); err != ErrContributionPartial {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(target, "ref.txt")); err != nil || string(data) != "base" {
		t.Fatal("overwrote type conflict")
	}
	if _, err := os.Stat(filepath.Join(target, "safe")); err != nil {
		t.Fatal(err)
	}
}

func TestContributionIgnoreChangeAfterPreview(t *testing.T) {
	m, repo, _ := workflowFixture(t)
	workflowGit(t, repo, "init", "-q")
	id := "group-example"
	if err := m.Install(id); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(m.GlobalDir, id, "late"), "local")
	err := m.Contribute(id, repo, func() (bool, error) {
		mustWrite(t, filepath.Join(repo, ".git/info/exclude"), "late\n")
		return true, nil
	})
	if err == nil || !strings.Contains(err.Error(), "changed during confirmation") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "skills/group/example/late")); !os.IsNotExist(err) {
		t.Fatal("wrote after ignore change")
	}
}
