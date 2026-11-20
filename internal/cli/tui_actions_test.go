package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/manager"
	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
	"github.com/adwise/developer-skills-manager/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIRemovalRoutingAndProtection(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "local"}[local], func(t *testing.T) {
			root := t.TempDir()
			var out bytes.Buffer
			var scopes []sourceManager
			var groups []tui.SourceGroup
			for _, name := range []string{"one", "two"} {
				repo := filepath.Join(root, name, "repo")
				fixtureSkill(t, repo, "example", "source")
				mgr := manager.Manager{Repository: source.Repository{Source: repo}, GlobalDir: filepath.Join(root, name, "installed"), StateDir: filepath.Join(root, name, "state"), Out: &out}
				if err := mgr.Install("example"); err != nil {
					t.Fatal(err)
				}
				scopes = append(scopes, sourceManager{name: name, Manager: mgr})
				groups = append(groups, tui.SourceGroup{Name: name, Skills: []skill.Metadata{{ID: "example"}}, Installed: map[string]bool{"example": true}})
			}
			target := filepath.Join(scopes[0].GlobalDir, "example")
			if local {
				fixtureFile(t, filepath.Join(target, "local.txt"), "keep")
			}
			model := tui.NewSourceInstallerModel(groups)
			next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
			model = next.(tui.InstallerModel)
			ids := model.SelectedRemovals()
			if len(ids) != 1 || ids[0] != "one/example" {
				t.Fatal(ids)
			}
			err := applySourcesSelection(scopes, tui.InstallerSelection{Removals: ids}, strings.NewReader(""), &out)
			if (err != nil) != local {
				t.Fatalf("local=%v: %v", local, err)
			}
			_, statErr := os.Stat(target)
			if local && statErr != nil {
				t.Fatal("protected target removed", statErr)
			}
			if !local && !os.IsNotExist(statErr) {
				t.Fatal("queued target not removed", statErr)
			}
			if _, err := os.Stat(filepath.Join(scopes[1].GlobalDir, "example")); err != nil {
				t.Fatal("other source removed", err)
			}
		})
	}
}
