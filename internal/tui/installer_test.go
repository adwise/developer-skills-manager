package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
)

func TestInstallerSelectsMultipleSkillsThroughSet(t *testing.T) {
	model := NewInstallerModel(
		[]skill.Metadata{{ID: "code-review"}, {ID: "noah/test"}},
		[]source.SkillSet{{Name: "team", Skills: []string{"code-review", "noah/test"}}},
		map[string]bool{},
	)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(InstallerModel)
	if selected := model.Selected(); !reflect.DeepEqual(selected, []string{"code-review", "noah/test"}) {
		t.Fatalf("unexpected selection: %#v", selected)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if selected := updated.(InstallerModel).Selected(); len(selected) != 0 {
		t.Fatalf("expected set to be deselected, got %#v", selected)
	}
}

func TestInstallerShowsAndSelectsAvailableUpdate(t *testing.T) {
	model := NewInstallerModelWithUpdates(
		[]skill.Metadata{{ID: "code-review", Description: "Review code changes."}},
		nil,
		map[string]bool{"code-review": true},
		map[string]bool{"code-review": true},
	)

	if view := model.View(); !strings.Contains(view, "[↑] code-review — update available — Review code changes.") {
		t.Fatalf("update is missing from view:\n%s", view)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(InstallerModel)
	if selected := model.SelectedUpdates(); !reflect.DeepEqual(selected, []string{"code-review"}) {
		t.Fatalf("unexpected update selection: %#v", selected)
	}
	if view := model.View(); !strings.Contains(view, "[x] code-review — update available") {
		t.Fatalf("selected update is missing from view:\n%s", view)
	}
}

func TestInstallerSelectsInstalledSkillForRemoval(t *testing.T) {
	model := NewInstallerModelWithUpdates(
		[]skill.Metadata{{ID: "code-review", Description: "Review code changes."}},
		nil,
		map[string]bool{"code-review": true},
		map[string]bool{"code-review": true},
	)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(InstallerModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(InstallerModel)
	if selected := model.SelectedRemovals(); !reflect.DeepEqual(selected, []string{"code-review"}) {
		t.Fatalf("unexpected removal selection: %#v", selected)
	}
	if selected := model.SelectedUpdates(); len(selected) != 0 {
		t.Fatalf("skill was selected for update and removal: %#v", selected)
	}
	if view := model.View(); !strings.Contains(view, "[-] code-review — remove") {
		t.Fatalf("selected removal is missing from view:\n%s", view)
	}
}

func TestInstallerSetSelectsInstallsAndUpdates(t *testing.T) {
	model := NewInstallerModelWithUpdates(
		[]skill.Metadata{{ID: "code-review"}, {ID: "noah/test"}},
		[]source.SkillSet{{Name: "team", Skills: []string{"code-review", "noah/test"}}},
		map[string]bool{"code-review": true},
		map[string]bool{"code-review": true},
	)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(InstallerModel)
	if selected := model.Selected(); !reflect.DeepEqual(selected, []string{"noah/test"}) {
		t.Fatalf("unexpected install selection: %#v", selected)
	}
	if selected := model.SelectedUpdates(); !reflect.DeepEqual(selected, []string{"code-review"}) {
		t.Fatalf("unexpected update selection: %#v", selected)
	}
}

func TestInstallerDoesNotReinstallInstalledSkillFromSet(t *testing.T) {
	model := NewInstallerModel(
		[]skill.Metadata{{ID: "code-review"}, {ID: "noah/test"}},
		[]source.SkillSet{{Name: "team", Skills: []string{"code-review", "noah/test"}}},
		map[string]bool{"code-review": true},
	)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	selected := updated.(InstallerModel).Selected()
	if !reflect.DeepEqual(selected, []string{"noah/test"}) {
		t.Fatalf("unexpected selection: %#v", selected)
	}
}
