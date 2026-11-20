package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAllSourcesCombinedOrderAndKeyboardRouting(t *testing.T) {
	m := NewSourceInstallerModel([]SourceGroup{
		{Name: "z", Skills: []skill.Metadata{{ID: "z"}, {ID: "a"}}, Sets: []source.SkillSet{{Name: "z", Skills: []string{"z"}}, {Name: "a", Skills: []string{"a"}}}},
		{Name: "a", Skills: []skill.Metadata{{ID: "x"}}, Sets: []source.SkillSet{{Name: "x", Skills: []string{"x"}}}},
	})
	m.width, m.height = 80, 40
	var labels []string
	for _, item := range m.rows {
		labels = append(labels, m.label(item))
	}
	want := []string{"a/@x", "z/@a", "z/@z", "a/x", "z/a", "z/z"}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("overview order: %v", labels)
	}
	view := m.View()
	if strings.Count(view, "── SETS") != 1 || strings.Count(view, "── SKILLS") != 1 || !strings.Contains(view, "\n\n── SKILLS") || strings.Contains(view, " / SETS") {
		t.Fatalf("overview sections:\n%s", view)
	}
	m, _ = tabKey(m, tea.KeyDown)
	m, _ = tabKey(m, tea.KeySpace)
	if !reflect.DeepEqual(m.Selected(), []string{"z/a"}) {
		t.Fatal("overview set routed incorrectly", m.Selected())
	}
	m, _ = tabKey(m, tea.KeyTab)
	if m.label(m.rows[0]) != "@z" || m.label(m.rows[1]) != "@a" {
		t.Fatal("source tab order or labels changed")
	}
	m, _ = tabKey(m, tea.KeyDown)
	m, _ = tabKey(m, tea.KeySpace)
	if len(m.Selected()) != 0 {
		t.Fatal("source tab did not toggle the same set", m.Selected())
	}
	m, _ = tabKey(m, tea.KeyShiftTab)
	if m.cursor != 1 || m.label(m.rows[m.cursor]) != "z/@a" {
		t.Fatal("overview focus lost")
	}
}

func TestSourceGroupingAndLocalSetSelection(t *testing.T) {
	groups := []SourceGroup{
		{Name: "adwise", Namespace: "skills-manager", Skills: []skill.Metadata{{ID: "nested/review", Description: "A"}}, Sets: []source.SkillSet{{Name: "work", Skills: []string{"nested/review"}}}},
		{Name: "personal", Namespace: "personal", Skills: []skill.Metadata{{ID: "nested/review", Description: "B"}}, Sets: []source.SkillSet{{Name: "work", Skills: []string{"nested/review"}}}, Installed: map[string]bool{"nested/review": true}, Updates: map[string]bool{"nested/review": true}, States: map[string]string{"nested/review": "local"}},
	}
	m := NewSourceInstallerModel(groups)
	m.height, m.width = 40, 120
	view := m.View()
	for _, text := range []string{"── SETS", "── SKILLS", "adwise/nested/review", "personal/nested/review", "local"} {
		if !strings.Contains(view, text) {
			t.Fatalf("missing %q:\n%s", text, view)
		}
	}
	if len(m.rows) != 4 {
		t.Fatal("headers became selectable")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(InstallerModel)
	if !reflect.DeepEqual(m.Selected(), []string{"adwise/nested/review"}) || len(m.SelectedUpdates()) != 0 {
		t.Fatal("set crossed source boundary", m.Selected(), m.SelectedUpdates())
	}
	m.cursor = 1
	m.toggleCurrent()
	if !reflect.DeepEqual(m.SelectedUpdates(), []string{"personal/nested/review"}) {
		t.Fatal(m.SelectedUpdates())
	}
	m.cursor = 3
	m.toggleRemoval()
	if !reflect.DeepEqual(m.SelectedRemovals(), []string{"personal/nested/review"}) || len(m.SelectedUpdates()) != 0 {
		t.Fatal("removal routing")
	}
	m.color = true
	if !strings.Contains(m.View(), "\x1b[") {
		t.Fatal("lost colors")
	}
}
