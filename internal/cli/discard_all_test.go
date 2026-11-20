package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type confirmationEditReader struct {
	edit  func()
	input io.Reader
}

func (r *confirmationEditReader) Read(p []byte) (int, error) {
	if r.edit != nil {
		r.edit()
		r.edit = nil
	}
	return r.input.Read(p)
}

func TestDiscardAllRechecksAfterConfirmation(t *testing.T) {
	for _, kind := range []string{"local", "source", "dependencies", "target"} {
		t.Run(kind, func(t *testing.T) {
			home, _, b, run := multiFixture(t)
			run(true, "install", "adwise/review", "personal/review")
			first := filepath.Join(home, ".agents", "skills", "skills-manager", "review", "local.txt")
			second := filepath.Join(home, ".agents", "skills", "personal", "review")
			fixtureFile(t, first, "preserve")
			reader := &confirmationEditReader{input: strings.NewReader("yes\n"), edit: func() {
				switch kind {
				case "local":
					fixtureFile(t, filepath.Join(second, "late.txt"), "late edit")
				case "source":
					fixtureSkill(t, b, "review", "late source edit")
				case "dependencies":
					fixtureSkill(t, b, "dependency", "new dependency")
					fixtureFile(t, filepath.Join(b, "skills.json"), `{"dependencies":{"review":["dependency"]}}`)
				case "target":
					if err := os.Symlink(b, filepath.Join(second, "link")); err != nil {
						t.Fatal(err)
					}
				}
			}}
			var out, stderr bytes.Buffer
			if code := RunWithIO([]string{"update", "--all", "--discard-local"}, reader, &out, &stderr); code == 0 {
				t.Fatalf("accepted changed selection: %s", &out)
			}
			if data, _ := os.ReadFile(first); string(data) != "preserve" {
				t.Fatal("partial write before all rechecks")
			}
		})
	}
}

func TestDiscardAllSourcesAndFilter(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		t.Run(map[bool]string{false: "all", true: "filtered"}[filtered], func(t *testing.T) {
			home, a, b, run := multiFixture(t)
			run(true, "install", "adwise/review", "personal/review")
			targets := []string{}
			for _, ns := range []string{"skills-manager", "personal"} {
				target := filepath.Join(home, ".agents", "skills", ns, "review", "SKILL.md")
				fixtureFile(t, target, "invalid local edit")
				targets = append(targets, target)
			}
			fixtureSkill(t, a, "dependency", "new dependency")
			fixtureSkill(t, a, "uninstalled", "not selected")
			fixtureSkill(t, b, "uninstalled", "not selected")
			fixtureFile(t, filepath.Join(a, "skills.json"), `{"dependencies":{"review":["dependency"]}}`)
			args := []string{"update", "--all", "--discard-local"}
			if filtered {
				args = append(args, "--from", "adwise")
			}
			// Explicit no and EOF leave packages and baseline state unchanged.
			for _, input := range []string{"no\n", ""} {
				var out, stderr bytes.Buffer
				if code := RunWithIO(args, strings.NewReader(input), &out, &stderr); code != 0 {
					t.Fatalf("%d %s", code, &stderr)
				}
				if !strings.Contains(out.String(), "Cancelled.") {
					t.Fatal(out.String())
				}
				for _, target := range targets {
					if data, _ := os.ReadFile(target); string(data) != "invalid local edit" {
						t.Fatal("cancel wrote files")
					}
				}
				if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "skills-manager", "dependency")); !os.IsNotExist(err) {
					t.Fatal("cancel installed dependency")
				}
			}
			run(false, "update", "--all") // Ordinary update still protects local edits.
			text := run(true, append(args, "--yes")...)
			for _, want := range []string{"dependency: install dependency", "-invalid local edit", "permanently overwritten without backup"} {
				if !strings.Contains(text, want) {
					t.Fatal(text)
				}
			}
			// All diffs must precede the first write.
			preview := strings.Index(text, "DISCARD LOCAL CHANGES AND UPDATE")
			if preview < 0 || strings.Index(text, "↑ Updated") < preview {
				t.Fatal(text)
			}
			for i, ns := range []string{"skills-manager", "personal"} {
				data, _ := os.ReadFile(targets[i])
				if (string(data) == "invalid local edit") != (filtered && i == 1) {
					t.Fatalf("wrong source selection: %s", data)
				}
				if _, err := os.Stat(filepath.Join(home, ".agents", ".skills-manager", ns, "backups")); !os.IsNotExist(err) {
					t.Fatal("created backups", err)
				}
				if _, err := os.Stat(filepath.Join(home, ".agents", "skills", ns, "uninstalled")); !os.IsNotExist(err) {
					t.Fatal("installed unselected skill")
				}
			}
		})
	}
}

func TestDiscardAllInvalidArgumentsAndPreflight(t *testing.T) {
	home, _, b, run := multiFixture(t)
	run(true, "install", "adwise/review", "personal/review")
	target := filepath.Join(home, ".agents", "skills", "skills-manager", "review", "local.txt")
	fixtureFile(t, target, "preserve")
	for _, args := range [][]string{
		{"update", "--all", "adwise/review", "--discard-local", "--yes"},
		{"update", "--all", "adwise/review"},
		{"update", "--discard-local", "--yes"},
		{"update", "--all", "--discard-local", "--bogus"},
		{"update", "--all", "--discard-local", "--from", "missing"},
	} {
		run(false, args...)
	}
	for _, command := range []string{"available", "search", "list", "status", "diff", "install", "uninstall", "tui", "doctor", "validate", "version", "help", "source"} {
		run(false, command, "--all")
	}
	// A missing upstream package in the last source must fail before updating the first.
	if err := os.RemoveAll(filepath.Join(b, "skills", "review")); err != nil {
		t.Fatal(err)
	}
	fixtureSkill(t, b, "other", "keep catalog valid")
	run(false, "update", "--all", "--discard-local", "--yes")
	if data, _ := os.ReadFile(target); string(data) != "preserve" {
		t.Fatal("partial update on known error")
	}
	run(true, "update", "--all", "--discard-local", "--from", "adwise", "--yes")
}
