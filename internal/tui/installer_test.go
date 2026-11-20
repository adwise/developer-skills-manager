package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

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

	if view := model.View(); !strings.Contains(view, "[↑] code-review  upstream") || !strings.Contains(view, "Review code changes.") {
		t.Fatalf("update is missing from view:\n%s", view)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(InstallerModel)
	if selected := model.SelectedUpdates(); !reflect.DeepEqual(selected, []string{"code-review"}) {
		t.Fatalf("unexpected update selection: %#v", selected)
	}
	if view := model.View(); !strings.Contains(view, "[x] code-review  update") {
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
	if view := model.View(); !strings.Contains(view, "[-] code-review  remove") {
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

func TestInstallerWidthLegendAndColor(t *testing.T) {
	model := NewInstallerModelWithStatus([]skill.Metadata{{ID: "example", Description: "A long description"}}, nil, map[string]bool{"example": true}, map[string]bool{"example": true}, map[string]string{"example": "both"})
	model.width = 40
	plain := model.View()
	if strings.Contains(plain, "\x1b[") || !strings.Contains(plain, "[!] example  both") || strings.Contains(plain, "verified baseline") || strings.Contains(plain, "clean: unchanged") || strings.Contains(plain, "review diff first") {
		t.Fatal(plain)
	}
	model.color = true
	colored := model.View()
	if !strings.Contains(colored, "\x1b[1;31mboth\x1b[0m") || ansi.Strip(colored) != plain {
		t.Fatal(colored)
	}
	for _, line := range strings.Split(ansi.Strip(colored), "\n") {
		if runewidth.StringWidth(line) > model.width {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func TestInstallerAlignedColumnsAcrossPages(t *testing.T) {
	model := NewInstallerModelWithStatus([]skill.Metadata{
		{ID: "短", Description: "First"},
		{ID: "longer-name", Description: "Second"},
		{ID: "third", Description: "Third"},
		{ID: "a-much-longer-name-off-page", Description: "Fourth"},
	}, nil, nil, nil, map[string]string{"短": "clean", "longer-name": "local"})
	model.height = 12
	view := model.View()
	var positions []int
	for _, line := range strings.Split(view, "\n") {
		for _, status := range []string{"clean", "local"} {
			if i := strings.Index(line, status); i >= 0 && strings.Contains(line, "] ") {
				positions = append(positions, runewidth.StringWidth(line[:i]))
			}
		}
	}
	if len(positions) != 2 || positions[0] != positions[1] || positions[0] != 35 {
		t.Fatalf("status columns %v:\n%s", positions, view)
	}
	if !strings.Contains(view, "Name") || !strings.Contains(view, "Status") {
		t.Fatal(view)
	}
}

func TestInstallerNarrowColumnsKeepStatus(t *testing.T) {
	model := NewInstallerModelWithStatus([]skill.Metadata{{ID: "very-long-skill-name", Description: "Long description"}}, nil, nil, nil, map[string]string{"very-long-skill-name": "local"})
	model.width = 25
	view := model.View()
	if !strings.Contains(view, "local") || !strings.Contains(view, "…") {
		t.Fatal(view)
	}
	for _, line := range strings.Split(view, "\n") {
		if runewidth.StringWidth(line) > model.width {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func TestInstallerDisplaysBaselineStates(t *testing.T) {
	for _, state := range []string{"local", "upstream", "both", "unknown (differs from upstream)"} {
		model := NewInstallerModelWithStatus([]skill.Metadata{{ID: "example"}}, nil, map[string]bool{"example": true}, map[string]bool{"example": true}, map[string]string{"example": state})
		view := model.View()
		if !strings.Contains(view, "Status: "+state) || strings.Contains(view, "update available") {
			t.Fatalf("%s: %s", state, view)
		}
		if (state == "local" || strings.HasPrefix(state, "unknown")) && !strings.Contains(view, "[!] example") {
			t.Fatal(view)
		}
		model.toggleCurrent()
		if len(model.SelectedUpdates()) != 1 {
			t.Fatalf("%s is not selectable", state)
		}
	}
}
