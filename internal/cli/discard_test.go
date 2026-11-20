package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/manager"
	"github.com/adwise/developer-skills-manager/internal/source"
	"github.com/adwise/developer-skills-manager/internal/tui"
)

func TestDiscardCLIConfirmationAndSourceRouting(t *testing.T) {
	home, a, _, run := multiFixture(t)
	run(true, "install", "adwise/review", "personal/review")
	target := filepath.Join(home, ".agents", "skills", "skills-manager", "review", "SKILL.md")
	fixtureFile(t, target, "invalid local edit")
	fixtureSkill(t, a, "review", "new a source")
	if text := run(false, "update", "review", "--discard-local", "--yes"); !strings.Contains(text, "ambiguous") {
		t.Fatal(text)
	}
	run(false, "update", "--discard-local", "--yes")
	for _, command := range []string{"available", "install", "uninstall", "contribute", "source", "validate"} {
		run(false, command, "review", "--discard-local")
	}
	// EOF and an explicit no cancel without writing even a backup.
	for _, input := range []string{"", "no\n"} {
		var out, stderr bytes.Buffer
		if code := RunWithIO([]string{"update", "adwise/review", "--discard-local"}, strings.NewReader(input), &out, &stderr); code != 0 {
			t.Fatalf("%d %s", code, &stderr)
		}
		if !strings.Contains(out.String(), "Cancelled.") {
			t.Fatal(out.String())
		}
		if data, _ := os.ReadFile(target); string(data) != "invalid local edit" {
			t.Fatal("cancel replaced files")
		}
		matches, _ := filepath.Glob(filepath.Join(home, ".agents", ".skills-manager", "skills-manager", "backups", "*"))
		if len(matches) != 0 {
			t.Fatal("cancel created backup")
		}
	}
	text := run(true, "update", "adwise/review", "--discard-local", "--yes")
	if !strings.Contains(text, "permanently overwritten without backup") {
		t.Fatal(text)
	}
	if data, _ := os.ReadFile(target); !strings.Contains(string(data), "new a source") {
		t.Fatal(string(data))
	}
	other := filepath.Join(home, ".agents", "skills", "personal", "review", "SKILL.md")
	if data, _ := os.ReadFile(other); !strings.Contains(string(data), "b original") {
		t.Fatal("wrong source updated")
	}
}

func TestDiscardSelectionPreflightBeforeWrites(t *testing.T) {
	for _, scenario := range []string{"cancel", "conflict", "unreachable", "success"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			var out bytes.Buffer
			var scopes []sourceManager
			var selected []tui.InstallerSelection
			for _, name := range []string{"one", "two"} {
				repo := filepath.Join(root, name, "repo")
				fixtureSkill(t, repo, "example", "source")
				m := manager.Manager{Repository: source.Repository{Source: repo}, GlobalDir: filepath.Join(root, name, "installed"), StateDir: filepath.Join(root, name, "state"), Out: &out}
				if err := m.Install("example"); err != nil {
					t.Fatal(err)
				}
				fixtureFile(t, filepath.Join(m.GlobalDir, "example", "local.txt"), "keep")
				scopes = append(scopes, sourceManager{name: name, Manager: m})
				selected = append(selected, tui.InstallerSelection{Discards: []string{"example"}})
			}
			input := "yes\n"
			switch scenario {
			case "cancel":
				input = "no\n"
			case "conflict":
				if err := os.Symlink(root, filepath.Join(scopes[1].GlobalDir, "example", "unsafe")); err != nil {
					t.Fatal(err)
				}
			case "unreachable":
				scopes[1].Repository.Source = filepath.Join(root, "missing")
			}
			err := applyDiscardSelection(scopes, selected, strings.NewReader(input), &out)
			if (err != nil) != (scenario == "conflict" || scenario == "unreachable") {
				t.Fatalf("%v\n%s", err, &out)
			}
			for _, scope := range scopes {
				if _, err := os.Stat(filepath.Join(scope.StateDir, "backups")); !os.IsNotExist(err) {
					t.Fatal("TUI discard created backups", err)
				}
				_, err := os.Stat(filepath.Join(scope.GlobalDir, "example", "local.txt"))
				if scenario == "success" {
					if !os.IsNotExist(err) {
						t.Fatal("not updated")
					}
				} else if err != nil {
					t.Fatal("partial replacement before confirmation/preflight", err)
				}
			}
		})
	}
}
